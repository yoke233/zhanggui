package web

import (
	"encoding/json"
	"fmt"
	"net/http"
)

const (
	a2aJSONRPCVersion      = "2.0"
	a2aRPCMethodNotFound   = -32601
	a2aRPCInvalidRequest   = -32600
	a2aMethodMessageSend   = "message/send"
	a2aMethodMessageStream = "message/stream"
	a2aMethodTasksGet      = "tasks/get"
	a2aMethodTasksCancel   = "tasks/cancel"
)

type a2aRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type a2aRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type a2aRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *a2aRPCError    `json:"error,omitempty"`
}

func decodeA2ARPCRequest(r *http.Request) (a2aRPCRequest, error) {
	var req a2aRPCRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&req); err != nil {
		return a2aRPCRequest{}, fmt.Errorf("decode a2a rpc request: %w", err)
	}
	return req, nil
}

func writeA2ARPCError(w http.ResponseWriter, id json.RawMessage, code int, message string) {
	resp := a2aRPCResponse{
		JSONRPC: a2aJSONRPCVersion,
		ID:      id,
		Error: &a2aRPCError{
			Code:    code,
			Message: message,
		},
	}
	writeJSON(w, http.StatusOK, resp)
}
