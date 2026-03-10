package core

// AgentDriver defines how to launch an ACP agent process.
// This is the process-level configuration — shared across profiles that use the same binary.
type AgentDriver struct {
	ID              string            `json:"id"`
	LaunchCommand   string            `json:"launch_command"`
	LaunchArgs      []string          `json:"launch_args,omitempty"`
	Env             map[string]string `json:"env,omitempty"`
	CapabilitiesMax DriverCapabilities `json:"capabilities_max"`
}

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
