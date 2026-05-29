package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pribit/dtrack-submit/internal/config"
	"github.com/pribit/dtrack-submit/internal/detector"
	"github.com/pribit/dtrack-submit/internal/dtrack"
	"github.com/pribit/dtrack-submit/internal/generator"
	"github.com/pribit/dtrack-submit/internal/report"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "dtrack-submit [dir]",
	Short: "Auto-detect project type, generate CycloneDX SBOM, and upload to Dependency-Track",
	Args:  cobra.MaximumNArgs(1),
	Example: `  # config.json이 현재 디렉토리 또는 실행 파일 옆에 있으면 자동 로딩
  dtrack-submit ./myproject

  # config 파일 명시
  dtrack-submit --config /etc/dtrack/config.json ./myproject

  # 모든 값을 플래그로 직접 지정
  dtrack-submit --server http://localhost:8080 --api-key odt_xxx ./myproject
  dtrack-submit --server http://localhost:8080 --api-key odt_xxx --project MyApp --version 1.2.0 ./myproject`,
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

func run(cmd *cobra.Command, args []string) error {
	cfg := &config.Config{}

	// Auto-load config.json if --config and --server are both absent.
	// Search order: current directory → executable directory.
	configPath := flags.configFile
	if configPath == "" && flags.server == "" {
		configPath = findConfig()
	}
	if configPath != "" {
		fileCfg, err := config.LoadFromFile(configPath)
		if err != nil && flags.configFile != "" {
			return err // only hard-fail if explicitly specified
		} else if err == nil {
			cfg = fileCfg
		}
	}

	// Positional argument takes priority over --dir flag
	if len(args) > 0 {
		flags.dir = args[0]
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

	// Step 1: Detect project(s) — supports mono-repos
	fmt.Printf("→ Scanning %s\n", absDir)
	projects, err := detector.DetectAll(absDir)
	if err != nil {
		return fmt.Errorf("detection error: %w", err)
	}
	if len(projects) == 0 {
		return fmt.Errorf("could not detect any project in %s\n  Supported: pom.xml, build.gradle, go.mod, *.csproj, *.sln, CMakeLists.txt, conanfile.*, vcpkg.json, package.json, Podfile, Package.swift", absDir)
	}

	rootName := cfg.Project
	if rootName == "" {
		rootName = filepath.Base(absDir)
	}
	isMulti := len(projects) > 1
	if isMulti {
		fmt.Printf("  Found %d sub-projects\n", len(projects))
	}

	client := dtrack.New(cfg.Server, cfg.APIKey)
	var failed []string

	for _, info := range projects {
		if err := submitProject(client, cfg, info, rootName, isMulti); err != nil {
			fmt.Printf("  ✗ %s: %v\n", info.Dir, err)
			failed = append(failed, info.Dir)
		}
	}

	if len(failed) > 0 {
		return fmt.Errorf("%d project(s) failed", len(failed))
	}
	return nil
}

func submitProject(client *dtrack.Client, cfg *config.Config, info *detector.ProjectInfo, rootName string, isMulti bool) error {
	subdir := filepath.Base(info.Dir)

	// projects map in config takes highest priority for mono-repos
	projectName := ""
	projectVersion := ""
	if ov, ok := cfg.Projects[subdir]; ok {
		projectName = ov.Name
		projectVersion = ov.Version
	}

	// Fall back to top-level config fields, then auto-detection
	if projectName == "" {
		if !isMulti && cfg.Project != "" {
			projectName = cfg.Project
		} else if info.Name != "" {
			projectName = info.Name
		} else {
			projectName = subdir
		}
		if isMulti {
			projectName = rootName + "/" + projectName
		}
	}
	if projectVersion == "" {
		if cfg.Version != "" {
			projectVersion = cfg.Version
		} else if info.Version != "" {
			projectVersion = info.Version
		} else {
			projectVersion = "0.0.0"
		}
	}

	fmt.Printf("\n  [%s] %s @ %s\n", info.Type, projectName, projectVersion)

	// Select generator
	gen := selectGenerator(info)
	if gen == nil || !gen.Available() {
		fallback := &generator.CdxgenGenerator{}
		if !fallback.Available() {
			return fmt.Errorf("no SBOM generator available\n  Install: npm install -g @cyclonedx/cdxgen")
		}
		if gen != nil {
			fmt.Printf("  ⚠ %s not found, falling back to cdxgen\n", gen.Name())
		}
		gen = fallback
	}
	fmt.Printf("  Generator: %s\n", gen.Name())

	// Generate SBOM
	bomPath := filepath.Join(os.TempDir(), "dtrack-bom-"+filepath.Base(info.Dir)+".json")
	defer os.Remove(bomPath)

	fmt.Printf("→ Generating SBOM...\n")
	if err := gen.Generate(info.Dir, bomPath); err != nil {
		return fmt.Errorf("SBOM generation failed: %w", err)
	}
	specVer, err := generator.DowngradeSpecVersion(bomPath, "1.6")
	if err != nil {
		return fmt.Errorf("BOM post-process failed: %w", err)
	}
	fi, _ := os.Stat(bomPath)
	fmt.Printf("  Done (%.1f KB, specVersion: %s)\n", float64(fi.Size())/1024, specVer)

	// Upload
	fmt.Printf("→ Uploading...\n")
	uuid, err := client.EnsureProject(projectName, projectVersion)
	if err != nil {
		return fmt.Errorf("project creation failed: %w", err)
	}

	token, err := client.UploadBOM(uuid, bomPath)
	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}

	fmt.Printf("→ Waiting for analysis...\n")
	if err := client.WaitForProcessing(token); err != nil {
		return err
	}

	proj, err := client.GetProjectMetrics(uuid)
	if err == nil && proj.Metrics != nil {
		m := proj.Metrics
		fmt.Printf("✓ Components: %d  Vulnerabilities: %d", m.Components, m.Vulnerabilities)
		if m.Vulnerabilities > 0 {
			fmt.Printf(" (Critical:%d High:%d Medium:%d Low:%d)", m.Critical, m.High, m.Medium, m.Low)
		}
		fmt.Printf("\n")
	} else {
		fmt.Printf("✓ Done\n")
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
	case detector.TypeCocoa:
		return generator.NewCocoaGenerator()
	case detector.TypeSwift:
		return generator.NewSwiftGenerator()
	default:
		return nil
	}
}

// findConfig looks for config.json in the current directory, then next to the executable.
func findConfig() string {
	candidates := []string{"config.json"}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "config.json"))
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// ── report command ──────────────────────────────────────────────────────────

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Query Dependency-Track for vulnerabilities and print a remediation report",
	Example: `  dtrack-submit report --server http://localhost:8080 --api-key odt_xxx \
    --project MyApp --version 1.0.0

  # Save as Markdown
  dtrack-submit report ... --output report.md

  # Save as JSON
  dtrack-submit report ... --output report.json

  # Lower severity threshold
  dtrack-submit report ... --severity MEDIUM`,
	RunE: runReport,
}

var reportFlags struct {
	server   string
	apiKey   string
	project  string
	version  string
	severity string
	output   string
}

func init() {
	f := reportCmd.Flags()
	f.StringVar(&reportFlags.server, "server", "", "Dependency-Track server URL")
	f.StringVar(&reportFlags.apiKey, "api-key", "", "Dependency-Track API key")
	f.StringVar(&reportFlags.project, "project", "", "Project name")
	f.StringVar(&reportFlags.version, "version", "", "Project version")
	f.StringVar(&reportFlags.severity, "severity", "HIGH", "Minimum severity to report (CRITICAL, HIGH, MEDIUM, LOW)")
	f.StringVar(&reportFlags.output, "output", "", "Save report to file (.md or .json)")

	rootCmd.AddCommand(reportCmd)
}

func runReport(cmd *cobra.Command, args []string) error {
	if reportFlags.server == "" {
		return fmt.Errorf("--server is required")
	}
	if reportFlags.apiKey == "" {
		return fmt.Errorf("--api-key is required")
	}
	if reportFlags.project == "" {
		return fmt.Errorf("--project is required")
	}
	if reportFlags.version == "" {
		return fmt.Errorf("--version is required")
	}

	minSeverity := strings.ToUpper(reportFlags.severity)

	client := dtrack.New(reportFlags.server, reportFlags.apiKey)

	// Resolve project UUID
	uuid, err := client.LookupProject(reportFlags.project, reportFlags.version)
	if err != nil {
		return fmt.Errorf("project not found (%s @ %s): %w", reportFlags.project, reportFlags.version, err)
	}

	fmt.Printf("→ Fetching findings for %s @ %s...\n", reportFlags.project, reportFlags.version)
	rows, err := report.Generate(client, uuid, reportFlags.project, reportFlags.version, minSeverity)
	if err != nil {
		return fmt.Errorf("report generation failed: %w", err)
	}

	report.PrintConsole(rows, reportFlags.project, reportFlags.version, minSeverity)

	if reportFlags.output != "" {
		var saveErr error
		if strings.HasSuffix(reportFlags.output, ".json") {
			saveErr = report.SaveJSON(rows, reportFlags.project, reportFlags.version, minSeverity, reportFlags.output)
		} else {
			saveErr = report.SaveMarkdown(rows, reportFlags.project, reportFlags.version, minSeverity, reportFlags.output)
		}
		if saveErr != nil {
			return fmt.Errorf("save report failed: %w", saveErr)
		}
		fmt.Printf("→ Saved to %s\n", reportFlags.output)
	}

	return nil
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
