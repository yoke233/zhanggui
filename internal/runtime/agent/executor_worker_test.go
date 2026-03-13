package agentruntime

import (
	"context"
	"errors"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
)

type fakeInvocationResultPublisher struct {
	err error
}

func (f fakeInvocationResultPublisher) Publish(context.Context, string, []byte, ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &jetstream.PubAck{}, nil
}

type fakeInvocationResultMessage struct {
	acked bool
	naked bool
}

func (m *fakeInvocationResultMessage) Ack() error {
	m.acked = true
	return nil
}

func (m *fakeInvocationResultMessage) Nak() error {
	m.naked = true
	return nil
}

func TestPublishInvocationResultAcksAfterSuccessfulPublish(t *testing.T) {
	t.Parallel()

	msg := &fakeInvocationResultMessage{}
	if err := publishInvocationResult(fakeInvocationResultPublisher{}, msg, "subject", []byte("ok")); err != nil {
		t.Fatalf("publishInvocationResult() error = %v", err)
	}
	if !msg.acked {
		t.Fatal("expected message to be acked after successful publish")
	}
	if msg.naked {
		t.Fatal("message should not be nacked after successful publish")
	}
}

func TestPublishInvocationResultNaksWhenPublishFails(t *testing.T) {
	t.Parallel()

	msg := &fakeInvocationResultMessage{}
	err := publishInvocationResult(fakeInvocationResultPublisher{err: errors.New("boom")}, msg, "subject", []byte("fail"))
	if err == nil {
		t.Fatal("expected publishInvocationResult() to return publish error")
	}
	if msg.acked {
		t.Fatal("message should not be acked when publish fails")
	}
	if !msg.naked {
		t.Fatal("expected message to be nacked when publish fails")
	}
}
