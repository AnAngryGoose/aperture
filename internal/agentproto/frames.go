package agentproto

import (
	"encoding/json"

	"github.com/aperture/aperture/internal/types"
)

const (
	TypeHello        = "hello"
	TypeAck          = "ack"
	TypeMetric       = "metric"
	TypeHeartbeat    = "heartbeat"
	TypeDockerReq    = "docker_req"
	TypeDockerResp   = "docker_resp"
	TypeComposeReq   = "compose_req"
	TypeComposeResp  = "compose_resp"
	TypeTerminalReq  = "terminal_req"
	TypeTerminalResp = "terminal_resp"
	TypeTerminalData = "terminal_data"
	// TypeConfig pushes per-host monitoring policy (sample interval, enabled
	// families, filters, mem_calc) from hub to agent. Sent on every
	// host_config PUT and once at agent connect after the hello/ack exchange.
	TypeConfig = "config"
)

type Frame struct {
	Type string `json:"type"`
}

type HelloFrame struct {
	Type       string         `json:"type"`
	Host       types.HostInfo `json:"host"`
	Version    string         `json:"version"`
	HasDocker  bool           `json:"has_docker"`
	HasCompose bool           `json:"has_compose"`
}

type AckFrame struct {
	Type   string `json:"type"`
	HostID string `json:"host_id"`
}

type MetricFrame struct {
	Type string             `json:"type"`
	Data types.MetricSample `json:"data"`
}

type HeartbeatFrame struct {
	Type string `json:"type"`
}

type DockerReqFrame struct {
	Type   string          `json:"type"`
	ReqID  string          `json:"req_id"`
	Action string          `json:"action"`
	CID    string          `json:"cid,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
}

type DockerRespFrame struct {
	Type  string          `json:"type"`
	ReqID string          `json:"req_id"`
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}

type ComposeReqFrame struct {
	Type       string   `json:"type"`
	ReqID      string   `json:"req_id"`
	Action     string   `json:"action"`
	Project    string   `json:"project,omitempty"`
	WorkingDir string   `json:"working_dir,omitempty"`
	SubAction  string   `json:"sub_action,omitempty"`
	Service    string   `json:"service,omitempty"`
	ExtraArgs  []string `json:"extra_args,omitempty"`
	Content    string   `json:"content,omitempty"`
	Tail       int      `json:"tail,omitempty"`
}

type ComposeRespFrame struct {
	Type  string          `json:"type"`
	ReqID string          `json:"req_id"`
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}

type TerminalReqFrame struct {
	Type   string `json:"type"`
	ReqID  string `json:"req_id"`
	Action string `json:"action"`
	CID    string `json:"cid,omitempty"`
	Cmd    string `json:"cmd,omitempty"`
	Cols   uint   `json:"cols,omitempty"`
	Rows   uint   `json:"rows,omitempty"`
}

type TerminalRespFrame struct {
	Type  string `json:"type"`
	ReqID string `json:"req_id"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type TerminalDataFrame struct {
	Type  string `json:"type"`
	ReqID string `json:"req_id"`
	Data  []byte `json:"data"`
}

// ConfigFrame pushes a per-host monitoring policy from hub to agent.
// Mirrors types.HostConfig but only the fields the agent acts on (numeric
// thresholds are evaluated server-side by the hub, not by the agent).
type ConfigFrame struct {
	Type            string                  `json:"type"`
	SampleIntervalS int                     `json:"sample_interval_s"`
	EnabledFamilies []string                `json:"enabled_families"`
	Filters         types.HostConfigFilters `json:"filters"`
	MemCalc         string                  `json:"mem_calc"`
}
