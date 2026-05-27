package generator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// CocoaGenerator parses Podfile.lock and builds a CycloneDX SBOM directly.
// Falls back to cdxgen if Podfile.lock is absent.
type CocoaGenerator struct {
	cdxgen *CdxgenGenerator
}

func NewCocoaGenerator() *CocoaGenerator {
	return &CocoaGenerator{cdxgen: &CdxgenGenerator{}}
}

func (g *CocoaGenerator) Name() string    { return "podfile-lock-parser" }
func (g *CocoaGenerator) Available() bool { return true }

func (g *CocoaGenerator) Generate(dir, outPath string) error {
	lockPath := filepath.Join(dir, "Podfile.lock")
	if _, err := os.Stat(lockPath); err != nil {
		// No Podfile.lock — fall back to cdxgen
		if g.cdxgen.Available() {
			return g.cdxgen.Generate(dir, outPath)
		}
		return fmt.Errorf("Podfile.lock not found and cdxgen unavailable")
	}
	return parsePodfileLock(lockPath, outPath)
}

var podLineRe = regexp.MustCompile(`^\s{2}- ([^/\s(]+)(?:/[^\s(]*)?\s+\(([^)]+)\)`)

func parsePodfileLock(lockPath, outPath string) error {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return fmt.Errorf("read Podfile.lock: %w", err)
	}

	// Collect unique top-level pod name → version (skip sub-specs like Pod/Sub)
	type pod struct{ name, version string }
	seen := map[string]bool{}
	var pods []pod

	inPods := false
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "PODS:") {
			inPods = true
			continue
		}
		if inPods && line != "" && line[0] != ' ' {
			break // end of PODS section
		}
		if !inPods {
			continue
		}
		m := podLineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name, ver := m[1], m[2]
		if !seen[name] {
			seen[name] = true
			pods = append(pods, pod{name, ver})
		}
	}

	// Build CycloneDX 1.6 BOM
	type component struct {
		Type    string `json:"type"`
		Name    string `json:"name"`
		Version string `json:"version"`
		PURL    string `json:"purl"`
		BOMRef  string `json:"bom-ref"`
	}
	var components []component
	for _, p := range pods {
		components = append(components, component{
			Type:    "library",
			Name:    p.name,
			Version: p.version,
			PURL:    fmt.Sprintf("pkg:cocoapods/%s@%s", p.name, p.version),
			BOMRef:  fmt.Sprintf("pkg:cocoapods/%s@%s", p.name, p.version),
		})
	}

	bom := map[string]interface{}{
		"bomFormat":    "CycloneDX",
		"specVersion":  "1.6",
		"serialNumber": "urn:uuid:" + uuid.New().String(),
		"version":      1,
		"metadata": map[string]interface{}{
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"tools": map[string]interface{}{
				"components": []map[string]string{
					{"type": "application", "name": "dtrack-submit", "version": "1.0.0"},
				},
			},
		},
		"components": components,
	}

	out, err := json.MarshalIndent(bom, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal bom: %w", err)
	}
	return os.WriteFile(outPath, out, 0644)
}
