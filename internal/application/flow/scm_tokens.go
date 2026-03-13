package flow

import "strings"

// SCMTokens carries PATs used by builtin SCM automation steps (push, open PR, merge).
// Each field corresponds to a specific SCM provider.
type SCMTokens struct {
	GitHub string
	Codeup string
}

// EffectivePAT returns the first non-empty PAT.
// For single-provider setups (the common case) this is all you need.
func (t SCMTokens) EffectivePAT() string {
	if v := strings.TrimSpace(t.GitHub); v != "" {
		return v
	}
	return strings.TrimSpace(t.Codeup)
}
