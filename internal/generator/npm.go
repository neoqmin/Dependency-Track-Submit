package generator

type NpmGenerator struct{}

func (g *NpmGenerator) Name() string { return "@cyclonedx/cyclonedx-npm" }

func (g *NpmGenerator) Available() bool { return toolExists("npx") }

func (g *NpmGenerator) Generate(dir, outPath string) error {
	return run(dir, "npx", "--yes", "@cyclonedx/cyclonedx-npm",
		"--output-format", "JSON",
		"--output-file", outPath,
		"--short-PURLs",
	)
}
