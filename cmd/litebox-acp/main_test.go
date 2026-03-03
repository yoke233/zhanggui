package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestIsJSONRPCLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		line string
		want bool
	}{
		{
			name: "valid json-rpc line",
			line: `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
			want: true,
		},
		{
			name: "valid json but not rpc",
			line: `{"hello":"world"}`,
			want: false,
		},
		{
			name: "banner line",
			line: "System information.",
			want: false,
		},
		{
			name: "empty line",
			line: "   ",
			want: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := isJSONRPCLine(tc.line)
			if got != tc.want {
				t.Fatalf("isJSONRPCLine(%q) = %v, want %v", tc.line, got, tc.want)
			}
		})
	}
}

func TestFilterStdoutLines_OnlyPassJSONRPC(t *testing.T) {
	t.Parallel()

	in := strings.Join([]string{
		"System information.",
		"=> Max user address: 0x7ffffffeffff",
		`{"jsonrpc":"2.0","id":1,"result":{"ok":true}}`,
		`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s1"}}`,
		"garbage line",
		"",
	}, "\n")

	var jsonOut bytes.Buffer
	var diagOut bytes.Buffer

	if err := filterStdoutLines(strings.NewReader(in), &jsonOut, &diagOut); err != nil {
		t.Fatalf("filterStdoutLines() error = %v", err)
	}

	gotJSON := jsonOut.String()
	wantJSON := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"result":{"ok":true}}`,
		`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s1"}}`,
		"",
	}, "\n")
	if gotJSON != wantJSON {
		t.Fatalf("json output mismatch\n--- got ---\n%s\n--- want ---\n%s", gotJSON, wantJSON)
	}

	gotDiag := diagOut.String()
	if !strings.Contains(gotDiag, "[litebox-acp/drop] System information.") {
		t.Fatalf("diag output missing dropped banner line, got=%q", gotDiag)
	}
	if !strings.Contains(gotDiag, "[litebox-acp/drop] garbage line") {
		t.Fatalf("diag output missing dropped garbage line, got=%q", gotDiag)
	}
}
