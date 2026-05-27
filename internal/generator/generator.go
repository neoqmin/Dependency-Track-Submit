package generator

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/tidwall/sjson"
)

// DowngradeSpecVersion rewrites the specVersion field in a CycloneDX JSON BOM
// to maxVersion if the file contains a newer version.
func DowngradeSpecVersion(bomPath, maxVersion string) error {
	data, err := os.ReadFile(bomPath)
	if err != nil {
		return err
	}
	patched, err := sjson.SetBytes(data, "specVersion", maxVersion)
	if err != nil {
		return err
	}
	return os.WriteFile(bomPath, patched, 0644)
}

// Generator produces a CycloneDX JSON SBOM at outPath.
type Generator interface {
	Name() string
	Available() bool
	Generate(dir, outPath string) error
}

// toolExists checks whether a CLI tool is in PATH.
func toolExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// run executes a command in dir and returns combined output on failure.
func run(dir string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s failed: %w\n%s", name, err, string(out))
	}
	return nil
}

func gradleWrapper() string {
	if runtime.GOOS == "windows" {
		return "gradlew.bat"
	}
	return "./gradlew"
}
