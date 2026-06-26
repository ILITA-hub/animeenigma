package controlplane

import (
	"encoding/json"
	"reflect"
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
// Frame.Decode failure paths
// --------------------------------------------------------------------------

// TestFrame_Decode_FailsOnTypeMismatch asserts a real decode error when a
// field's JSON type does not match the target Go type (string vs int).
func TestFrame_Decode_FailsOnTypeMismatch(t *testing.T) {
	// "seq" is a string in the payload but HeartbeatPayload.SegmentIdx is an int.
	f := Frame{Type: "heartbeat", Seq: 1, Payload: json.RawMessage(`{"segment_idx": "not-an-int"}`)}
	var hp HeartbeatPayload
	if err := f.Decode(&hp); err == nil {
		t.Error("Decode of a string into an int field should return an error")
	}

	// Explicitly broken JSON payload also errors.
	f2 := Frame{Type: "heartbeat", Seq: 2, Payload: json.RawMessage(`{broken`)}
	var hp2 HeartbeatPayload
	if err := f2.Decode(&hp2); err == nil {
		t.Error("Decode on malformed JSON should return an error")
	}
}

// TestFrame_Decode_NilPayload asserts a clean error (not a panic) when the
// Frame has a nil/empty Payload — json.Unmarshal of nil returns "unexpected
// end of JSON input".
func TestFrame_Decode_NilPayload(t *testing.T) {
	var f Frame // zero value: Payload is nil
	var hp HeartbeatPayload
	if err := f.Decode(&hp); err == nil {
		t.Error("Decode of a nil payload should return an error, got nil")
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

// --------------------------------------------------------------------------
// LeaseGrantPayload + ModelHandle parity guards
// --------------------------------------------------------------------------

// TestLeaseGrantPayload_WireParityWithWorker asserts that the server-side
// LeaseGrantPayload and ModelHandle structs have exactly the JSON field tags
// that the worker/internal/wire package mirrors. Tag drift between the two
// would silently break deserialization on the worker side. The canonical
// tags are duplicated here so a future change to either side fails fast.
func TestLeaseGrantPayload_WireParityWithWorker(t *testing.T) {
	t.Parallel()

	// Canonical json tags — must match worker/internal/wire/wire.go exactly.
	wantGrantTags := map[string]string{
		"JobID":       "job_id",
		"Idx":         "idx",
		"Handles":     "handles",
		"Model":       "model",
		"Scale":       "scale",
		"ModelHandle": "model_handle,omitempty",
	}

	grantType := reflect.TypeOf(LeaseGrantPayload{})
	for i := 0; i < grantType.NumField(); i++ {
		f := grantType.Field(i)
		want, ok := wantGrantTags[f.Name]
		if !ok {
			t.Errorf("LeaseGrantPayload: unexpected field %q — update parity test", f.Name)
			continue
		}
		got := f.Tag.Get("json")
		if got != want {
			t.Errorf("LeaseGrantPayload.%s json tag = %q, want %q (worker wire tag)", f.Name, got, want)
		}
		delete(wantGrantTags, f.Name)
	}
	for missing := range wantGrantTags {
		t.Errorf("LeaseGrantPayload: missing expected field %q", missing)
	}

	wantHandleTags := map[string]string{
		"Exp": "exp",
		"Sig": "sig",
	}
	handleType := reflect.TypeOf(ModelHandle{})
	for i := 0; i < handleType.NumField(); i++ {
		f := handleType.Field(i)
		want, ok := wantHandleTags[f.Name]
		if !ok {
			t.Errorf("ModelHandle: unexpected field %q — update parity test", f.Name)
			continue
		}
		got := f.Tag.Get("json")
		if got != want {
			t.Errorf("ModelHandle.%s json tag = %q, want %q (worker wire tag)", f.Name, got, want)
		}
		delete(wantHandleTags, f.Name)
	}
	for missing := range wantHandleTags {
		t.Errorf("ModelHandle: missing expected field %q", missing)
	}
}

// TestNewFrame_LeaseGrant_WithModelHandle verifies LeaseGrantPayload round-trips
// with Model, Scale, and ModelHandle populated, and omits model_handle when nil.
func TestNewFrame_LeaseGrant_WithModelHandle(t *testing.T) {
	t.Run("with model handle", func(t *testing.T) {
		want := LeaseGrantPayload{
			JobID: "job-model-1",
			Idx:   2,
			Handles: LeaseHandles{
				GetHandle: "h-get", GetExp: "1234567890", GetSig: "abc123",
				PutHandle: "h-put", PutExp: "9876543210", PutSig: "def456",
			},
			Model: "realesrgan-x4plus-anime",
			Scale: 4,
			ModelHandle: &ModelHandle{
				Exp: "9999999999",
				Sig: "cafebabe00000000000000000000beef",
			},
		}
		f, err := NewFrame("lease_grant", 10, want)
		if err != nil {
			t.Fatalf("NewFrame: %v", err)
		}
		var got LeaseGrantPayload
		if err := f.Decode(&got); err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if got.Model != want.Model {
			t.Errorf("Model = %q, want %q", got.Model, want.Model)
		}
		if got.Scale != want.Scale {
			t.Errorf("Scale = %d, want %d", got.Scale, want.Scale)
		}
		if got.ModelHandle == nil {
			t.Fatal("ModelHandle = nil, want non-nil")
		}
		if got.ModelHandle.Exp != want.ModelHandle.Exp {
			t.Errorf("ModelHandle.Exp = %q, want %q", got.ModelHandle.Exp, want.ModelHandle.Exp)
		}
		if got.ModelHandle.Sig != want.ModelHandle.Sig {
			t.Errorf("ModelHandle.Sig = %q, want %q", got.ModelHandle.Sig, want.ModelHandle.Sig)
		}
	})

	t.Run("mock model — model_handle omitted", func(t *testing.T) {
		want := LeaseGrantPayload{
			JobID:       "job-mock-1",
			Idx:         0,
			Model:       "mock",
			Scale:       2,
			ModelHandle: nil,
		}
		f, err := NewFrame("lease_grant", 11, want)
		if err != nil {
			t.Fatalf("NewFrame: %v", err)
		}
		// Confirm model_handle key absent from JSON.
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(f.Payload, &raw); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		if _, present := raw["model_handle"]; present {
			t.Error("model_handle key present in JSON when ModelHandle is nil (omitempty not working)")
		}
		var got LeaseGrantPayload
		if err := f.Decode(&got); err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if got.ModelHandle != nil {
			t.Errorf("ModelHandle = %+v, want nil", got.ModelHandle)
		}
	})
}
