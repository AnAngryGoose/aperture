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
