package generator

// CppGenerator delegates entirely to cdxgen since C++ tooling is fragmented.
type CppGenerator struct {
	fallback *CdxgenGenerator
}

func NewCppGenerator() *CppGenerator {
	return &CppGenerator{fallback: &CdxgenGenerator{}}
}

func (g *CppGenerator) Name() string { return g.fallback.Name() }

func (g *CppGenerator) Available() bool { return g.fallback.Available() }

func (g *CppGenerator) Generate(dir, outPath string) error {
	return g.fallback.Generate(dir, outPath)
}
