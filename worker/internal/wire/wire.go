// Package wire contains the worker-local copies of the control-plane protocol
// types and helpers. These mirror the JSON shapes defined in
// services/upscaler/internal/controlplane/protocol.go but live in the worker
// module so the worker binary has zero import dependency on the server packages.
package wire

import "encoding/json"

// Frame is the top-level WebSocket envelope. Every message sent or received on
// the control-plane connection is a JSON-encoded Frame.
type Frame struct {
	Type    string          `json:"type"`
	Seq     int             `json:"seq"`
	Payload json.RawMessage `json:"payload"`
}

// NewFrame marshals payload into a json.RawMessage and wraps it in a Frame.
func NewFrame(typ string, seq int, payload any) (Frame, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return Frame{}, err
	}
	return Frame{Type: typ, Seq: seq, Payload: raw}, nil
}

// Decode unmarshals the frame's Payload into v.
func (f Frame) Decode(v any) error {
	return json.Unmarshal(f.Payload, v)
}

// RegisterPayload is sent by a worker immediately after the WebSocket
// connection is established (type="register").
type RegisterPayload struct {
	WorkerID        string   `json:"worker_id"`
	GPUInfo         string   `json:"gpu_info"`
	ImageVersion    string   `json:"image_version"`
	ModelsAvailable []string `json:"models_available"`
}

// EnrollRequest is the payload sent by a worker at POST /worker/enroll.
type EnrollRequest struct {
	Token string `json:"token"`
}

// EnrollResponse is returned by the server on successful enrollment.
type EnrollResponse struct {
	WorkerID string `json:"worker_id"`
	Handle   string `json:"handle"`
	Exp      string `json:"exp"`
	Sig      string `json:"sig"`
}

// CommandPayload is sent by the server to issue a command to a worker
// (type="command").
type CommandPayload struct {
	Cmd  string          `json:"cmd"`
	Args json.RawMessage `json:"args"`
}

// HeartbeatPayload is sent by the worker to report progress on the current
// segment (type="heartbeat").
type HeartbeatPayload struct {
	JobID       string `json:"job_id"`
	SegmentIdx  int    `json:"segment_idx"`
	ProgressPct int    `json:"progress_pct"`
	ETASeconds  int    `json:"eta_seconds"`
}

// LeaseReqPayload is sent by the worker to request the next segment lease
// (type="lease_req"). It carries no fields.
type LeaseReqPayload struct{}

// LeaseHandles contains the pre-signed MinIO handles for fetching the input
// segment and putting the output segment.
type LeaseHandles struct {
	GetHandle string `json:"get_handle"`
	GetExp    string `json:"get_exp"`
	GetSig    string `json:"get_sig"`
	PutHandle string `json:"put_handle"`
	PutExp    string `json:"put_exp"`
	PutSig    string `json:"put_sig"`
}

// ModelHandle carries a short-lived HMAC capability the worker uses to fetch
// a model binary from the server (GET {SERVER_URL}/worker/models/{Model}).
// Nil/omitted when Model is "mock" or empty (built-in; never fetched).
type ModelHandle struct {
	Exp string `json:"exp"`
	Sig string `json:"sig"`
}

// LeaseGrantPayload is sent by the server in response to a lease_req
// (type="lease_grant").
type LeaseGrantPayload struct {
	JobID       string       `json:"job_id"`
	Idx         int          `json:"idx"`
	Handles     LeaseHandles `json:"handles"`
	Model       string       `json:"model"`
	Scale       int          `json:"scale"`
	ModelHandle *ModelHandle `json:"model_handle,omitempty"`
}

// MetricsPayload is sent by the worker to report GPU and processing metrics.
type MetricsPayload struct {
	GPUModel       string  `json:"gpu_model"`
	ImageVersion   string  `json:"image_version"`
	GPUUtil        float64 `json:"gpu_util"`
	VRAMUsedBytes  float64 `json:"vram_used_bytes"`
	VRAMTotalBytes float64 `json:"vram_total_bytes"`
	GPUTempC       float64 `json:"gpu_temp_c"`
	GPUPowerW      float64 `json:"gpu_power_w"`
	DecodeFPS      float64 `json:"decode_fps"`
	InferenceFPS   float64 `json:"inference_fps"`
	EncodeFPS      float64 `json:"encode_fps"`
}

// ExecPayload is used for exec_open/exec_data/exec_close frames.
type ExecPayload struct {
	SessionID string `json:"session_id"`
	Data      []byte `json:"data"`
	Cols      int    `json:"cols"`
	Rows      int    `json:"rows"`
	ExitCode  *int   `json:"exit_code,omitempty"`
	Pty       bool   `json:"pty,omitempty"`
}
