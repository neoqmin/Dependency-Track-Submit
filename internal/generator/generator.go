package generator

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// DowngradeSpecVersion rewrites the specVersion field in a CycloneDX JSON BOM to maxVersion.
// Returns the specVersion that was written.
func DowngradeSpecVersion(bomPath, maxVersion string) (string, error) {
	data, err := os.ReadFile(bomPath)
	if err != nil {
		return "", fmt.Errorf("read bom: %w", err)
	}

	var bom map[string]json.RawMessage
	if err := json.Unmarshal(data, &bom); err != nil {
		return "", fmt.Errorf("parse bom json: %w", err)
	}

	versionBytes, err := json.Marshal(maxVersion)
	if err != nil {
		return "", err
	}
	bom["specVersion"] = json.RawMessage(versionBytes)

	patched, err := json.Marshal(bom)
	if err != nil {
		return "", fmt.Errorf("marshal bom: %w", err)
	}
	return maxVersion, os.WriteFile(bomPath, patched, 0644)
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
