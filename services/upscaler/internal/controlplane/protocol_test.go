package controlplane

import (
	"encoding/json"
	"testing"
)

// --------------------------------------------------------------------------
// NewFrame + Decode round-trips
// --------------------------------------------------------------------------

func TestNewFrame_Register_RoundTrip(t *testing.T) {
	want := RegisterPayload{
		WorkerID:        "w-1",
		GPUInfo:         "RTX 4090",
		ImageVersion:    "v1.2.3",
		ModelsAvailable: []string{"realesrgan-x4plus"},
	}
	f, err := NewFrame("register", 1, want)
	if err != nil {
		t.Fatalf("NewFrame: %v", err)
	}
	if f.Type != "register" {
		t.Errorf("Type = %q, want %q", f.Type, "register")
	}
	if f.Seq != 1 {
		t.Errorf("Seq = %d, want 1", f.Seq)
	}

	var got RegisterPayload
	if err := f.Decode(&got); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.WorkerID != want.WorkerID {
		t.Errorf("WorkerID = %q, want %q", got.WorkerID, want.WorkerID)
	}
	if got.GPUInfo != want.GPUInfo {
		t.Errorf("GPUInfo = %q, want %q", got.GPUInfo, want.GPUInfo)
	}
	if len(got.ModelsAvailable) != len(want.ModelsAvailable) {
		t.Errorf("ModelsAvailable len = %d, want %d", len(got.ModelsAvailable), len(want.ModelsAvailable))
	}
}

func TestNewFrame_Command_RoundTrip(t *testing.T) {
	args, _ := json.Marshal(map[string]string{"key": "val"})
	want := CommandPayload{Cmd: "start", Args: args}
	f, err := NewFrame("command", 2, want)
	if err != nil {
		t.Fatalf("NewFrame: %v", err)
	}
	var got CommandPayload
	if err := f.Decode(&got); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Cmd != "start" {
		t.Errorf("Cmd = %q, want %q", got.Cmd, "start")
	}
}

func TestNewFrame_Heartbeat_RoundTrip(t *testing.T) {
	want := HeartbeatPayload{JobID: "job-42", SegmentIdx: 3, ProgressPct: 50, ETASeconds: 120}
	f, err := NewFrame("heartbeat", 3, want)
	if err != nil {
		t.Fatalf("NewFrame: %v", err)
	}
	var got HeartbeatPayload
	if err := f.Decode(&got); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.JobID != "job-42" || got.SegmentIdx != 3 || got.ProgressPct != 50 || got.ETASeconds != 120 {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestNewFrame_Metrics_RoundTrip(t *testing.T) {
	want := MetricsPayload{
		GPUModel: "RTX 4090", ImageVersion: "v2",
		GPUUtil: 95.5, VRAMUsedBytes: 1e9, VRAMTotalBytes: 24e9,
		GPUTempC: 72.3, GPUPowerW: 350, DecodeFPS: 120, InferenceFPS: 30, EncodeFPS: 60,
	}
	f, err := NewFrame("metrics", 4, want)
	if err != nil {
		t.Fatalf("NewFrame: %v", err)
	}
	var got MetricsPayload
	if err := f.Decode(&got); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.GPUUtil != 95.5 {
		t.Errorf("GPUUtil = %v, want 95.5", got.GPUUtil)
	}
	if got.InferenceFPS != 30 {
		t.Errorf("InferenceFPS = %v, want 30", got.InferenceFPS)
	}
}

func TestNewFrame_LeaseReq_RoundTrip(t *testing.T) {
	f, err := NewFrame("lease_req", 5, LeaseReqPayload{})
	if err != nil {
		t.Fatalf("NewFrame: %v", err)
	}
	var got LeaseReqPayload
	if err := f.Decode(&got); err != nil {
		t.Fatalf("Decode: %v", err)
	}
}

func TestNewFrame_LeaseGrant_RoundTrip(t *testing.T) {
	want := LeaseGrantPayload{
		JobID: "job-1",
		Idx:   7,
		Handles: LeaseHandles{
			GetHandle: "h-get", GetExp: "1234567890", GetSig: "abc123",
			PutHandle: "h-put", PutExp: "9876543210", PutSig: "def456",
		},
	}
	f, err := NewFrame("lease_grant", 6, want)
	if err != nil {
		t.Fatalf("NewFrame: %v", err)
	}
	var got LeaseGrantPayload
	if err := f.Decode(&got); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Idx != 7 {
		t.Errorf("Idx = %d, want 7", got.Idx)
	}
	if got.Handles.GetSig != "abc123" {
		t.Errorf("GetSig = %q, want %q", got.Handles.GetSig, "abc123")
	}
}

func TestNewFrame_Exec_RoundTrip(t *testing.T) {
	exitCode := 0
	want := ExecPayload{
		SessionID: "sess-1",
		Data:      []byte("hello"),
		Cols:      80,
		Rows:      24,
		ExitCode:  &exitCode,
	}
	f, err := NewFrame("exec_data", 7, want)
	if err != nil {
		t.Fatalf("NewFrame: %v", err)
	}
	var got ExecPayload
	if err := f.Decode(&got); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if string(got.Data) != "hello" {
		t.Errorf("Data = %q, want %q", got.Data, "hello")
	}
	if got.ExitCode == nil || *got.ExitCode != 0 {
		t.Errorf("ExitCode = %v, want *0", got.ExitCode)
	}
}

// --------------------------------------------------------------------------
// IsValidType
// --------------------------------------------------------------------------

func TestIsValidType_AcceptsAllTenTypes(t *testing.T) {
	cases := []string{
		"register", "command", "log", "heartbeat", "metrics",
		"exec_open", "exec_data", "exec_close", "lease_req", "lease_grant",
	}
	for _, typ := range cases {
		if !IsValidType(typ) {
			t.Errorf("IsValidType(%q) = false, want true", typ)
		}
	}
}

func TestIsValidType_RejectsUnknownTypes(t *testing.T) {
	unknown := []string{"", "unknown", "REGISTER", "Register", "ping", "pong", "error"}
	for _, typ := range unknown {
		if IsValidType(typ) {
			t.Errorf("IsValidType(%q) = true, want false", typ)
		}
	}
}

// --------------------------------------------------------------------------
// Frame.Decode failure on type mismatch
// --------------------------------------------------------------------------

func TestFrame_Decode_FailsGracefullyOnTypeMismatch(t *testing.T) {
	// Build a HeartbeatPayload frame, then try to decode it into MetricsPayload.
	// JSON unmarshal won't error (numeric fields just get zero values) but the
	// key test is that Decode does NOT panic and returns a usable (zero) struct.
	hp := HeartbeatPayload{JobID: "j", SegmentIdx: 1, ProgressPct: 50, ETASeconds: 10}
	f, err := NewFrame("heartbeat", 1, hp)
	if err != nil {
		t.Fatalf("NewFrame: %v", err)
	}

	// Decode into wrong type — should succeed (JSON is lenient) without panic.
	var mp MetricsPayload
	if err := f.Decode(&mp); err != nil {
		// A decode error here is also acceptable for strict implementations.
		t.Logf("Decode into wrong type returned error (ok): %v", err)
	}
	// No panic means the test passed.

	// Explicitly broken JSON payload.
	f2 := Frame{Type: "heartbeat", Seq: 2, Payload: json.RawMessage(`{broken`)}
	var hp2 HeartbeatPayload
	if err := f2.Decode(&hp2); err == nil {
		t.Error("Decode on malformed JSON should return an error")
	}
}

// --------------------------------------------------------------------------
// NewFrame error path (un-marshallable payload)
// --------------------------------------------------------------------------

func TestNewFrame_MarshallError(t *testing.T) {
	// channels cannot be marshalled to JSON.
	ch := make(chan int)
	_, err := NewFrame("log", 1, ch)
	if err == nil {
		t.Error("NewFrame with un-marshallable payload should return error")
	}
}
