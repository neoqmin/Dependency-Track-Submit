package generator

import "fmt"

// CdxgenGenerator is the universal fallback using cdxgen.
type CdxgenGenerator struct{}

func (g *CdxgenGenerator) Name() string { return "cdxgen" }

func (g *CdxgenGenerator) Available() bool {
	return toolExists("cdxgen") || toolExists("npx")
}

func (g *CdxgenGenerator) Generate(dir, outPath string) error {
	args := []string{"-o", outPath, "--spec-version", "1.6", "."}
	if toolExists("cdxgen") {
		return run(dir, "cdxgen", args...)
	}
	if toolExists("npx") {
		return run(dir, "npx", append([]string{"--yes", "@cyclonedx/cdxgen"}, args...)...)
	}
	return fmt.Errorf("cdxgen not available: install via 'npm install -g @cyclonedx/cdxgen'")
}
