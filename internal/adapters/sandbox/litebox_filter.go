package sandbox

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const liteboxDropPrefix = "[litebox-acp/drop] "

// NewJSONRPCFilterReader returns an io.Reader that only passes through
// JSON-RPC 2.0 lines from r. Non-JSON-RPC output is written to diagW
// with a diagnostic prefix so it doesn't corrupt the transport stream.
func NewJSONRPCFilterReader(r io.Reader, diagW io.Writer) io.Reader {
	scanner := bufio.NewScanner(r)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	return &jsonrpcFilterReader{scanner: scanner, diagW: diagW}
}

type jsonrpcFilterReader struct {
	scanner *bufio.Scanner
	diagW   io.Writer
	buf     []byte
	offset  int
}

func (r *jsonrpcFilterReader) Read(p []byte) (int, error) {
	// Drain buffered data from a previous line first.
	if r.offset < len(r.buf) {
		n := copy(p, r.buf[r.offset:])
		r.offset += n
		return n, nil
	}

	for r.scanner.Scan() {
		raw := r.scanner.Text()
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		if isJSONRPCLine(trimmed) {
			r.buf = []byte(trimmed + "\n")
			r.offset = 0
			n := copy(p, r.buf)
			r.offset = n
			return n, nil
		}
		if r.diagW != nil {
			fmt.Fprintf(r.diagW, "%s%s\n", liteboxDropPrefix, raw)
		}
	}

	if err := r.scanner.Err(); err != nil {
		return 0, err
	}
	return 0, io.EOF
}

func isJSONRPCLine(line string) bool {
	var probe struct {
		JSONRPC string `json:"jsonrpc"`
	}
	if err := json.Unmarshal([]byte(line), &probe); err != nil {
		return false
	}
	return probe.JSONRPC == "2.0"
}
