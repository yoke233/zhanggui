package core

// DriverCapabilities defines the maximum capabilities a driver binary can support.
// Profile capabilities must be a subset of these.
type DriverCapabilities struct {
	FSRead   bool `json:"fs_read"`
	FSWrite  bool `json:"fs_write"`
	Terminal bool `json:"terminal"`
}

// Covers returns true if max covers the requested capability set.
func (dc DriverCapabilities) Covers(req DriverCapabilities) bool {
	if req.FSRead && !dc.FSRead {
		return false
	}
	if req.FSWrite && !dc.FSWrite {
		return false
	}
	if req.Terminal && !dc.Terminal {
		return false
	}
	return true
}
