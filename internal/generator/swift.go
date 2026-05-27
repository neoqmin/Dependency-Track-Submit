package generator

// SwiftGenerator handles Swift projects (SPM, Xcode, Carthage, plain .swift)
// using cdxgen as the underlying tool.
type SwiftGenerator struct {
	cdxgen *CdxgenGenerator
}

func NewSwiftGenerator() *SwiftGenerator {
	return &SwiftGenerator{cdxgen: &CdxgenGenerator{}}
}

func (g *SwiftGenerator) Name() string    { return "cdxgen (swift)" }
func (g *SwiftGenerator) Available() bool { return g.cdxgen.Available() }
func (g *SwiftGenerator) Generate(dir, outPath string) error {
	return g.cdxgen.Generate(dir, outPath)
}
