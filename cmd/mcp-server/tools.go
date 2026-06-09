package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/pribit/dtrack-submit/internal/detector"
	"github.com/pribit/dtrack-submit/internal/generator"
	"github.com/pribit/dtrack-submit/internal/report"
)

type toolDef struct {
	Name        string
	Description string
	InputSchema map[string]any
	Handler     func(ctx context.Context, args map[string]any) (string, error)
}

func allTools(c *DTrackClient) []toolDef {
	return []toolDef{
		listProjectsTool(c),
		getFindingsTool(c),
		uploadBOMTool(c),
		getMetricsTool(c),
		getRemediationTool(c),
		submitProjectTool(c),
		generateReportTool(c),
		checkUpdateTool(),
	}
}

// ── dtrack_check_update ───────────────────────────────────────────────────────

func checkUpdateTool() toolDef {
	return toolDef{
		Name:        "dtrack_check_update",
		Description: "dtrack-mcp-server의 최신 릴리스를 확인하고, 새 버전이 있으면 자가 업데이트합니다. apply=false(기본)면 확인만 하고, apply=true면 실제로 교체합니다. 교체 후에는 MCP 서버를 재시작해야 적용됩니다.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"apply": map[string]any{"type": "boolean", "description": "true면 새 버전을 실제로 다운로드하여 교체합니다 (기본 false: 확인만)"},
			},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			apply, _ := args["apply"].(bool)

			rel, err := checkForUpdate(ctx)
			if err != nil {
				return "", fmt.Errorf("업데이트 확인 실패: %w", err)
			}
			if rel == nil {
				return fmt.Sprintf("이미 최신 버전입니다 (현재: %s).", version), nil
			}

			if !apply {
				return fmt.Sprintf("새 버전이 있습니다: %s → %s\n  릴리스: %s\n업데이트하려면 apply=true로 다시 호출하세요. (교체 후 MCP 서버 재시작 필요)",
					version, rel.TagName, rel.HTMLURL), nil
			}

			if err := applyUpdate(ctx, rel); err != nil {
				return "", fmt.Errorf("업데이트 적용 실패: %w", err)
			}
			logf("self-update applied: %s → %s", version, rel.TagName)
			return fmt.Sprintf("✓ 업데이트 완료: %s → %s\n  MCP 서버를 재시작하면 새 버전이 적용됩니다.", version, rel.TagName), nil
		},
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

func argString(args map[string]any, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func severityOrder(s string) int {
	switch strings.ToUpper(s) {
	case "CRITICAL":
		return 0
	case "HIGH":
		return 1
	case "MEDIUM":
		return 2
	case "LOW":
		return 3
	default:
		return 4
	}
}

// ── 1. dtrack_list_projects ───────────────────────────────────────────────────

func listProjectsTool(c *DTrackClient) toolDef {
	return toolDef{
		Name:        "dtrack_list_projects",
		Description: "Dependency-Track에 등록된 프로젝트 목록을 조회합니다.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			projects, err := c.ListProjects(ctx)
			if err != nil {
				return "", fmt.Errorf("프로젝트 목록 조회 실패: %w", err)
			}
			if len(projects) == 0 {
				return "등록된 프로젝트가 없습니다.", nil
			}
			var sb strings.Builder
			fmt.Fprintf(&sb, "Dependency-Track 프로젝트 목록 (%d개)\n\n", len(projects))
			for _, p := range projects {
				ver := p.Version
				if ver == "" {
					ver = "-"
				}
				fmt.Fprintf(&sb, "• %s  (버전: %s)\n", p.Name, ver)
				if p.Metrics != nil {
					m := p.Metrics
					fmt.Fprintf(&sb, "  취약점: CRITICAL=%d HIGH=%d MEDIUM=%d LOW=%d  위험점수=%.1f\n",
						m.Critical, m.High, m.Medium, m.Low, m.InheritedRiskScore)
				}
				if p.LastBOMImport > 0 {
					t := time.UnixMilli(p.LastBOMImport)
					fmt.Fprintf(&sb, "  마지막 BOM 업로드: %s\n", t.Format("2006-01-02 15:04"))
				}
			}
			return sb.String(), nil
		},
	}
}

// ── 2. dtrack_get_findings ────────────────────────────────────────────────────

func getFindingsTool(c *DTrackClient) toolDef {
	return toolDef{
		Name:        "dtrack_get_findings",
		Description: "Dependency-Track에서 프로젝트의 취약점(findings) 목록을 조회합니다. severity로 필터링 가능.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_name":    map[string]any{"type": "string", "description": "프로젝트 이름"},
				"project_version": map[string]any{"type": "string", "description": "프로젝트 버전 (생략 시 최신)"},
				"severity":        map[string]any{"type": "string", "description": "CRITICAL, HIGH, MEDIUM, LOW, all (기본: all)"},
			},
			"required": []string{"project_name"},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			name := argString(args, "project_name")
			version := argString(args, "project_version")
			filter := strings.ToUpper(argString(args, "severity"))
			if filter == "" || filter == "ALL" {
				filter = ""
			}

			project, err := c.LookupProject(ctx, name, version)
			if err != nil {
				return "", fmt.Errorf("프로젝트 조회 실패: %w", err)
			}
			if project == nil {
				return fmt.Sprintf("프로젝트 '%s'를 찾을 수 없습니다. SBOM을 먼저 업로드하세요 (dtrack_upload_bom).", name), nil
			}

			findings, err := c.GetFindings(ctx, project.UUID)
			if err != nil {
				return "", fmt.Errorf("취약점 조회 실패: %w", err)
			}

			var filtered []Finding
			for _, f := range findings {
				if f.Analysis.Suppressed {
					continue
				}
				if filter != "" && strings.ToUpper(f.Vulnerability.Severity) != filter {
					continue
				}
				filtered = append(filtered, f)
			}

			if len(filtered) == 0 {
				if filter != "" {
					return fmt.Sprintf("프로젝트 '%s'에서 %s 수준 취약점이 없습니다.", name, filter), nil
				}
				return fmt.Sprintf("프로젝트 '%s'에서 취약점이 발견되지 않았습니다.", name), nil
			}

			sort.Slice(filtered, func(i, j int) bool {
				si := severityOrder(filtered[i].Vulnerability.Severity)
				sj := severityOrder(filtered[j].Vulnerability.Severity)
				if si != sj {
					return si < sj
				}
				return filtered[i].Vulnerability.VulnID < filtered[j].Vulnerability.VulnID
			})

			var sb strings.Builder
			fmt.Fprintf(&sb, "프로젝트: %s %s\n취약점 %d건 발견\n\n", project.Name, project.Version, len(filtered))

			counts := map[string]int{}
			for _, f := range filtered {
				counts[strings.ToUpper(f.Vulnerability.Severity)]++
			}
			for _, sev := range []string{"CRITICAL", "HIGH", "MEDIUM", "LOW"} {
				if n := counts[sev]; n > 0 {
					fmt.Fprintf(&sb, "%s: %d건\n", sev, n)
				}
			}
			sb.WriteString("\n")

			for _, f := range filtered {
				vuln := f.Vulnerability
				comp := f.Component
				fmt.Fprintf(&sb, "[%s] %s\n", vuln.Severity, vuln.VulnID)
				fmt.Fprintf(&sb, "  컴포넌트: %s %s\n", comp.Name, comp.Version)
				if comp.LatestVersion != "" && comp.LatestVersion != comp.Version {
					fmt.Fprintf(&sb, "  최신버전: %s\n", comp.LatestVersion)
				}
				if vuln.CVSSv3 > 0 {
					fmt.Fprintf(&sb, "  CVSSv3: %.1f\n", vuln.CVSSv3)
				}
				if vuln.Recommendation != "" {
					rec := vuln.Recommendation
					if len(rec) > 200 {
						rec = rec[:200] + "..."
					}
					fmt.Fprintf(&sb, "  권고: %s\n", rec)
				}
				sb.WriteString("\n")
			}
			return sb.String(), nil
		},
	}
}

// ── 3. dtrack_upload_bom ──────────────────────────────────────────────────────

func uploadBOMTool(c *DTrackClient) toolDef {
	return toolDef{
		Name:        "dtrack_upload_bom",
		Description: "현재 작업 디렉토리의 SBOM을 Dependency-Track에 업로드합니다. syft가 있으면 사용하고, 없으면 lockfile을 파싱합니다.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_name":    map[string]any{"type": "string", "description": "프로젝트 이름"},
				"project_version": map[string]any{"type": "string", "description": "프로젝트 버전 (기본: SNAPSHOT)"},
				"working_dir":     map[string]any{"type": "string", "description": "작업 디렉토리 경로 (기본: 현재 디렉토리)"},
			},
			"required": []string{"project_name"},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			name := argString(args, "project_name")
			version := argString(args, "project_version")
			workDir := argString(args, "working_dir")
			if version == "" {
				version = "SNAPSHOT"
			}

			bomPath, method, err := generateBOMFile(ctx, workDir)
			if err != nil {
				return "", fmt.Errorf("SBOM 생성 실패: %w", err)
			}
			defer os.Remove(bomPath)

			project, err := c.LookupProject(ctx, name, version)
			if err != nil {
				return "", fmt.Errorf("프로젝트 조회 실패: %w", err)
			}
			created := false
			if project == nil {
				project, err = c.CreateProject(ctx, name, version)
				if err != nil {
					return "", fmt.Errorf("프로젝트 생성 실패: %w", err)
				}
				created = true
			}

			if err := c.UploadBOMFile(ctx, project.UUID, bomPath); err != nil {
				return "", fmt.Errorf("BOM 업로드/처리 실패: %w", err)
			}

			action := "업데이트"
			if created {
				action = "새로 생성"
			}
			return fmt.Sprintf("✓ BOM 업로드 및 분석 완료\n  프로젝트: %s %s (%s)\n  생성 방법: %s\n  이제 dtrack_get_findings로 취약점을 조회하세요.",
				name, version, action, method), nil
		},
	}
}

// ── 4. dtrack_get_project_metrics ─────────────────────────────────────────────

func getMetricsTool(c *DTrackClient) toolDef {
	return toolDef{
		Name:        "dtrack_get_project_metrics",
		Description: "Dependency-Track 프로젝트의 취약점 통계 및 위험 점수를 조회합니다.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_name":    map[string]any{"type": "string", "description": "프로젝트 이름"},
				"project_version": map[string]any{"type": "string", "description": "프로젝트 버전 (생략 시 최신)"},
			},
			"required": []string{"project_name"},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			name := argString(args, "project_name")
			version := argString(args, "project_version")

			project, err := c.LookupProject(ctx, name, version)
			if err != nil {
				return "", fmt.Errorf("프로젝트 조회 실패: %w", err)
			}
			if project == nil {
				return fmt.Sprintf("프로젝트 '%s'를 찾을 수 없습니다.", name), nil
			}

			metrics, err := c.GetMetrics(ctx, project.UUID)
			if err != nil {
				return "", fmt.Errorf("메트릭 조회 실패: %w", err)
			}

			total := metrics.Critical + metrics.High + metrics.Medium + metrics.Low + metrics.Unassigned
			return fmt.Sprintf("프로젝트: %s %s\n\n취약점 통계\n  CRITICAL : %d\n  HIGH     : %d\n  MEDIUM   : %d\n  LOW      : %d\n  미분류   : %d\n  합계     : %d\n\n위험 점수 (Inherited Risk Score): %.1f",
				project.Name, project.Version,
				metrics.Critical, metrics.High, metrics.Medium, metrics.Low, metrics.Unassigned,
				total, metrics.InheritedRiskScore), nil
		},
	}
}

// ── 5. dtrack_get_component_remediation ───────────────────────────────────────

func getRemediationTool(c *DTrackClient) toolDef {
	return toolDef{
		Name:        "dtrack_get_component_remediation",
		Description: "특정 컴포넌트의 취약점과 수정 버전 정보를 조회합니다.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_name":    map[string]any{"type": "string", "description": "프로젝트 이름"},
				"project_version": map[string]any{"type": "string", "description": "프로젝트 버전 (생략 시 최신)"},
				"component_name":  map[string]any{"type": "string", "description": "컴포넌트(패키지) 이름"},
			},
			"required": []string{"project_name", "component_name"},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			projName := argString(args, "project_name")
			projVersion := argString(args, "project_version")
			compName := strings.ToLower(argString(args, "component_name"))

			project, err := c.LookupProject(ctx, projName, projVersion)
			if err != nil {
				return "", fmt.Errorf("프로젝트 조회 실패: %w", err)
			}
			if project == nil {
				return fmt.Sprintf("프로젝트 '%s'를 찾을 수 없습니다.", projName), nil
			}

			findings, err := c.GetFindings(ctx, project.UUID)
			if err != nil {
				return "", fmt.Errorf("취약점 조회 실패: %w", err)
			}

			var matched []Finding
			for _, f := range findings {
				if strings.Contains(strings.ToLower(f.Component.Name), compName) {
					matched = append(matched, f)
				}
			}

			if len(matched) == 0 {
				return fmt.Sprintf("컴포넌트 '%s'에서 취약점이 발견되지 않았습니다.", compName), nil
			}

			comp := matched[0].Component
			var sb strings.Builder
			fmt.Fprintf(&sb, "컴포넌트: %s\n현재 버전: %s\n", comp.Name, comp.Version)
			if comp.LatestVersion != "" {
				fmt.Fprintf(&sb, "최신 버전: %s\n", comp.LatestVersion)
			}
			if comp.PURL != "" {
				fmt.Fprintf(&sb, "PURL: %s\n", comp.PURL)
			}
			fmt.Fprintf(&sb, "\n취약점 %d건:\n", len(matched))

			sort.Slice(matched, func(i, j int) bool {
				return severityOrder(matched[i].Vulnerability.Severity) < severityOrder(matched[j].Vulnerability.Severity)
			})

			for _, f := range matched {
				v := f.Vulnerability
				fmt.Fprintf(&sb, "\n  [%s] %s", v.Severity, v.VulnID)
				if v.CVSSv3 > 0 {
					fmt.Fprintf(&sb, " (CVSSv3: %.1f)", v.CVSSv3)
				}
				sb.WriteString("\n")
				if v.Recommendation != "" {
					rec := v.Recommendation
					if len(rec) > 300 {
						rec = rec[:300] + "..."
					}
					fmt.Fprintf(&sb, "  권고: %s\n", rec)
				}
			}
			return sb.String(), nil
		},
	}
}

// ── 6. dtrack_submit_project ──────────────────────────────────────────────────

func submitProjectTool(c *DTrackClient) toolDef {
	return toolDef{
		Name:        "dtrack_submit_project",
		Description: "프로젝트 디렉토리를 자동 감지해 SBOM을 생성하고 Dependency-Track에 등록 및 분석을 요청합니다. dtrack-submit의 프로젝트 타입 감지 로직(Maven, Gradle, Go, .NET, NPM 등)을 사용합니다.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"working_dir":     map[string]any{"type": "string", "description": "프로젝트 디렉토리 (기본: 현재 디렉토리)"},
				"project_name":    map[string]any{"type": "string", "description": "프로젝트 이름 (생략 시 자동 감지)"},
				"project_version": map[string]any{"type": "string", "description": "프로젝트 버전 (생략 시 자동 감지)"},
			},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			workDir := argString(args, "working_dir")
			nameOverride := argString(args, "project_name")
			versionOverride := argString(args, "project_version")

			if workDir == "" {
				var err error
				workDir, err = os.Getwd()
				if err != nil {
					return "", fmt.Errorf("작업 디렉토리 확인 실패: %w", err)
				}
			}

			// Detect project type and metadata
			info, err := detector.Detect(workDir)
			if err != nil {
				return "", fmt.Errorf("프로젝트 감지 실패: %w", err)
			}

			projName := nameOverride
			if projName == "" {
				projName = info.Name
			}
			projVersion := versionOverride
			if projVersion == "" {
				projVersion = info.Version
			}
			if projName == "" {
				projName = "unknown"
			}
			if projVersion == "" {
				projVersion = "SNAPSHOT"
			}

			// Select appropriate generator
			gen := selectGenerator(info)
			if gen == nil || !gen.Available() {
				// Fall back to BOM file generation (syft / lockfile)
				bomPath, method, err := generateBOMFile(ctx, workDir)
				if err != nil {
					return "", fmt.Errorf("SBOM 생성 실패 (감지된 타입: %s): %w", info.Type, err)
				}
				defer os.Remove(bomPath)
				return submitBOMFile(ctx, c, bomPath, projName, projVersion, method)
			}

			// Use the dedicated generator
			tmp, err := os.CreateTemp("", "bom-*.json")
			if err != nil {
				return "", fmt.Errorf("임시 파일 생성 실패: %w", err)
			}
			tmp.Close()
			bomPath := tmp.Name()
			defer os.Remove(bomPath)

			if err := gen.Generate(workDir, bomPath); err != nil {
				// Generator failed — fall back
				bomPath2, method2, err2 := generateBOMFile(ctx, workDir)
				if err2 != nil {
					return "", fmt.Errorf("SBOM 생성 실패 (%s 실패, fallback도 실패): %w", gen.Name(), err2)
				}
				defer os.Remove(bomPath2)
				return submitBOMFile(ctx, c, bomPath2, projName, projVersion, "fallback/"+method2)
			}

			return submitBOMFile(ctx, c, bomPath, projName, projVersion, gen.Name())
		},
	}
}

func submitBOMFile(ctx context.Context, c *DTrackClient, bomPath, name, version, method string) (string, error) {
	project, err := c.LookupProject(ctx, name, version)
	if err != nil {
		return "", fmt.Errorf("프로젝트 조회 실패: %w", err)
	}
	created := false
	if project == nil {
		project, err = c.CreateProject(ctx, name, version)
		if err != nil {
			return "", fmt.Errorf("프로젝트 생성 실패: %w", err)
		}
		created = true
	}

	if err := c.UploadBOMFile(ctx, project.UUID, bomPath); err != nil {
		return "", fmt.Errorf("BOM 업로드/처리 실패: %w", err)
	}

	action := "업데이트"
	if created {
		action = "새로 생성"
	}
	return fmt.Sprintf("✓ 프로젝트 등록 및 분석 완료\n  프로젝트: %s %s (%s)\n  BOM 생성기: %s\n  이제 dtrack_generate_report로 취약점 리포트를 조회하세요.",
		name, version, action, method), nil
}

// selectGenerator mirrors dtrack-submit's selectGenerator logic.
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
	case detector.TypeNpm:
		return &generator.NpmGenerator{}
	case detector.TypeCpp:
		return generator.NewCppGenerator()
	case detector.TypeCocoa:
		return generator.NewCocoaGenerator()
	case detector.TypeSwift:
		return generator.NewSwiftGenerator()
	default:
		return &generator.CdxgenGenerator{}
	}
}

// ── 7. dtrack_generate_report ─────────────────────────────────────────────────

func generateReportTool(c *DTrackClient) toolDef {
	return toolDef{
		Name:        "dtrack_generate_report",
		Description: "Dependency-Track 프로젝트의 취약점 리포트를 생성합니다. 패키지별로 그룹화하고 최신 버전 정보를 포함합니다.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_name":    map[string]any{"type": "string", "description": "프로젝트 이름"},
				"project_version": map[string]any{"type": "string", "description": "프로젝트 버전 (생략 시 최신)"},
				"min_severity":    map[string]any{"type": "string", "description": "최소 심각도 필터 (CRITICAL, HIGH, MEDIUM, LOW — 기본: HIGH)"},
			},
			"required": []string{"project_name"},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			name := argString(args, "project_name")
			version := argString(args, "project_version")
			minSev := strings.ToUpper(argString(args, "min_severity"))
			if minSev == "" {
				minSev = "HIGH"
			}

			project, err := c.LookupProject(ctx, name, version)
			if err != nil {
				return "", fmt.Errorf("프로젝트 조회 실패: %w", err)
			}
			if project == nil {
				return fmt.Sprintf("프로젝트 '%s'를 찾을 수 없습니다. dtrack_submit_project로 먼저 등록하세요.", name), nil
			}

			rows, err := report.Generate(c.RawClient(), project.UUID, project.Name, project.Version, minSev)
			if err != nil {
				return "", fmt.Errorf("리포트 생성 실패: %w", err)
			}

			if len(rows) == 0 {
				return fmt.Sprintf("✓ 프로젝트 '%s %s'에서 %s 이상 취약점이 발견되지 않았습니다.",
					project.Name, project.Version, minSev), nil
			}

			// Build text output
			var sb strings.Builder
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

			fmt.Fprintf(&sb, "취약점 리포트: %s @ %s\n", project.Name, project.Version)
			fmt.Fprintf(&sb, "필터: %s 이상  |  패키지 %d개, 취약점 %d개 (CRITICAL: %d, HIGH: %d)\n\n",
				minSev, len(rows), totalVulns, critCount, highCount)

			for _, r := range rows {
				latVer := r.LatestVer
				if latVer == "" {
					latVer = "알 수 없음"
				}
				needsUpdate := r.LatestVer != "" && r.LatestVer != r.CurrentVer
				updateMark := ""
				if needsUpdate {
					updateMark = " ← 업데이트 필요"
				}
				fmt.Fprintf(&sb, "📦 %s\n", r.Package)
				fmt.Fprintf(&sb, "   현재: %s  →  최신: %s%s\n", r.CurrentVer, latVer, updateMark)
				if r.Purl != "" {
					fmt.Fprintf(&sb, "   PURL: %s\n", r.Purl)
				}
				for _, v := range r.Vulns {
					cvssStr := ""
					if v.Cvss > 0 {
						cvssStr = fmt.Sprintf(" (CVSSv3: %.1f)", v.Cvss)
					}
					fmt.Fprintf(&sb, "   [%s] %s%s\n", v.Severity, v.VulnId, cvssStr)
				}
				sb.WriteString("\n")
			}

			return sb.String(), nil
		},
	}
}
