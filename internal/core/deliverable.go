package core

// Asset is an attachment/output file associated with a Run result.
type Asset struct {
	Name      string `json:"name"`
	URI       string `json:"uri"`
	MediaType string `json:"media_type,omitempty"`
}
