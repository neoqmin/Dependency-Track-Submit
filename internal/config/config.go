package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// ProjectOverride lets users customize the name/version of a specific sub-project.
// Key in the Projects map is the subdirectory name (e.g. "api", "dashboard").
type ProjectOverride struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Config struct {
	Server  string `json:"server"`
	APIKey  string `json:"api_key"`
	Dir     string `json:"dir"`
	Project string `json:"project"`
	Version string `json:"version"`
	// Projects overrides name/version per sub-directory for mono-repos.
	// Key: subdirectory name, Value: override
	Projects map[string]ProjectOverride `json:"projects"`
}

func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config file read error: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config file parse error: %w", err)
	}
	return &cfg, nil
}

// Merge overlays non-empty fields from override onto base.
func Merge(base, override *Config) *Config {
	result := *base
	if override.Server != "" {
		result.Server = override.Server
	}
	if override.APIKey != "" {
		result.APIKey = override.APIKey
	}
	if override.Dir != "" {
		result.Dir = override.Dir
	}
	if override.Project != "" {
		result.Project = override.Project
	}
	if override.Version != "" {
		result.Version = override.Version
	}
	return &result
}

func (c *Config) Validate() error {
	if c.Server == "" {
		return fmt.Errorf("--server is required")
	}
	if c.APIKey == "" {
		return fmt.Errorf("--api-key is required")
	}
	if c.Dir == "" {
		c.Dir = "."
	}
	return nil
}
