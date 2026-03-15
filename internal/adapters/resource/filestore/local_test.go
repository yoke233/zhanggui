package filestore

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestLocalFileStoreSaveOpenDelete(t *testing.T) {
	ctx := context.Background()
	store := NewLocal(t.TempDir())

	uri, size, err := store.Save(ctx, "demo.txt", strings.NewReader("hello file store"))
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if size != int64(len("hello file store")) {
		t.Fatalf("unexpected size: %d", size)
	}
	if uri == "" {
		t.Fatal("expected uri")
	}

	rc, err := store.Open(ctx, uri)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer rc.Close()
	body, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(body) != "hello file store" {
		t.Fatalf("unexpected content: %q", string(body))
	}
	if err := rc.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if err := store.Delete(ctx, uri); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := store.Open(ctx, uri); err == nil {
		t.Fatal("expected open to fail after delete")
	}
}
