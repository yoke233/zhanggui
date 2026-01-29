package taskbundle

import (
	"fmt"

	yaml "go.yaml.in/yaml/v3"
)

type criteriaFile struct {
	SchemaVersion int `yaml:"schema_version"`

	CriteriaSet struct {
		ID      string `yaml:"id"`
		Version string `yaml:"version"`
		Title   string `yaml:"title"`
	} `yaml:"criteria_set"`

	Criteria []struct {
		ID       string `yaml:"id"`
		Title    string `yaml:"title"`
		Severity string `yaml:"severity"`
		Scope    string `yaml:"scope"`

		Description string `yaml:"description"`

		Evidence struct {
			Kind string `yaml:"kind"`
			Path string `yaml:"path"`
		} `yaml:"evidence"`
	} `yaml:"criteria"`
}

func parseCriteriaYAML(b []byte) (criteriaFile, error) {
	var cf criteriaFile
	if err := yaml.Unmarshal(b, &cf); err != nil {
		return criteriaFile{}, err
	}
	if cf.SchemaVersion != 1 {
		return criteriaFile{}, fmt.Errorf("criteria schema_version 不支持: %d", cf.SchemaVersion)
	}
	if cf.CriteriaSet.ID == "" || cf.CriteriaSet.Version == "" {
		return criteriaFile{}, fmt.Errorf("criteria_set.id/version 不能为空")
	}
	return cf, nil
}
