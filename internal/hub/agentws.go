package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aperture/aperture/internal/store"
	"github.com/aperture/aperture/internal/types"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// ── wire frame types ────────────────────────────────────────────────────────

// agentFrame is used to peek at the "type" field before full unmarshalling.
type agentFrame struct {
	Type string `json:"type"`
}

// helloFrame is the first frame an agent sends after connecting.
type helloFrame struct {
	Type       string         `json:"type"` // "hello"
	Host       types.HostInfo `json:"host"`
	Version    string         `json:"version"`
	HasDocker  bool           `json:"has_docker"`
	HasCompose bool           `json:"has_compose"`
}

// composeReqFrame is sent hub→agent to request a compose operation.
type composeReqFrame struct {
	Type       string   `json:"type"`                 // "compose_req"
	ReqID      string   `json:"req_id"`
	Action     string   `json:"action"`               // "discover"|"get_stack"|"exec"|"logs"|"read_file"|"write_file"
	Project    string   `json:"project,omitempty"`
	WorkingDir string   `json:"working_dir,omitempty"`
	SubAction  string   `json:"sub_action,omitempty"` // for "exec": up, down, restart, pull, stop, start
	Service    string   `json:"service,omitempty"`
	ExtraArgs  []string `json:"extra_args,omitempty"`
	Content    string   `json:"content,omitempty"` // for "write_file"
	Tail       int      `json:"tail,omitempty"`    // for "logs"
}

// composeRespFrame is sent agent→hub with the result of a compose operation.
type composeRespFrame struct {
	Type  string          `json:"type"` // "compose_resp"
	ReqID string          `json:"req_id"`
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}

// metricFrame carries one metric sample.
type metricFrame struct {
	Type string            `json:"type"` // "metric"
	Data types.MetricSample `json:"data"`
}

// ackFrame is sent by the hub to confirm host registration.
type ackFrame struct {
	Type   string `json:"type"` // "ack"
	HostID string `json:"host_id"`
}

// dockerReqFrame is sent hub→agent to request a docker operation.
type dockerReqFrame struct {
	Type   string          `json:"type"` // "docker_req"
	ReqID  string          `json:"req_id"`
	Action string          `json:"action"`
	CID    string          `json:"cid,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
}

// dockerRespFrame is sent agent→hub with the result of a docker operation.
type dockerRespFrame struct {
	Type  string          `json:"type"` // "docker_resp"
	ReqID string          `json:"req_id"`
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}

// ── session ──────────────────────────────────────────────────────────────────

type agentSession struct {
	conn           *websocket.Conn
	hostID         string
	mu             sync.Mutex
	pending        map[string]chan dockerRespFrame  // docker req_id → channel
	composePending map[string]chan composeRespFrame // compose req_id → channel
}

// ── handler ──────────────────────────────────────────────────────────────────

// AgentHandler manages all active agent WebSocket connections.
// Mounted at GET /api/agents/ws.
type AgentHandler struct {
	hub      *Hub
	store    *store.Store
	log      *slog.Logger
	mu       sync.RWMutex
	sessions map[string]*agentSession // host_id → session
	reqCounter atomic.Int64
}

// NewAgentHandler constructs an AgentHandler.
func NewAgentHandler(h *Hub, st *store.Store, log *slog.Logger) *AgentHandler {
	return &AgentHandler{
		hub:      h,
		store:    st,
		log:      log,
		sessions: make(map[string]*agentSession),
	}
}

// ConnectedAgents returns the list of currently connected host IDs.
func (ah *AgentHandler) ConnectedAgents() []string {
	ah.mu.RLock()
	defer ah.mu.RUnlock()
	out := make([]string, 0, len(ah.sessions))
	for id := range ah.sessions {
		out = append(out, id)
	}
	return out
}

// ServeHTTP handles the WebSocket upgrade for agent connections.
func (ah *AgentHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. Extract and verify bearer token.
	authHdr := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHdr, "Bearer ") {
		http.Error(w, "missing bearer token", http.StatusUnauthorized)
		return
	}
	plaintext := strings.TrimPrefix(authHdr, "Bearer ")

	if _, err := ah.store.VerifyAgentToken(r.Context(), plaintext); err != nil {
		ah.log.Warn("agent auth failed", "err", err, "remote", r.RemoteAddr)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// 2. Upgrade to WebSocket.
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // CORS skipped; token provides auth
	})
	if err != nil {
		ah.log.Warn("ws accept failed", "err", err)
		return
	}

	// 3. Read hello frame (10s timeout).
	helloCtx, helloCancel := context.WithTimeout(r.Context(), 10*time.Second)
	var raw json.RawMessage
	if err := wsjson.Read(helloCtx, conn, &raw); err != nil {
		helloCancel()
		ah.log.Warn("read hello failed", "err", err)
		conn.Close(websocket.StatusPolicyViolation, "expected hello frame")
		return
	}
	helloCancel()

	var frame agentFrame
	if err := json.Unmarshal(raw, &frame); err != nil || frame.Type != "hello" {
		ah.log.Warn("invalid hello frame", "type", frame.Type)
		conn.Close(websocket.StatusPolicyViolation, "expected hello frame")
		return
	}
	var hello helloFrame
	if err := json.Unmarshal(raw, &hello); err != nil {
		ah.log.Warn("unmarshal hello failed", "err", err)
		conn.Close(websocket.StatusPolicyViolation, "malformed hello")
		return
	}

	// 4. Register the host.
	hello.Host.Source = "agent"
	hostID := DeriveHostID(hello.Host)
	now := time.Now().UTC()
	host := types.Host{
		ID:           hostID,
		Name:         hello.Host.Name,
		OS:           hello.Host.OS,
		Platform:     hello.Host.Platform,
		Kernel:       hello.Host.Kernel,
		Arch:         hello.Host.Arch,
		CPUModel:     hello.Host.CPUModel,
		CPUCount:     hello.Host.CPUCount,
		MemTotal:     hello.Host.MemTotal,
		Source:       "agent",
		AgentVersion: hello.Version,
		CreatedAt:    now,
		LastSeen:     now,
	}
	if err := ah.store.UpsertHost(r.Context(), host); err != nil {
		ah.log.Error("upsert host", "err", err, "host_id", hostID)
		conn.Close(websocket.StatusInternalError, "registration failed")
		return
	}

	ah.hub.mu.Lock()
	ah.hub.hosts[hostID] = host
	ah.hub.mu.Unlock()

	// 5. Register session.
	sess := &agentSession{
		conn:           conn,
		hostID:         hostID,
		pending:        make(map[string]chan dockerRespFrame),
		composePending: make(map[string]chan composeRespFrame),
	}
	ah.mu.Lock()
	ah.sessions[hostID] = sess
	ah.mu.Unlock()

	// 6. Optionally register docker + compose providers.
	if hello.HasDocker {
		dp := &agentDockerProvider{handler: ah, hostID: hostID}
		ah.hub.RegisterDocker(hostID, dp)
		ah.log.Info("docker provider active", "host_id", hostID)
	}
	if hello.HasCompose {
		cp := &agentComposeProvider{handler: ah, hostID: hostID}
		ah.hub.RegisterCompose(hostID, cp)
		ah.log.Info("compose provider active", "host_id", hostID)
	}

	// 7. Send ack.
	ack := ackFrame{Type: "ack", HostID: hostID}
	if err := wsjson.Write(r.Context(), conn, ack); err != nil {
		ah.log.Warn("write ack failed", "host_id", hostID, "err", err)
	}

	ah.log.Info("agent connected", "host_id", hostID, "name", hello.Host.Name, "version", hello.Version)

	// 8. Receive loop — runs until the connection closes.
	defer func() {
		ah.mu.Lock()
		delete(ah.sessions, hostID)
		ah.mu.Unlock()

		ah.hub.mu.Lock()
		delete(ah.hub.dockers, hostID)
		delete(ah.hub.composes, hostID)
		ah.hub.mu.Unlock()

		// Drain pending requests so callers unblock immediately.
		sess.mu.Lock()
		for reqID, ch := range sess.pending {
			ch <- dockerRespFrame{ReqID: reqID, OK: false, Error: "agent disconnected"}
			delete(sess.pending, reqID)
		}
		for reqID, ch := range sess.composePending {
			ch <- composeRespFrame{ReqID: reqID, OK: false, Error: "agent disconnected"}
			delete(sess.composePending, reqID)
		}
		sess.mu.Unlock()

		ah.log.Info("agent disconnected", "host_id", hostID)
	}()

	samplesIn := ah.hub.samplesIn(hostID)

	for {
		var raw json.RawMessage
		if err := wsjson.Read(r.Context(), conn, &raw); err != nil {
			// Normal close or context cancelled.
			break
		}
		var peek agentFrame
		if err := json.Unmarshal(raw, &peek); err != nil {
			continue
		}
		switch peek.Type {
		case "metric":
			var mf metricFrame
			if err := json.Unmarshal(raw, &mf); err == nil {
				mf.Data.HostID = hostID
				select {
				case samplesIn <- mf.Data:
				default:
					ah.log.Warn("dropping metric: buffer full", "host_id", hostID)
				}
			}
		case "heartbeat":
			_ = ah.store.TouchHost(r.Context(), hostID, time.Now().UTC())
		case "docker_resp":
			var resp dockerRespFrame
			if err := json.Unmarshal(raw, &resp); err != nil {
				continue
			}
			sess.mu.Lock()
			ch, ok := sess.pending[resp.ReqID]
			if ok {
				delete(sess.pending, resp.ReqID)
			}
			sess.mu.Unlock()
			if ok {
				ch <- resp
			}
		case "compose_resp":
			var resp composeRespFrame
			if err := json.Unmarshal(raw, &resp); err != nil {
				continue
			}
			sess.mu.Lock()
			ch, ok := sess.composePending[resp.ReqID]
			if ok {
				delete(sess.composePending, resp.ReqID)
			}
			sess.mu.Unlock()
			if ok {
				ch <- resp
			}
		}
	}
}

// sendDockerCmd sends a docker request to the agent and waits for the response.
func (ah *AgentHandler) sendDockerCmd(ctx context.Context, hostID, action, cid string, params json.RawMessage) (json.RawMessage, error) {
	ah.mu.RLock()
	sess, ok := ah.sessions[hostID]
	ah.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("agent not connected for host %s", hostID)
	}

	reqID := fmt.Sprintf("%d", ah.reqCounter.Add(1))
	ch := make(chan dockerRespFrame, 1)

	sess.mu.Lock()
	sess.pending[reqID] = ch
	sess.mu.Unlock()

	req := dockerReqFrame{
		Type:   "docker_req",
		ReqID:  reqID,
		Action: action,
		CID:    cid,
		Params: params,
	}

	writeCtx, writeCancel := context.WithTimeout(ctx, 5*time.Second)
	defer writeCancel()
	if err := wsjson.Write(writeCtx, sess.conn, req); err != nil {
		sess.mu.Lock()
		delete(sess.pending, reqID)
		sess.mu.Unlock()
		return nil, fmt.Errorf("send docker_req: %w", err)
	}

	timeout := 30 * time.Second
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case resp := <-ch:
		if !resp.OK {
			return nil, fmt.Errorf("agent docker error: %s", resp.Error)
		}
		return resp.Data, nil
	case <-timer.C:
		sess.mu.Lock()
		delete(sess.pending, reqID)
		sess.mu.Unlock()
		return nil, fmt.Errorf("docker command timed out after %s", timeout)
	case <-ctx.Done():
		sess.mu.Lock()
		delete(sess.pending, reqID)
		sess.mu.Unlock()
		return nil, ctx.Err()
	}
}

// ── agentDockerProvider ──────────────────────────────────────────────────────

// agentDockerProvider implements hub.DockerProvider by forwarding calls over
// the WebSocket connection to the remote agent.
type agentDockerProvider struct {
	handler *AgentHandler
	hostID  string
}

func marshalParams(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func (p *agentDockerProvider) List(ctx context.Context, all bool) ([]types.Container, error) {
	params := marshalParams(map[string]any{"all": all})
	data, err := p.handler.sendDockerCmd(ctx, p.hostID, "list", "", params)
	if err != nil {
		return nil, err
	}
	var out []types.Container
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (p *agentDockerProvider) Create(ctx context.Context, spec types.CreateSpec) (string, error) {
	params := marshalParams(spec)
	data, err := p.handler.sendDockerCmd(ctx, p.hostID, "create", "", params)
	if err != nil {
		return "", err
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", err
	}
	return resp.ID, nil
}

func (p *agentDockerProvider) Start(ctx context.Context, id string) error {
	_, err := p.handler.sendDockerCmd(ctx, p.hostID, "start", id, nil)
	return err
}

func (p *agentDockerProvider) Stop(ctx context.Context, id string, timeoutSec *int) error {
	var params json.RawMessage
	if timeoutSec != nil {
		params = marshalParams(map[string]any{"timeout_sec": *timeoutSec})
	}
	_, err := p.handler.sendDockerCmd(ctx, p.hostID, "stop", id, params)
	return err
}

func (p *agentDockerProvider) Restart(ctx context.Context, id string, timeoutSec *int) error {
	var params json.RawMessage
	if timeoutSec != nil {
		params = marshalParams(map[string]any{"timeout_sec": *timeoutSec})
	}
	_, err := p.handler.sendDockerCmd(ctx, p.hostID, "restart", id, params)
	return err
}

func (p *agentDockerProvider) Pause(ctx context.Context, id string) error {
	_, err := p.handler.sendDockerCmd(ctx, p.hostID, "pause", id, nil)
	return err
}

func (p *agentDockerProvider) Unpause(ctx context.Context, id string) error {
	_, err := p.handler.sendDockerCmd(ctx, p.hostID, "unpause", id, nil)
	return err
}

func (p *agentDockerProvider) Kill(ctx context.Context, id, signal string) error {
	params := marshalParams(map[string]any{"signal": signal})
	_, err := p.handler.sendDockerCmd(ctx, p.hostID, "kill", id, params)
	return err
}

func (p *agentDockerProvider) Remove(ctx context.Context, id string, force, removeVolumes bool) error {
	params := marshalParams(map[string]any{"force": force, "remove_volumes": removeVolumes})
	_, err := p.handler.sendDockerCmd(ctx, p.hostID, "remove", id, params)
	return err
}

func (p *agentDockerProvider) Logs(ctx context.Context, id string, tail int) (string, error) {
	params := marshalParams(map[string]any{"tail": tail})
	data, err := p.handler.sendDockerCmd(ctx, p.hostID, "logs", id, params)
	if err != nil {
		return "", err
	}
	var resp struct {
		Logs string `json:"logs"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", err
	}
	return resp.Logs, nil
}

func (p *agentDockerProvider) Inspect(ctx context.Context, id string) (*types.ContainerInspect, error) {
	data, err := p.handler.sendDockerCmd(ctx, p.hostID, "inspect", id, nil)
	if err != nil {
		return nil, err
	}
	var out types.ContainerInspect
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (p *agentDockerProvider) UpdateResources(ctx context.Context, id string, update types.ResourceUpdate) error {
	params := marshalParams(update)
	_, err := p.handler.sendDockerCmd(ctx, p.hostID, "update_resources", id, params)
	return err
}

func (p *agentDockerProvider) ListNetworks(ctx context.Context) ([]types.DockerNetwork, error) {
	data, err := p.handler.sendDockerCmd(ctx, p.hostID, "list_networks", "", nil)
	if err != nil {
		return nil, err
	}
	var out []types.DockerNetwork
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (p *agentDockerProvider) InspectNetwork(ctx context.Context, id string) (*types.DockerNetwork, error) {
	data, err := p.handler.sendDockerCmd(ctx, p.hostID, "inspect_network", id, nil)
	if err != nil {
		return nil, err
	}
	var out types.DockerNetwork
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (p *agentDockerProvider) CreateNetwork(ctx context.Context, spec types.NetworkCreateSpec) (string, error) {
	params := marshalParams(spec)
	data, err := p.handler.sendDockerCmd(ctx, p.hostID, "create_network", "", params)
	if err != nil {
		return "", err
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", err
	}
	return resp.ID, nil
}

func (p *agentDockerProvider) RemoveNetwork(ctx context.Context, id string) error {
	_, err := p.handler.sendDockerCmd(ctx, p.hostID, "remove_network", id, nil)
	return err
}

func (p *agentDockerProvider) ConnectContainer(ctx context.Context, networkID, containerID string) error {
	params := marshalParams(map[string]any{"container_id": containerID})
	_, err := p.handler.sendDockerCmd(ctx, p.hostID, "connect_network", networkID, params)
	return err
}

func (p *agentDockerProvider) DisconnectContainer(ctx context.Context, networkID, containerID string) error {
	params := marshalParams(map[string]any{"container_id": containerID})
	_, err := p.handler.sendDockerCmd(ctx, p.hostID, "disconnect_network", networkID, params)
	return err
}

func (p *agentDockerProvider) ListVolumes(ctx context.Context) ([]types.DockerVolume, error) {
	data, err := p.handler.sendDockerCmd(ctx, p.hostID, "list_volumes", "", nil)
	if err != nil {
		return nil, err
	}
	var out []types.DockerVolume
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (p *agentDockerProvider) InspectVolume(ctx context.Context, name string) (*types.DockerVolume, error) {
	data, err := p.handler.sendDockerCmd(ctx, p.hostID, "inspect_volume", name, nil)
	if err != nil {
		return nil, err
	}
	var out types.DockerVolume
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (p *agentDockerProvider) CreateVolume(ctx context.Context, spec types.VolumeCreateSpec) (string, error) {
	params := marshalParams(spec)
	data, err := p.handler.sendDockerCmd(ctx, p.hostID, "create_volume", "", params)
	if err != nil {
		return "", err
	}
	var resp struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", err
	}
	return resp.Name, nil
}

func (p *agentDockerProvider) RemoveVolume(ctx context.Context, name string, force bool) error {
	params := marshalParams(map[string]any{"force": force})
	_, err := p.handler.sendDockerCmd(ctx, p.hostID, "remove_volume", name, params)
	return err
}


// ── agentComposeProvider ─────────────────────────────────────────────────────

// agentComposeProvider implements hub.ComposeProvider by forwarding compose
// operations to the remote agent over the WebSocket connection.
type agentComposeProvider struct {
	handler *AgentHandler
	hostID  string
}

// sendComposeCmd sends a compose_req to the agent and waits for compose_resp.
// Uses a 5-minute timeout since compose pulls and builds can be slow.
func (ah *AgentHandler) sendComposeCmd(ctx context.Context, hostID string, req composeReqFrame) (json.RawMessage, error) {
	ah.mu.RLock()
	sess, ok := ah.sessions[hostID]
	ah.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("agent not connected for host %s", hostID)
	}

	reqID := fmt.Sprintf("c%d", ah.reqCounter.Add(1))
	ch := make(chan composeRespFrame, 1)

	sess.mu.Lock()
	sess.composePending[reqID] = ch
	sess.mu.Unlock()

	req.Type = "compose_req"
	req.ReqID = reqID

	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := wsjson.Write(writeCtx, sess.conn, req); err != nil {
		sess.mu.Lock()
		delete(sess.composePending, reqID)
		sess.mu.Unlock()
		return nil, fmt.Errorf("send compose_req: %w", err)
	}

	// Compose ops like image pulls can take minutes.
	const timeout = 5 * time.Minute
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case resp := <-ch:
		if !resp.OK {
			return nil, fmt.Errorf("%s", resp.Error)
		}
		return resp.Data, nil
	case <-timer.C:
		sess.mu.Lock()
		delete(sess.composePending, reqID)
		sess.mu.Unlock()
		return nil, fmt.Errorf("compose command timed out after %s", timeout)
	case <-ctx.Done():
		sess.mu.Lock()
		delete(sess.composePending, reqID)
		sess.mu.Unlock()
		return nil, ctx.Err()
	}
}

func (p *agentComposeProvider) DiscoverStacks(ctx context.Context) ([]types.ComposeStack, error) {
	data, err := p.handler.sendComposeCmd(ctx, p.hostID, composeReqFrame{Action: "discover"})
	if err != nil {
		return nil, err
	}
	var out []types.ComposeStack
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (p *agentComposeProvider) GetStack(ctx context.Context, project string) (*types.ComposeStack, error) {
	data, err := p.handler.sendComposeCmd(ctx, p.hostID, composeReqFrame{
		Action:  "get_stack",
		Project: project,
	})
	if err != nil {
		return nil, err
	}
	var out types.ComposeStack
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (p *agentComposeProvider) StackAction(ctx context.Context, project, workingDir, action, service string, extraArgs ...string) (string, error) {
	data, err := p.handler.sendComposeCmd(ctx, p.hostID, composeReqFrame{
		Action:     "exec",
		Project:    project,
		WorkingDir: workingDir,
		SubAction:  action,
		Service:    service,
		ExtraArgs:  extraArgs,
	})
	if err != nil {
		return "", err
	}
	var resp struct {
		Output string `json:"output"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", err
	}
	return resp.Output, nil
}

func (p *agentComposeProvider) Logs(ctx context.Context, project, workingDir, service string, tail int) (string, error) {
	data, err := p.handler.sendComposeCmd(ctx, p.hostID, composeReqFrame{
		Action:     "logs",
		Project:    project,
		WorkingDir: workingDir,
		Service:    service,
		Tail:       tail,
	})
	if err != nil {
		return "", err
	}
	var resp struct {
		Output string `json:"output"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", err
	}
	return resp.Output, nil
}

func (p *agentComposeProvider) ReadFile(ctx context.Context, workingDir string) (string, error) {
	data, err := p.handler.sendComposeCmd(ctx, p.hostID, composeReqFrame{
		Action:     "read_file",
		WorkingDir: workingDir,
	})
	if err != nil {
		return "", err
	}
	var resp struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", err
	}
	return resp.Content, nil
}

func (p *agentComposeProvider) WriteFile(ctx context.Context, workingDir, content string) error {
	_, err := p.handler.sendComposeCmd(ctx, p.hostID, composeReqFrame{
		Action:     "write_file",
		WorkingDir: workingDir,
		Content:    content,
	})
	return err
}
