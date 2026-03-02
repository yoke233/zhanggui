package acpclient

import "context"

type Handler interface {
	HandleReadFile(ctx context.Context, req ReadFileRequest) (ReadFileResult, error)
	HandleWriteFile(ctx context.Context, req WriteFileRequest) (WriteFileResult, error)
	HandleRequestPermission(ctx context.Context, req PermissionRequest) (PermissionDecision, error)
	HandleTerminalCreate(ctx context.Context, req TerminalCreateRequest) (TerminalCreateResult, error)
	HandleTerminalWrite(ctx context.Context, req TerminalWriteRequest) (TerminalWriteResult, error)
	HandleTerminalRead(ctx context.Context, req TerminalReadRequest) (TerminalReadResult, error)
	HandleTerminalResize(ctx context.Context, req TerminalResizeRequest) (TerminalResizeResult, error)
	HandleTerminalClose(ctx context.Context, req TerminalCloseRequest) (TerminalCloseResult, error)
}

type EventHandler interface {
	HandleSessionUpdate(ctx context.Context, update SessionUpdate) error
}

type NopHandler struct{}

func (h *NopHandler) HandleReadFile(context.Context, ReadFileRequest) (ReadFileResult, error) {
	return ReadFileResult{}, nil
}

func (h *NopHandler) HandleWriteFile(context.Context, WriteFileRequest) (WriteFileResult, error) {
	return WriteFileResult{}, nil
}

func (h *NopHandler) HandleRequestPermission(context.Context, PermissionRequest) (PermissionDecision, error) {
	return PermissionDecision{}, nil
}

func (h *NopHandler) HandleTerminalCreate(context.Context, TerminalCreateRequest) (TerminalCreateResult, error) {
	return TerminalCreateResult{}, nil
}

func (h *NopHandler) HandleTerminalWrite(context.Context, TerminalWriteRequest) (TerminalWriteResult, error) {
	return TerminalWriteResult{}, nil
}

func (h *NopHandler) HandleTerminalRead(context.Context, TerminalReadRequest) (TerminalReadResult, error) {
	return TerminalReadResult{}, nil
}

func (h *NopHandler) HandleTerminalResize(context.Context, TerminalResizeRequest) (TerminalResizeResult, error) {
	return TerminalResizeResult{}, nil
}

func (h *NopHandler) HandleTerminalClose(context.Context, TerminalCloseRequest) (TerminalCloseResult, error) {
	return TerminalCloseResult{}, nil
}

func (h *NopHandler) HandleSessionUpdate(context.Context, SessionUpdate) error {
	return nil
}
