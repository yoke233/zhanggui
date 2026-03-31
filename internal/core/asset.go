package core

// Asset preserves the legacy inline run result asset shape used by SQLite
// result_assets data. New flows should prefer Resource records.
type Asset struct {
	Name     string         `json:"name,omitempty"`
	URI      string         `json:"uri,omitempty"`
	MimeType string         `json:"mime_type,omitempty"`
	Role     string         `json:"role,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}
