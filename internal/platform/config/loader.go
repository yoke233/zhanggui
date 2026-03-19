package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

func LoadFile(path string) (*Config, error) {
	return LoadGlobal(path)
}

// LoadLayerFile loads a raw config layer file without applying defaults or env overrides.
func LoadLayerFile(path string) (*ConfigLayer, error) {
	return loadLayerFromFile(path)
}

// LoadLayerBytes loads a raw config layer from TOML bytes.
func LoadLayerBytes(data []byte) (*ConfigLayer, error) {
	return loadLayerFromBytes(data)
}

// MarshalLayerTOML serializes a config layer into TOML.
func MarshalLayerTOML(layer *ConfigLayer) ([]byte, error) {
	if layer == nil {
		layer = &ConfigLayer{}
	}
	data, err := toml.Marshal(layer)
	if err != nil {
		return nil, fmt.Errorf("marshal config layer: %w", err)
	}
	return data, nil
}

// LoadGlobal loads config from a file (TOML or YAML), applies env overrides, and validates.
// secretsPath is optional — if non-empty, secrets are loaded and merged before validation.
func LoadGlobal(path string, secretsPaths ...string) (*Config, error) {
	cfg := Defaults()

	if path != "" {
		layer, err := loadLayerFromFile(path)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
			// File does not exist — use defaults only.
		} else {
			ApplyConfigLayer(&cfg, layer)
		}
	}

	// Apply secrets file if provided.
	if len(secretsPaths) > 0 && secretsPaths[0] != "" {
		secrets, err := LoadSecrets(secretsPaths[0])
		if err != nil {
			return nil, err
		}
		ApplySecrets(&cfg, secrets)
	}

	if err := ApplyEnvOverrides(&cfg); err != nil {
		return nil, err
	}
	if err := validateConfig(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func LoadProject(repoPath string) (*ConfigLayer, error) {
	path := ProjectConfigPath(repoPath)
	layer, err := loadLayerFromFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &ConfigLayer{}, nil
		}
		return nil, err
	}
	return layer, nil
}

func loadLayerFromFile(path string) (*ConfigLayer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".toml":
		return loadLayerFromTOML(data)
	case ".yaml", ".yml":
		return loadLayerFromYAML(data)
	default:
		return loadLayerFromTOML(data)
	}
}

func decodeLayerFromMap(raw map[string]any) (*ConfigLayer, error) {
	if raw == nil {
		return &ConfigLayer{}, nil
	}

	data, err := toml.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal override map: %w", err)
	}
	return loadLayerFromTOML(data)
}

// loadLayerFromBytes parses config layer from TOML bytes (default format).
func loadLayerFromBytes(data []byte) (*ConfigLayer, error) {
	return loadLayerFromTOML(data)
}

func loadLayerFromTOML(data []byte) (*ConfigLayer, error) {
	layer := &ConfigLayer{}
	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(layer); err != nil {
		return nil, err
	}
	return layer, nil
}

func loadLayerFromYAML(data []byte) (*ConfigLayer, error) {
	layer := &ConfigLayer{}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(layer); err != nil {
		return nil, err
	}
	return layer, nil
}
