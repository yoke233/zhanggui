package webassets

import (
	"io/fs"
	"strings"
	"testing"
)

func TestEmbeddedFrontendMode_DefaultFallback(t *testing.T) {
	if got := EmbeddedFrontendMode(); got != "fallback" {
		t.Fatalf("default embedded frontend mode mismatch, got %q want %q", got, "fallback")
	}
}

func TestDistFS_ProvidesUsableSPAAssets(t *testing.T) {
	dist, err := DistFS()
	if err != nil {
		t.Fatalf("DistFS() error = %v, want nil", err)
	}

	indexContent, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	indexBody := string(indexContent)
	if !strings.Contains(indexBody, `<div id="root"></div>`) {
		t.Fatalf("index.html should contain SPA root node, got: %s", indexBody)
	}
	if !strings.Contains(indexBody, "/assets/main.js") {
		t.Fatalf("index.html should reference fallback asset main.js, got: %s", indexBody)
	}

	jsContent, err := fs.ReadFile(dist, "assets/main.js")
	if err != nil {
		t.Fatalf("read assets/main.js: %v", err)
	}
	if len(strings.TrimSpace(string(jsContent))) == 0 {
		t.Fatal("assets/main.js should not be empty")
	}
}
