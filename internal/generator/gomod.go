package generator

type GoModGenerator struct{}

func (g *GoModGenerator) Name() string { return "cyclonedx-gomod" }

func (g *GoModGenerator) Available() bool { return toolExists("cyclonedx-gomod") }

func (g *GoModGenerator) Generate(dir, outPath string) error {
	return run(dir, "cyclonedx-gomod", "app", "-json", "-output", outPath, ".")
}
