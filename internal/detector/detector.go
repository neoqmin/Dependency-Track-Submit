package detector

import (
	"encoding/json"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
)

type ProjectType string

const (
	TypeMaven   ProjectType = "maven"
	TypeGradle  ProjectType = "gradle"
	TypeGo      ProjectType = "go"
	TypeDotNet  ProjectType = "dotnet"
	TypeCpp     ProjectType = "cpp"
	TypeNpm     ProjectType = "npm"
	TypeCocoa   ProjectType = "cocoapods"
	TypeSwift   ProjectType = "swift"
	TypeUnknown ProjectType = "unknown"
)

type ProjectInfo struct {
	Type    ProjectType
	Name    string
	Version string
	Dir     string
	// Extra holds the primary manifest path (e.g. *.csproj path)
	Extra string
}

// DetectAll scans dir and its immediate subdirectories.
// Returns all detected projects. If the root itself is a project, only the root is returned.
func DetectAll(dir string) ([]*ProjectInfo, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}

	root, err := Detect(abs)
	if err != nil {
		return nil, err
	}
	if root.Type != TypeUnknown {
		root.Dir = abs
		return []*ProjectInfo{root}, nil
	}

	// Root has no manifest — scan immediate subdirectories
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}

	var results []*ProjectInfo
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		sub := filepath.Join(abs, e.Name())
		info, err := Detect(sub)
		if err != nil || info.Type == TypeUnknown {
			continue
		}
		info.Dir = sub
		results = append(results, info)
	}
	return results, nil
}

func Detect(dir string) (*ProjectInfo, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}

	// Maven
	if exists(abs, "pom.xml") {
		info := &ProjectInfo{Type: TypeMaven}
		parsePom(filepath.Join(abs, "pom.xml"), info)
		return info, nil
	}

	// Gradle
	if exists(abs, "build.gradle") || exists(abs, "build.gradle.kts") {
		info := &ProjectInfo{Type: TypeGradle}
		parseGradle(abs, info)
		return info, nil
	}

	// Go
	if exists(abs, "go.mod") {
		info := &ProjectInfo{Type: TypeGo}
		parseGoMod(filepath.Join(abs, "go.mod"), info)
		return info, nil
	}

	// C#/.NET — look for *.csproj or *.sln
	if csproj, ok := findGlob(abs, "*.csproj"); ok {
		info := &ProjectInfo{Type: TypeDotNet, Extra: csproj}
		parseCsproj(csproj, info)
		return info, nil
	}
	if sln, ok := findGlob(abs, "*.sln"); ok {
		info := &ProjectInfo{Type: TypeDotNet, Extra: sln}
		info.Name = strings.TrimSuffix(filepath.Base(sln), ".sln")
		return info, nil
	}

	// C++
	if exists(abs, "CMakeLists.txt") || exists(abs, "conanfile.txt") ||
		exists(abs, "conanfile.py") || exists(abs, "vcpkg.json") {
		info := &ProjectInfo{Type: TypeCpp}
		if exists(abs, "vcpkg.json") {
			parseVcpkg(filepath.Join(abs, "vcpkg.json"), info)
		} else if exists(abs, "CMakeLists.txt") {
			parseCMake(filepath.Join(abs, "CMakeLists.txt"), info)
		}
		return info, nil
	}

	// Node.js / npm
	if exists(abs, "package.json") {
		info := &ProjectInfo{Type: TypeNpm}
		parsePackageJSON(filepath.Join(abs, "package.json"), info)
		return info, nil
	}

	// CocoaPods (iOS / macOS Objective-C / Swift)
	if exists(abs, "Podfile") || exists(abs, "Podfile.lock") {
		info := &ProjectInfo{Type: TypeCocoa}
		parsePodfile(abs, info)
		return info, nil
	}

	// Swift Package Manager
	if exists(abs, "Package.swift") {
		info := &ProjectInfo{Type: TypeSwift}
		parsePackageSwift(filepath.Join(abs, "Package.swift"), info)
		return info, nil
	}

	// Carthage
	if exists(abs, "Cartfile") || exists(abs, "Cartfile.resolved") {
		info := &ProjectInfo{Type: TypeSwift, Name: filepath.Base(abs)}
		return info, nil
	}

	// Xcode project / workspace (no package manager)
	if xcodeproj, ok := findGlob(abs, "*.xcodeproj"); ok {
		info := &ProjectInfo{Type: TypeSwift}
		info.Name = strings.TrimSuffix(filepath.Base(xcodeproj), ".xcodeproj")
		return info, nil
	}
	if xcworkspace, ok := findGlob(abs, "*.xcworkspace"); ok {
		info := &ProjectInfo{Type: TypeSwift}
		info.Name = strings.TrimSuffix(filepath.Base(xcworkspace), ".xcworkspace")
		return info, nil
	}

	// Plain Swift source (*.swift files present, no other manifest)
	if swiftFiles, _ := filepath.Glob(filepath.Join(abs, "*.swift")); len(swiftFiles) > 0 {
		info := &ProjectInfo{Type: TypeSwift, Name: filepath.Base(abs)}
		return info, nil
	}

	return &ProjectInfo{Type: TypeUnknown}, nil
}

func exists(dir, file string) bool {
	_, err := os.Stat(filepath.Join(dir, file))
	return err == nil
}

func findGlob(dir, pattern string) (string, bool) {
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil || len(matches) == 0 {
		return "", false
	}
	return matches[0], true
}

// Maven
type pomXML struct {
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
}

func parsePom(path string, info *ProjectInfo) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var pom pomXML
	if err := xml.Unmarshal(data, &pom); err != nil {
		return
	}
	info.Name = pom.ArtifactID
	info.Version = pom.Version
}

// Gradle — best-effort line scanning
func parseGradle(dir string, info *ProjectInfo) {
	for _, f := range []string{"settings.gradle", "settings.gradle.kts"} {
		data, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "rootProject.name") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					info.Name = strings.Trim(strings.TrimSpace(parts[1]), `"' `)
				}
			}
		}
	}
	for _, f := range []string{"build.gradle", "build.gradle.kts"} {
		data, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "version") && strings.Contains(line, "=") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					info.Version = strings.Trim(strings.TrimSpace(parts[1]), `"' `)
				}
			}
		}
		break
	}
}

// Go modules
func parseGoMod(path string, info *ProjectInfo) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			mod := strings.TrimPrefix(line, "module ")
			parts := strings.Split(strings.TrimSpace(mod), "/")
			info.Name = parts[len(parts)-1]
		}
		if strings.HasPrefix(line, "go ") {
			info.Version = strings.TrimPrefix(line, "go ")
		}
	}
}

// C# .csproj
type csprojXML struct {
	PropertyGroups []struct {
		AssemblyName string `xml:"AssemblyName"`
		Version      string `xml:"Version"`
	} `xml:"PropertyGroup"`
}

func parseCsproj(path string, info *ProjectInfo) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var proj csprojXML
	if err := xml.Unmarshal(data, &proj); err != nil {
		// fallback: use filename
		info.Name = strings.TrimSuffix(filepath.Base(path), ".csproj")
		return
	}
	for _, pg := range proj.PropertyGroups {
		if pg.AssemblyName != "" {
			info.Name = pg.AssemblyName
		}
		if pg.Version != "" {
			info.Version = pg.Version
		}
	}
	if info.Name == "" {
		info.Name = strings.TrimSuffix(filepath.Base(path), ".csproj")
	}
}

// vcpkg.json
func parseVcpkg(path string, info *ProjectInfo) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var v struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return
	}
	info.Name = v.Name
	info.Version = v.Version
}

// CMakeLists.txt — look for project() call
func parseCMake(path string, info *ProjectInfo) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	content := string(data)
	lower := strings.ToLower(content)
	idx := strings.Index(lower, "project(")
	if idx < 0 {
		return
	}
	rest := content[idx+len("project("):]
	end := strings.Index(rest, ")")
	if end < 0 {
		return
	}
	args := strings.Fields(rest[:end])
	if len(args) > 0 {
		info.Name = args[0]
	}
	for i, arg := range args {
		if strings.EqualFold(arg, "VERSION") && i+1 < len(args) {
			info.Version = args[i+1]
		}
	}
}

// package.json
func parsePackageJSON(path string, info *ProjectInfo) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var pkg struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return
	}
	info.Name = pkg.Name
	info.Version = pkg.Version
}

// Podfile — extract app name from xcodeproj reference or directory name
func parsePodfile(dir string, info *ProjectInfo) {
	data, err := os.ReadFile(filepath.Join(dir, "Podfile"))
	if err != nil {
		// fall back to directory name
		info.Name = filepath.Base(dir)
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		// e.g. project 'MyApp' or xcodeproj 'MyApp/MyApp.xcodeproj'
		for _, prefix := range []string{"project '", `project "`, "xcodeproj '", `xcodeproj "`} {
			if strings.HasPrefix(strings.ToLower(line), strings.ToLower(prefix)) {
				val := line[len(prefix):]
				end := strings.IndexAny(val, `'"`)
				if end > 0 {
					// strip path, keep last component without extension
					base := filepath.Base(val[:end])
					info.Name = strings.TrimSuffix(strings.TrimSuffix(base, ".xcodeproj"), ".xcworkspace")
					return
				}
			}
		}
	}
	if info.Name == "" {
		info.Name = filepath.Base(dir)
	}
}

// Package.swift — extract name from swift package declaration
func parsePackageSwift(path string, info *ProjectInfo) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		// name: "MyPackage"
		if strings.Contains(line, "name:") {
			start := strings.Index(line, `"`)
			if start >= 0 {
				rest := line[start+1:]
				end := strings.Index(rest, `"`)
				if end >= 0 {
					info.Name = rest[:end]
					return
				}
			}
		}
	}
}
