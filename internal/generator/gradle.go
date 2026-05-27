package generator

import (
	"fmt"
	"os"
	"path/filepath"
)

type GradleGenerator struct{}

func (g *GradleGenerator) Name() string { return "cyclonedx-gradle-plugin" }

func (g *GradleGenerator) Available() bool {
	wrapper := filepath.Join(".", gradleWrapper())
	if _, err := os.Stat(wrapper); err == nil {
		return true
	}
	return toolExists("gradle")
}

func (g *GradleGenerator) Generate(dir, outPath string) error {
	wrapper := gradleWrapper()
	wrapperPath := filepath.Join(dir, wrapper)
	var cmd string
	if _, err := os.Stat(wrapperPath); err == nil {
		cmd = wrapperPath
	} else {
		cmd = "gradle"
	}
	if err := run(dir, cmd, "cyclonedxBom"); err != nil {
		return err
	}
	// Gradle outputs to build/reports/cyclonedx/bom.json
	src := filepath.Join(dir, "build", "reports", "cyclonedx", "bom.json")
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("gradle SBOM not found at %s", src)
	}
	return os.Rename(src, outPath)
}
