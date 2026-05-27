package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pribit/dtrack-submit/internal/config"
	"github.com/pribit/dtrack-submit/internal/detector"
	"github.com/pribit/dtrack-submit/internal/dtrack"
	"github.com/pribit/dtrack-submit/internal/generator"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "dtrack-submit",
	Short: "Auto-detect project type, generate CycloneDX SBOM, and upload to Dependency-Track",
	Example: `  dtrack-submit --server http://localhost:8080 --api-key odt_xxx --dir ./myproject
  dtrack-submit --config config.json
  dtrack-submit --server http://localhost:8080 --api-key odt_xxx --dir ./myproject --project MyApp --version 1.2.0`,
	RunE: run,
}

var flags struct {
	configFile string
	server     string
	apiKey     string
	dir        string
	project    string
	version    string
}

func init() {
	rootCmd.Flags().StringVar(&flags.configFile, "config", "", "JSON config file path")
	rootCmd.Flags().StringVar(&flags.server, "server", "", "Dependency-Track server URL (e.g. http://localhost:8080)")
	rootCmd.Flags().StringVar(&flags.apiKey, "api-key", "", "Dependency-Track API key")
	rootCmd.Flags().StringVar(&flags.dir, "dir", "", "Project directory to scan (default: current directory)")
	rootCmd.Flags().StringVar(&flags.project, "project", "", "Project name override")
	rootCmd.Flags().StringVar(&flags.version, "version", "", "Project version override")
}

func run(cmd *cobra.Command, _ []string) error {
	cfg := &config.Config{}

	// Load JSON config if provided
	if flags.configFile != "" {
		fileCfg, err := config.LoadFromFile(flags.configFile)
		if err != nil {
			return err
		}
		cfg = fileCfg
	}

	// CLI flags override JSON config
	cfg = config.Merge(cfg, &config.Config{
		Server:  flags.server,
		APIKey:  flags.apiKey,
		Dir:     flags.dir,
		Project: flags.project,
		Version: flags.version,
	})

	if err := cfg.Validate(); err != nil {
		return err
	}

	absDir, err := filepath.Abs(cfg.Dir)
	if err != nil {
		return fmt.Errorf("invalid directory: %w", err)
	}

	// Step 1: Detect project type
	fmt.Printf("→ Scanning %s\n", absDir)
	info, err := detector.Detect(absDir)
	if err != nil {
		return fmt.Errorf("detection error: %w", err)
	}
	if info.Type == detector.TypeUnknown {
		return fmt.Errorf("could not detect project type in %s\n  Supported: pom.xml, build.gradle, go.mod, *.csproj, *.sln, CMakeLists.txt, conanfile.*, vcpkg.json, package.json", absDir)
	}
	fmt.Printf("  Detected: %s\n", info.Type)

	// Step 2: Resolve name/version
	projectName := cfg.Project
	if projectName == "" {
		projectName = info.Name
	}
	if projectName == "" {
		projectName = filepath.Base(absDir)
	}
	projectVersion := cfg.Version
	if projectVersion == "" {
		projectVersion = info.Version
	}
	if projectVersion == "" {
		projectVersion = "0.0.0"
	}
	fmt.Printf("  Project:  %s @ %s\n", projectName, projectVersion)

	// Step 3: Select generator
	gen := selectGenerator(info)
	if gen == nil || !gen.Available() {
		fallback := &generator.CdxgenGenerator{}
		if !fallback.Available() {
			return fmt.Errorf("no SBOM generator available\n  Install one of:\n  - cdxgen: npm install -g @cyclonedx/cdxgen\n  - Language-specific: mvn, gradle, cyclonedx-gomod, dotnet CycloneDX, npx")
		}
		if gen != nil {
			fmt.Printf("  ⚠ %s not found, falling back to cdxgen\n", gen.Name())
		}
		gen = fallback
	}
	fmt.Printf("  Generator: %s\n", gen.Name())

	// Step 4: Generate SBOM
	bomPath := filepath.Join(os.TempDir(), "dtrack-bom.json")
	defer os.Remove(bomPath)

	fmt.Printf("→ Generating SBOM...\n")
	if err := gen.Generate(absDir, bomPath); err != nil {
		return fmt.Errorf("SBOM generation failed: %w", err)
	}
	fi, _ := os.Stat(bomPath)
	fmt.Printf("  Done (%s, %.1f KB)\n", filepath.Base(bomPath), float64(fi.Size())/1024)

	// Step 5: Upload to Dependency-Track
	client := dtrack.New(cfg.Server, cfg.APIKey)

	fmt.Printf("→ Creating project in Dependency-Track...\n")
	uuid, err := client.EnsureProject(projectName, projectVersion)
	if err != nil {
		return fmt.Errorf("project creation failed: %w", err)
	}
	fmt.Printf("  UUID: %s\n", uuid)

	fmt.Printf("→ Uploading SBOM...\n")
	token, err := client.UploadBOM(uuid, bomPath)
	if err != nil {
		return fmt.Errorf("SBOM upload failed: %w", err)
	}

	fmt.Printf("→ Waiting for analysis to complete...\n")
	if err := client.WaitForProcessing(token); err != nil {
		return err
	}

	// Step 6: Report results
	proj, err := client.GetProjectMetrics(uuid)
	if err == nil && proj.Metrics != nil {
		m := proj.Metrics
		fmt.Printf("\n✓ Done!\n")
		fmt.Printf("  Components:      %d\n", m.Components)
		fmt.Printf("  Vulnerabilities: %d", m.Vulnerabilities)
		if m.Vulnerabilities > 0 {
			fmt.Printf(" (Critical:%d High:%d Medium:%d Low:%d)", m.Critical, m.High, m.Medium, m.Low)
		}
		fmt.Printf("\n")
		fmt.Printf("  Dashboard: %s\n", cfg.Server)
	} else {
		fmt.Printf("\n✓ Done! View results at %s\n", cfg.Server)
	}

	return nil
}

func selectGenerator(info *detector.ProjectInfo) generator.Generator {
	switch info.Type {
	case detector.TypeMaven:
		return &generator.MavenGenerator{}
	case detector.TypeGradle:
		return &generator.GradleGenerator{}
	case detector.TypeGo:
		return &generator.GoModGenerator{}
	case detector.TypeDotNet:
		return &generator.DotNetGenerator{ManifestPath: info.Extra}
	case detector.TypeCpp:
		return generator.NewCppGenerator()
	case detector.TypeNpm:
		return &generator.NpmGenerator{}
	default:
		return nil
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
