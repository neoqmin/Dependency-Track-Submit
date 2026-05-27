package generator

import (
	"fmt"
	"os"
	"path/filepath"
)

type DotNetGenerator struct {
	// ManifestPath is the .csproj or .sln file to analyse.
	ManifestPath string
}

func (g *DotNetGenerator) Name() string { return "dotnet-CycloneDX" }

func (g *DotNetGenerator) Available() bool {
	if !toolExists("dotnet") {
		return false
	}
	// Check for the global tool
	err := run(".", "dotnet", "tool", "list", "--global")
	return err == nil
}

func (g *DotNetGenerator) Generate(dir, outPath string) error {
	manifest := g.ManifestPath
	if manifest == "" {
		manifest = dir
	}
	outDir := filepath.Dir(outPath)
	if err := run(dir, "dotnet", "CycloneDX", manifest, "-o", outDir, "-F", "Json"); err != nil {
		return err
	}
	// dotnet-CycloneDX writes bom.json into outDir
	src := filepath.Join(outDir, "bom.json")
	if src == outPath {
		return nil
	}
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("dotnet SBOM not found at %s", src)
	}
	return os.Rename(src, outPath)
}
