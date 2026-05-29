package report

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pribit/dtrack-submit/internal/dtrack"
)

var severityRank = map[string]int{
	"CRITICAL": 4,
	"HIGH":     3,
	"MEDIUM":   2,
	"LOW":      1,
	"INFO":     0,
}

// Row represents one output row: one component with its worst vulnerability.
type Row struct {
	Package       string
	CurrentVer    string
	LatestVer     string
	Purl          string
	Vulns         []VulnSummary
	WorseSeverity string
	WorseCvss     float64
}

type VulnSummary struct {
	VulnId   string
	Severity string
	Cvss     float64
}

// JSONReport is the machine-readable output format.
type JSONReport struct {
	GeneratedAt   string `json:"generatedAt"`
	Project       string `json:"project"`
	Version       string `json:"version"`
	MinSeverity   string `json:"minSeverity"`
	TotalPackages int    `json:"totalPackages"`
	TotalVulns    int    `json:"totalVulns"`
	Critical      int    `json:"critical"`
	High          int    `json:"high"`
	Rows          []Row  `json:"rows"`
}

// Generate filters findings, fetches latest versions, and produces a report.
func Generate(client *dtrack.Client, projectUUID, projectName, projectVersion, minSeverity string) ([]Row, error) {
	findings, err := client.GetFindings(projectUUID)
	if err != nil {
		return nil, err
	}

	minRank := severityRank[strings.ToUpper(minSeverity)]

	// Group by purl (component key)
	type compKey = string
	grouped := make(map[compKey][]dtrack.Finding)
	purlToInfo := make(map[compKey]dtrack.Finding)

	for _, f := range findings {
		if f.Analysis.IsSuppressed {
			continue
		}
		rank, ok := severityRank[f.Vulnerability.Severity]
		if !ok || rank < minRank {
			continue
		}
		key := f.Component.Purl
		if key == "" {
			key = f.Component.Name + "@" + f.Component.Version
		}
		grouped[key] = append(grouped[key], f)
		purlToInfo[key] = f
	}

	rows := make([]Row, 0, len(grouped))
	for key, fs := range grouped {
		info := purlToInfo[key]

		var latestVer string
		if info.Component.Purl != "" {
			latestVer, _ = client.GetLatestVersion(info.Component.Purl)
		}

		var vulns []VulnSummary
		worstRank := -1
		worstCvss := 0.0
		worstSev := ""
		for _, f := range fs {
			cvss := f.Vulnerability.CvssV3
			if cvss == 0 {
				cvss = f.Vulnerability.CvssV2
			}
			vulns = append(vulns, VulnSummary{
				VulnId:   f.Vulnerability.VulnId,
				Severity: f.Vulnerability.Severity,
				Cvss:     cvss,
			})
			r := severityRank[f.Vulnerability.Severity]
			if r > worstRank {
				worstRank = r
				worstSev = f.Vulnerability.Severity
				worstCvss = cvss
			}
		}
		// Sort vulns by severity desc
		sort.Slice(vulns, func(i, j int) bool {
			return severityRank[vulns[i].Severity] > severityRank[vulns[j].Severity]
		})

		pkg := info.Component.Name
		if info.Component.Group != "" && !strings.HasPrefix(pkg, info.Component.Group) {
			pkg = info.Component.Group + "/" + pkg
		}

		rows = append(rows, Row{
			Package:       pkg,
			CurrentVer:    info.Component.Version,
			LatestVer:     latestVer,
			Purl:          info.Component.Purl,
			Vulns:         vulns,
			WorseSeverity: worstSev,
			WorseCvss:     worstCvss,
		})
	}

	// Sort rows by worst severity desc, then package name
	sort.Slice(rows, func(i, j int) bool {
		ri := severityRank[rows[i].WorseSeverity]
		rj := severityRank[rows[j].WorseSeverity]
		if ri != rj {
			return ri > rj
		}
		return rows[i].Package < rows[j].Package
	})

	return rows, nil
}

// PrintConsole prints a formatted table to stdout.
func PrintConsole(rows []Row, projectName, projectVersion, minSeverity string) {
	now := time.Now().Format("2006-01-02 15:04:05")

	critCount, highCount, totalVulns := 0, 0, 0
	for _, r := range rows {
		for _, v := range r.Vulns {
			totalVulns++
			switch v.Severity {
			case "CRITICAL":
				critCount++
			case "HIGH":
				highCount++
			}
		}
	}

	fmt.Printf("\n취약점 리포트: %s @ %s\n", projectName, projectVersion)
	fmt.Printf("조회 시각: %s\n", now)
	fmt.Printf("필터: %s 이상\n\n", minSeverity)

	if len(rows) == 0 {
		fmt.Printf("✓ %s 이상 취약점이 발견되지 않았습니다.\n", minSeverity)
		return
	}

	// Determine column widths
	colPkg := len("패키지")
	colCur := len("현재 버전")
	colLat := len("권장 버전")
	colCve := len("CVE")
	colSev := len("심각도")

	for _, r := range rows {
		if w := utf8.RuneCountInString(r.Package); w > colPkg {
			colPkg = w
		}
		if w := len(r.CurrentVer); w > colCur {
			colCur = w
		}
		if w := len(r.LatestVer); w > colLat {
			colLat = w
		}
		for _, v := range r.Vulns {
			if w := len(v.VulnId); w > colCve {
				colCve = w
			}
		}
	}

	sep := fmt.Sprintf("+-%-*s-+-%-*s-+-%-*s-+-%-*s-+-%-*s-+-------+",
		colPkg, strings.Repeat("-", colPkg),
		colCur, strings.Repeat("-", colCur),
		colLat, strings.Repeat("-", colLat),
		colCve, strings.Repeat("-", colCve),
		colSev, strings.Repeat("-", colSev),
	)

	header := fmt.Sprintf("| %-*s | %-*s | %-*s | %-*s | %-*s | CVSS  |",
		colPkg, "패키지",
		colCur, "현재 버전",
		colLat, "권장 버전",
		colCve, "CVE",
		colSev, "심각도",
	)

	fmt.Println(sep)
	fmt.Println(header)
	fmt.Println(sep)

	for _, r := range rows {
		latVer := r.LatestVer
		if latVer == "" {
			latVer = "-"
		}
		for i, v := range r.Vulns {
			cvssStr := "-"
			if v.Cvss > 0 {
				cvssStr = fmt.Sprintf("%.1f", v.Cvss)
			}
			if i == 0 {
				// First vuln row: show package + version info
				pkgRunes := []rune(r.Package)
				pkgPadded := string(pkgRunes)
				padding := colPkg - utf8.RuneCountInString(r.Package)
				if padding > 0 {
					pkgPadded += strings.Repeat(" ", padding)
				}
				fmt.Printf("| %s | %-*s | %-*s | %-*s | %-*s | %-5s |\n",
					pkgPadded,
					colCur, r.CurrentVer,
					colLat, latVer,
					colCve, v.VulnId,
					colSev, v.Severity,
					cvssStr,
				)
			} else {
				// Continuation rows: package column blank
				fmt.Printf("| %-*s | %-*s | %-*s | %-*s | %-*s | %-5s |\n",
					colPkg, "",
					colCur, "",
					colLat, "",
					colCve, v.VulnId,
					colSev, v.Severity,
					cvssStr,
				)
			}
		}
	}

	fmt.Println(sep)
	fmt.Printf("\n총 %d개 패키지, %d개 취약점 (CRITICAL: %d, HIGH: %d)\n",
		len(rows), totalVulns, critCount, highCount)
}

// SaveMarkdown writes a Markdown report to the given path.
func SaveMarkdown(rows []Row, projectName, projectVersion, minSeverity, path string) error {
	var sb strings.Builder

	now := time.Now().Format("2006-01-02 15:04:05")
	critCount, highCount, totalVulns := 0, 0, 0
	for _, r := range rows {
		for _, v := range r.Vulns {
			totalVulns++
			switch v.Severity {
			case "CRITICAL":
				critCount++
			case "HIGH":
				highCount++
			}
		}
	}

	fmt.Fprintf(&sb, "# 취약점 리포트: %s @ %s\n\n", projectName, projectVersion)
	fmt.Fprintf(&sb, "- **조회 시각**: %s\n", now)
	fmt.Fprintf(&sb, "- **필터**: %s 이상\n", minSeverity)
	fmt.Fprintf(&sb, "- **총 패키지**: %d개, **총 취약점**: %d개 (CRITICAL: %d, HIGH: %d)\n\n",
		len(rows), totalVulns, critCount, highCount)

	if len(rows) == 0 {
		sb.WriteString("✓ 해당 심각도 이상의 취약점이 발견되지 않았습니다.\n")
	} else {
		sb.WriteString("| 패키지 | 현재 버전 | 권장 버전 | CVE | 심각도 | CVSS |\n")
		sb.WriteString("|--------|----------|----------|-----|--------|------|\n")
		for _, r := range rows {
			latVer := r.LatestVer
			if latVer == "" {
				latVer = "-"
			}
			for i, v := range r.Vulns {
				cvssStr := "-"
				if v.Cvss > 0 {
					cvssStr = fmt.Sprintf("%.1f", v.Cvss)
				}
				pkg := ""
				if i == 0 {
					pkg = r.Package
				}
				cur, lat := "", ""
				if i == 0 {
					cur = r.CurrentVer
					lat = latVer
				}
				fmt.Fprintf(&sb, "| %s | %s | %s | %s | %s | %s |\n",
					pkg, cur, lat, v.VulnId, v.Severity, cvssStr)
			}
		}
	}

	return os.WriteFile(path, []byte(sb.String()), 0644)
}

// SaveJSON writes a JSON report to the given path.
func SaveJSON(rows []Row, projectName, projectVersion, minSeverity, path string) error {
	critCount, highCount, totalVulns := 0, 0, 0
	for _, r := range rows {
		for _, v := range r.Vulns {
			totalVulns++
			switch v.Severity {
			case "CRITICAL":
				critCount++
			case "HIGH":
				highCount++
			}
		}
	}

	out := JSONReport{
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Project:       projectName,
		Version:       projectVersion,
		MinSeverity:   minSeverity,
		TotalPackages: len(rows),
		TotalVulns:    totalVulns,
		Critical:      critCount,
		High:          highCount,
		Rows:          rows,
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
