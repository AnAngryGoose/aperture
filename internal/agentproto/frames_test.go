package agentproto

import (
	"encoding/json"
	"testing"
)

func TestTerminalFramesEncodeAsObjects(t *testing.T) {
	tests := []struct {
		name string
		v    any
	}{
		{"start", TerminalReqFrame{Type: TypeTerminalReq, ReqID: "r1", Action: "start", CID: "c1", Cmd: "/bin/sh"}},
		{"resize", TerminalReqFrame{Type: TypeTerminalReq, ReqID: "r1", Action: "resize", Cols: 120, Rows: 40}},
		{"close", TerminalReqFrame{Type: TypeTerminalReq, ReqID: "r1", Action: "close"}},
		{"data", TerminalDataFrame{Type: TypeTerminalData, ReqID: "r1", Data: []byte("hello")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := json.Marshal(tt.v)
			if err != nil {
				t.Fatal(err)
			}
			var obj map[string]any
			if err := json.Unmarshal(b, &obj); err != nil {
				t.Fatalf("frame did not encode as object: %s", b)
			}
			if obj["type"] == nil || obj["req_id"] == nil {
				t.Fatalf("missing expected fields: %s", b)
			}
		})
	}
}
