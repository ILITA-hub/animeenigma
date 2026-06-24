package wire

import (
	"encoding/json"
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
