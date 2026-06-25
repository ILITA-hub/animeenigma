package wire

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestMetricsPayloadRoundTrip(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   MetricsPayload
	}{
		{
			name: "all fields set",
			in: MetricsPayload{
				GPUModel:       "NVIDIA GeForce RTX 3090",
				ImageVersion:   "v1.2.3",
				GPUUtil:        72.5,
				VRAMUsedBytes:  float64(8192) * 1024 * 1024,
				VRAMTotalBytes: float64(24576) * 1024 * 1024,
				GPUTempC:       71.0,
				GPUPowerW:      250.0,
				DecodeFPS:      30.5,
				InferenceFPS:   15.2,
				EncodeFPS:      28.7,
			},
		},
		{
			name: "zero values",
			in:   MetricsPayload{},
		},
		{
			name: "partial fields",
			in: MetricsPayload{
				GPUModel:  "AMD Radeon RX 7900 XTX",
				GPUUtil:   99.9,
				GPUTempC:  85.0,
				GPUPowerW: 355.0,
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b, err := json.Marshal(tc.in)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			var got MetricsPayload
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			if got.GPUModel != tc.in.GPUModel {
				t.Errorf("GPUModel: got %q, want %q", got.GPUModel, tc.in.GPUModel)
			}
			if got.ImageVersion != tc.in.ImageVersion {
				t.Errorf("ImageVersion: got %q, want %q", got.ImageVersion, tc.in.ImageVersion)
			}
			if got.GPUUtil != tc.in.GPUUtil {
				t.Errorf("GPUUtil: got %v, want %v", got.GPUUtil, tc.in.GPUUtil)
			}
			if got.VRAMUsedBytes != tc.in.VRAMUsedBytes {
				t.Errorf("VRAMUsedBytes: got %v, want %v", got.VRAMUsedBytes, tc.in.VRAMUsedBytes)
			}
			if got.VRAMTotalBytes != tc.in.VRAMTotalBytes {
				t.Errorf("VRAMTotalBytes: got %v, want %v", got.VRAMTotalBytes, tc.in.VRAMTotalBytes)
			}
			if got.GPUTempC != tc.in.GPUTempC {
				t.Errorf("GPUTempC: got %v, want %v", got.GPUTempC, tc.in.GPUTempC)
			}
			if got.GPUPowerW != tc.in.GPUPowerW {
				t.Errorf("GPUPowerW: got %v, want %v", got.GPUPowerW, tc.in.GPUPowerW)
			}
			if got.DecodeFPS != tc.in.DecodeFPS {
				t.Errorf("DecodeFPS: got %v, want %v", got.DecodeFPS, tc.in.DecodeFPS)
			}
			if got.InferenceFPS != tc.in.InferenceFPS {
				t.Errorf("InferenceFPS: got %v, want %v", got.InferenceFPS, tc.in.InferenceFPS)
			}
			if got.EncodeFPS != tc.in.EncodeFPS {
				t.Errorf("EncodeFPS: got %v, want %v", got.EncodeFPS, tc.in.EncodeFPS)
			}
		})
	}
}

func TestExecPayloadRoundTrip(t *testing.T) {
	t.Parallel()

	exitCode := 42

	cases := []struct {
		name string
		in   ExecPayload
	}{
		{
			name: "pty open no exit code",
			in: ExecPayload{
				SessionID: "session-abc",
				Data:      []byte("ls -la /tmp"),
				Cols:      80,
				Rows:      24,
				Pty:       true,
			},
		},
		{
			name: "close with exit code",
			in: ExecPayload{
				SessionID: "session-xyz",
				Data:      []byte("output data"),
				ExitCode:  &exitCode,
			},
		},
		{
			name: "zero values",
			in:   ExecPayload{},
		},
		{
			name: "binary data",
			in: ExecPayload{
				SessionID: "s1",
				Data:      []byte{0x00, 0x1b, 0x5b, 0x41, 0xff},
				Cols:      132,
				Rows:      50,
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b, err := json.Marshal(tc.in)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			var got ExecPayload
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			if got.SessionID != tc.in.SessionID {
				t.Errorf("SessionID: got %q, want %q", got.SessionID, tc.in.SessionID)
			}
			if string(got.Data) != string(tc.in.Data) {
				t.Errorf("Data: got %v, want %v", got.Data, tc.in.Data)
			}
			if got.Cols != tc.in.Cols {
				t.Errorf("Cols: got %d, want %d", got.Cols, tc.in.Cols)
			}
			if got.Rows != tc.in.Rows {
				t.Errorf("Rows: got %d, want %d", got.Rows, tc.in.Rows)
			}
			if got.Pty != tc.in.Pty {
				t.Errorf("Pty: got %v, want %v", got.Pty, tc.in.Pty)
			}
			// ExitCode pointer comparison
			if tc.in.ExitCode == nil && got.ExitCode != nil {
				t.Errorf("ExitCode: got %v, want nil", *got.ExitCode)
			} else if tc.in.ExitCode != nil && got.ExitCode == nil {
				t.Errorf("ExitCode: got nil, want %d", *tc.in.ExitCode)
			} else if tc.in.ExitCode != nil && got.ExitCode != nil && *got.ExitCode != *tc.in.ExitCode {
				t.Errorf("ExitCode: got %d, want %d", *got.ExitCode, *tc.in.ExitCode)
			}
		})
	}
}

// TestLeaseGrantPayload_WireParityWithServer asserts that LeaseGrantPayload and
// ModelHandle in this worker package have exactly the same JSON field names (tags)
// as the server-side controlplane.LeaseGrantPayload / controlplane.ModelHandle in
// services/upscaler/internal/controlplane/protocol.go. The worker and server are
// separate modules; this test guards against silent tag drift.
//
// Canonical server-side tags (kept byte-identical across the two files):
//
//	LeaseGrantPayload:  job_id, idx, handles, model, scale, model_handle (omitempty)
//	ModelHandle:        exp, sig
func TestLeaseGrantPayload_WireParityWithServer(t *testing.T) {
	t.Parallel()

	// -- LeaseGrantPayload field → expected json tag --
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
			t.Errorf("LeaseGrantPayload: unexpected field %q — update the parity test", f.Name)
			continue
		}
		got := f.Tag.Get("json")
		if got != want {
			t.Errorf("LeaseGrantPayload.%s json tag = %q, want %q (server tag)", f.Name, got, want)
		}
		delete(wantGrantTags, f.Name)
	}
	for missing := range wantGrantTags {
		t.Errorf("LeaseGrantPayload: missing expected field %q", missing)
	}

	// -- ModelHandle field → expected json tag --
	wantHandleTags := map[string]string{
		"Exp": "exp",
		"Sig": "sig",
	}

	handleType := reflect.TypeOf(ModelHandle{})
	for i := 0; i < handleType.NumField(); i++ {
		f := handleType.Field(i)
		want, ok := wantHandleTags[f.Name]
		if !ok {
			t.Errorf("ModelHandle: unexpected field %q — update the parity test", f.Name)
			continue
		}
		got := f.Tag.Get("json")
		if got != want {
			t.Errorf("ModelHandle.%s json tag = %q, want %q (server tag)", f.Name, got, want)
		}
		delete(wantHandleTags, f.Name)
	}
	for missing := range wantHandleTags {
		t.Errorf("ModelHandle: missing expected field %q", missing)
	}
}

// TestLeaseGrantPayload_ModelHandleRoundTrip exercises JSON marshal/unmarshal
// of LeaseGrantPayload with ModelHandle set and omitted.
func TestLeaseGrantPayload_ModelHandleRoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("with model handle", func(t *testing.T) {
		t.Parallel()
		in := LeaseGrantPayload{
			JobID: "job-abc",
			Idx:   3,
			Handles: LeaseHandles{
				GetHandle: "gh", GetExp: "1000", GetSig: "gs",
				PutHandle: "ph", PutExp: "2000", PutSig: "ps",
			},
			Model: "realesrgan-x4plus-anime",
			Scale: 4,
			ModelHandle: &ModelHandle{
				Exp: "9999999999",
				Sig: "deadbeef00000000000000000000cafe",
			},
		}
		b, err := json.Marshal(in)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var got LeaseGrantPayload
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got.Model != in.Model {
			t.Errorf("Model: got %q, want %q", got.Model, in.Model)
		}
		if got.Scale != in.Scale {
			t.Errorf("Scale: got %d, want %d", got.Scale, in.Scale)
		}
		if got.ModelHandle == nil {
			t.Fatal("ModelHandle: got nil, want non-nil")
		}
		if got.ModelHandle.Exp != in.ModelHandle.Exp {
			t.Errorf("ModelHandle.Exp: got %q, want %q", got.ModelHandle.Exp, in.ModelHandle.Exp)
		}
		if got.ModelHandle.Sig != in.ModelHandle.Sig {
			t.Errorf("ModelHandle.Sig: got %q, want %q", got.ModelHandle.Sig, in.ModelHandle.Sig)
		}
	})

	t.Run("mock model — model_handle omitted", func(t *testing.T) {
		t.Parallel()
		in := LeaseGrantPayload{
			JobID: "job-mock",
			Idx:   0,
			Handles: LeaseHandles{
				GetHandle: "gh", GetExp: "1", GetSig: "gs",
				PutHandle: "ph", PutExp: "1", PutSig: "ps",
			},
			Model:       "mock",
			Scale:       2,
			ModelHandle: nil,
		}
		b, err := json.Marshal(in)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		// Verify model_handle key is absent from JSON when nil.
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(b, &raw); err != nil {
			t.Fatalf("unmarshal to map: %v", err)
		}
		if _, present := raw["model_handle"]; present {
			t.Errorf("model_handle should be omitted when nil (omitempty), but key present in JSON")
		}
		var got LeaseGrantPayload
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("unmarshal to struct: %v", err)
		}
		if got.ModelHandle != nil {
			t.Errorf("ModelHandle: got %+v, want nil", got.ModelHandle)
		}
		if got.Model != "mock" {
			t.Errorf("Model: got %q, want %q", got.Model, "mock")
		}
	})
}
