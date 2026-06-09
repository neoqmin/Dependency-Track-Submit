package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// generateBOMFile writes a CycloneDX XML BOM to a temp file and returns the path + method used.
func generateBOMFile(ctx context.Context, workDir string) (string, string, error) {
	if workDir == "" {
		var err error
		workDir, err = os.Getwd()
		if err != nil {
			return "", "", err
		}
	}

	tmp, err := os.CreateTemp("", "bom-*.xml")
	if err != nil {
		return "", "", fmt.Errorf("temp file: %w", err)
	}
	tmp.Close()
	outPath := tmp.Name()

	// Try syft first
	if path, err := generateBOMWithSyft(ctx, workDir, outPath); err == nil {
		return path, "syft", nil
	}

	// Fallback: parse lockfiles
	xml, method, err := generateBOMFromLockfiles(workDir)
	if err != nil {
		os.Remove(outPath)
		return "", "", err
	}
	if err := os.WriteFile(outPath, []byte(xml), 0600); err != nil {
		return "", "", err
	}
	return outPath, method, nil
}

func generateBOMWithSyft(ctx context.Context, workDir, outPath string) (string, error) {
	syftPath, err := exec.LookPath("syft")
	if err != nil {
		return "", fmt.Errorf("syft not found")
	}
	ctx2, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx2, syftPath, workDir, "-o", "cyclonedx-xml="+outPath)
	cmd.Dir = workDir
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("syft failed: %w — %s", err, errBuf.String())
	}
	return outPath, nil
}

func generateBOMFromLockfiles(workDir string) (string, string, error) {
	if _, err := os.Stat(filepath.Join(workDir, "go.mod")); err == nil {
		xml, err := bomFromGoMod(workDir)
		if err == nil {
			return xml, "go.mod parser", nil
		}
	}
	if _, err := os.Stat(filepath.Join(workDir, "package-lock.json")); err == nil {
		xml, err := bomFromPackageLock(workDir)
		if err == nil {
			return xml, "package-lock.json parser", nil
		}
	}
	if _, err := os.Stat(filepath.Join(workDir, "requirements.txt")); err == nil {
		xml, err := bomFromRequirements(workDir)
		if err == nil {
			return xml, "requirements.txt parser", nil
		}
	}
	return "", "", fmt.Errorf("syft not found and no supported lockfile (go.mod, package-lock.json, requirements.txt)")
}

// dep is a single component. ecosystem is the Package URL type (golang, npm,
// pypi) used to build a PURL — Dependency-Track matches vulnerabilities by PURL,
// so a component without one yields zero findings.
type dep struct{ name, version, ecosystem string }

// purl builds a CycloneDX Package URL for the dependency, e.g.
// pkg:golang/golang.org/x/crypto@v0.51.0 or pkg:npm/lodash@4.17.21.
func (d dep) purl() string {
	if d.ecosystem == "" {
		return ""
	}
	name := d.name
	ver := d.version
	switch d.ecosystem {
	case "pypi":
		// PyPI PURLs are lowercase with underscores normalized to hyphens.
		name = strings.ReplaceAll(strings.ToLower(name), "_", "-")
	case "npm":
		// Scoped packages (@scope/name) keep the @ but the slash is encoded.
		name = strings.Replace(name, "/", "%2F", 1)
	}
	return fmt.Sprintf("pkg:%s/%s@%s", d.ecosystem, name, ver)
}

func bomFromGoMod(workDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(workDir, "go.mod"))
	if err != nil {
		return "", err
	}
	var deps []dep
	scanner := bufio.NewScanner(bytes.NewReader(data))
	inRequire := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "require (" {
			inRequire = true
			continue
		}
		if inRequire && line == ")" {
			inRequire = false
			continue
		}
		if inRequire || strings.HasPrefix(line, "require ") {
			line = strings.TrimPrefix(line, "require ")
			parts := strings.Fields(line)
			if len(parts) >= 2 && !strings.HasPrefix(parts[0], "//") {
				deps = append(deps, dep{parts[0], parts[1], "golang"})
			}
		}
	}
	return buildCycloneDXXML(deps), nil
}

func bomFromPackageLock(workDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(workDir, "package-lock.json"))
	if err != nil {
		return "", err
	}
	var deps []dep
	scanner := bufio.NewScanner(bytes.NewReader(data))
	var currentName, currentVersion string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, `"version":`) {
			currentVersion = strings.Trim(strings.TrimPrefix(line, `"version":`), ` ",`)
		} else if strings.HasSuffix(line, `": {`) && !strings.HasPrefix(line, `"node_modules`) {
			currentName = strings.Trim(line, `" {:`)
		}
		if currentName != "" && currentVersion != "" {
			deps = append(deps, dep{currentName, currentVersion, "npm"})
			currentName, currentVersion = "", ""
		}
	}
	return buildCycloneDXXML(deps), nil
}

func bomFromRequirements(workDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(workDir, "requirements.txt"))
	if err != nil {
		return "", err
	}
	var deps []dep
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		for _, sep := range []string{"==", ">=", "<=", "~="} {
			if idx := strings.Index(line, sep); idx > 0 {
				name := strings.TrimSpace(line[:idx])
				ver := strings.TrimSpace(line[idx+len(sep):])
				if i := strings.IndexAny(ver, ",; "); i > 0 {
					ver = ver[:i]
				}
				deps = append(deps, dep{name, ver, "pypi"})
				break
			}
		}
	}
	return buildCycloneDXXML(deps), nil
}

func buildCycloneDXXML(deps []dep) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	sb.WriteString("\n")
	sb.WriteString(`<bom xmlns="http://cyclonedx.org/schema/bom/1.4" version="1">`)
	sb.WriteString("\n  <components>\n")
	for _, d := range deps {
		name := escapeXML(d.name)
		ver := escapeXML(d.version)
		purl := d.purl()
		if purl != "" {
			fmt.Fprintf(&sb, "    <component type=\"library\" bom-ref=\"%s\">\n", escapeXML(purl))
		} else {
			fmt.Fprintf(&sb, "    <component type=\"library\">\n")
		}
		fmt.Fprintf(&sb, "      <name>%s</name>\n", name)
		fmt.Fprintf(&sb, "      <version>%s</version>\n", ver)
		if purl != "" {
			fmt.Fprintf(&sb, "      <purl>%s</purl>\n", escapeXML(purl))
		}
		fmt.Fprintf(&sb, "    </component>\n")
	}
	sb.WriteString("  </components>\n</bom>")
	return sb.String()
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}
