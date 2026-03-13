package executor

import (
	"context"

	"github.com/yoke233/ai-workflow/internal/adapters/agent/acpclient"
	runtimeapp "github.com/yoke233/ai-workflow/internal/application/runtime"
)

type multiSink struct {
	sinks []runtimeapp.EventSink
}

func newMultiSink(sinks ...runtimeapp.EventSink) runtimeapp.EventSink {
	filtered := make([]runtimeapp.EventSink, 0, len(sinks))
	for _, sink := range sinks {
		if sink != nil {
			filtered = append(filtered, sink)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	if len(filtered) == 1 {
		return filtered[0]
	}
	return &multiSink{sinks: filtered}
}

func (m *multiSink) HandleSessionUpdate(ctx context.Context, update acpclient.SessionUpdate) error {
	for _, sink := range m.sinks {
		if sink != nil {
			_ = sink.HandleSessionUpdate(ctx, update)
		}
	}
	return nil
}
