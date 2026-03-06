package config

import (
	_ "embed"
	"fmt"

	toml "github.com/pelletier/go-toml/v2"
)

//go:embed defaults.toml
var defaultsTOML []byte

// DefaultsTOML returns the raw embedded default config TOML bytes.
func DefaultsTOML() []byte {
	return append([]byte(nil), defaultsTOML...)
}

// Defaults parses the embedded defaults.toml into a Config.
func Defaults() Config {
	var cfg Config
	if err := toml.Unmarshal(defaultsTOML, &cfg); err != nil {
		panic(fmt.Sprintf("BUG: embedded defaults.toml is invalid: %v", err))
	}
	return cfg
}

func ptrValue[T any](v T) *T { return &v }
