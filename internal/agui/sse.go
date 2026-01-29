package agui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
)

type SSEStream struct {
	w       http.ResponseWriter
	bw      *bufio.Writer
	flusher http.Flusher

	eventName string
}

func NewSSEStream(w http.ResponseWriter) (*SSEStream, error) {
	return NewSSEStreamWithEvent(w, "agui")
}

func NewSSEStreamWithEvent(w http.ResponseWriter, eventName string) (*SSEStream, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("response 不支持 flush")
	}

	h := w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")

	bw := bufio.NewWriterSize(w, 32*1024)
	s := &SSEStream{w: w, bw: bw, flusher: flusher, eventName: eventName}
	s.flusher.Flush()
	return s, nil
}

func (s *SSEStream) SendJSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.SendBytes(b)
}

func (s *SSEStream) SendBytes(jsonBytes []byte) error {
	eventName := s.eventName
	if eventName == "" {
		eventName = "agui"
	}
	if _, err := s.bw.WriteString("event: " + eventName + "\n"); err != nil {
		return err
	}
	if _, err := s.bw.WriteString("data: "); err != nil {
		return err
	}
	if _, err := s.bw.Write(jsonBytes); err != nil {
		return err
	}
	if _, err := s.bw.WriteString("\n\n"); err != nil {
		return err
	}
	if err := s.bw.Flush(); err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}
