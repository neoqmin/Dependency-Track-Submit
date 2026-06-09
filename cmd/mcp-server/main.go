package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/pribit/dtrack-submit/internal/config"
)

func main() {
	cfg := loadConfig()

	if cfg.Server == "" {
		fmt.Fprintln(os.Stderr, "[dtrack-mcp] DTRACK_URL or config server not set — defaulting to http://localhost:8080")
		cfg.Server = "http://localhost:8080"
	}
	if cfg.APIKey == "" {
		fmt.Fprintln(os.Stderr, "[dtrack-mcp] WARNING: no API key configured (set DTRACK_API_KEY or config api_key)")
	}

	logf("starting — version=%s server=%s", version, cfg.Server)

	cleanupOldBinary()
	startupUpdateCheck()

	client := NewDTrackClient(cfg.Server, cfg.APIKey)
	server := NewServer(client)
	if err := server.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "[dtrack-mcp] fatal: %v\n", err)
		os.Exit(1)
	}
}

// loadConfig builds a Config by merging (in priority order):
//  1. Environment variables (DTRACK_URL, DTRACK_API_KEY)
//  2. Config file at DTRACK_CONFIG env var path
//  3. Config file at default locations (./dtrack.json, ~/.dtrack.json)
func loadConfig() *config.Config {
	cfg := &config.Config{}

	// Default config file locations
	for _, path := range configSearchPaths() {
		if fc, err := config.LoadFromFile(path); err == nil {
			cfg = config.Merge(cfg, fc)
			logf("loaded config from %s", path)
			break
		}
	}

	// Environment overrides
	if v := os.Getenv("DTRACK_URL"); v != "" {
		cfg.Server = v
	}
	if v := os.Getenv("DTRACK_API_KEY"); v != "" {
		cfg.APIKey = v
	}

	return cfg
}

func configSearchPaths() []string {
	var paths []string
	if p := os.Getenv("DTRACK_CONFIG"); p != "" {
		paths = append(paths, p)
	}
	paths = append(paths, "dtrack.json")
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, home+"/.dtrack.json")
	}
	return paths
}

// writeExampleConfig prints a sample config.json to stdout and exits.
// Usage: dtrack-mcp-server --example-config
func init() {
	for _, arg := range os.Args[1:] {
		if arg == "--example-config" {
			example := map[string]string{
				"server":  "http://localhost:8080",
				"api_key": "YOUR_API_KEY_HERE",
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(example)
			os.Exit(0)
		}
	}
}
