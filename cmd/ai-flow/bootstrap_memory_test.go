package main

import (
	"testing"

	storesqlite "github.com/yoke233/ai-workflow/internal/plugins/store-sqlite"
)

func TestMemoryFromStore_SQLiteStore(t *testing.T) {
	store, err := storesqlite.New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer store.Close()

	memory := memoryFromStore(store)
	if memory == nil {
		t.Fatal("memoryFromStore() returned nil for SQLiteStore")
	}
	if _, ok := memory.(*storesqlite.SQLiteMemory); !ok {
		t.Fatalf("memoryFromStore() type = %T, want *storesqlite.SQLiteMemory", memory)
	}
}

func TestMemoryFromStore_NilStore(t *testing.T) {
	if memory := memoryFromStore(nil); memory != nil {
		t.Fatalf("memoryFromStore(nil) = %T, want nil", memory)
	}
}
