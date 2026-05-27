package generator

import (
	"fmt"
	"os"
	"path/filepath"
)

type MavenGenerator struct{}

func (g *MavenGenerator) Name() string { return "cyclonedx-maven-plugin" }

func (g *MavenGenerator) Available() bool { return toolExists("mvn") }

func (g *MavenGenerator) Generate(dir, outPath string) error {
	if err := run(dir, "mvn",
		"org.cyclonedx:cyclonedx-maven-plugin:makeAggregateBom",
		"-DoutputFormat=json",
		"-DoutputName=bom",
		"--batch-mode",
	); err != nil {
		return err
	}
	// Maven outputs to target/bom.json
	src := filepath.Join(dir, "target", "bom.json")
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("maven SBOM not found at %s", src)
	}
	return os.Rename(src, outPath)
}
