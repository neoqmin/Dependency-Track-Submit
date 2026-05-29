package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	dtclient "github.com/pribit/dtrack-submit/internal/dtrack"
)

// DTrackClient wraps the dtrack-submit client and adds list/create operations.
type DTrackClient struct {
	inner   *dtclient.Client
	baseURL string
	apiKey  string
	http    *http.Client
}

func NewDTrackClient(baseURL, apiKey string) *DTrackClient {
	base := strings.TrimRight(baseURL, "/")
	return &DTrackClient{
		inner:   dtclient.New(base, apiKey),
		baseURL: base,
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 60 * time.Second},
	}
}

// Project is the public project descriptor.
type Project struct {
	UUID          string
	Name          string
	Version       string
	Metrics       *ProjectMetrics
	LastBOMImport int64
}

// ProjectMetrics holds vulnerability counts.
type ProjectMetrics struct {
	Critical           int
	High               int
	Medium             int
	Low                int
	Unassigned         int
	InheritedRiskScore float64
}

// Finding is the public finding type mirroring dtrack-submit's Finding.
type Finding struct {
	Component     FindingComponent
	Vulnerability FindingVuln
	Analysis      FindingAnalysis
}

type FindingComponent struct {
	Name          string
	Group         string
	Version       string
	PURL          string
	LatestVersion string
}

type FindingVuln struct {
	VulnID         string
	Severity       string
	CVSSv3         float64
	CVSSv2         float64
	Description    string
	Recommendation string
}

type FindingAnalysis struct {
	Suppressed bool
}

// projectListItem is the raw JSON shape from GET /api/v1/project.
type projectListItem struct {
	UUID    string `json:"uuid"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Metrics *struct {
		Critical           int     `json:"critical"`
		High               int     `json:"high"`
		Medium             int     `json:"medium"`
		Low                int     `json:"low"`
		Unassigned         int     `json:"unassigned"`
		InheritedRiskScore float64 `json:"inheritedRiskScore"`
	} `json:"metrics"`
	LastBOMImport int64 `json:"lastBomImport"`
}

func (c *DTrackClient) get(path string, out interface{}) error {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", c.apiKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(data))
	}
	return json.Unmarshal(data, out)
}

func (c *DTrackClient) ListProjects(ctx context.Context) ([]Project, error) {
	_ = ctx
	var all []projectListItem
	page := 1
	for {
		var page_items []projectListItem
		path := fmt.Sprintf("/api/v1/project?limit=100&page=%d", page)
		if err := c.get(path, &page_items); err != nil {
			if page == 1 {
				return nil, err
			}
			break
		}
		if len(page_items) == 0 {
			break
		}
		all = append(all, page_items...)
		if len(page_items) < 100 {
			break
		}
		page++
	}

	projects := make([]Project, 0, len(all))
	for _, item := range all {
		p := Project{
			UUID:          item.UUID,
			Name:          item.Name,
			Version:       item.Version,
			LastBOMImport: item.LastBOMImport,
		}
		if item.Metrics != nil {
			p.Metrics = &ProjectMetrics{
				Critical:           item.Metrics.Critical,
				High:               item.Metrics.High,
				Medium:             item.Metrics.Medium,
				Low:                item.Metrics.Low,
				Unassigned:         item.Metrics.Unassigned,
				InheritedRiskScore: item.Metrics.InheritedRiskScore,
			}
		}
		projects = append(projects, p)
	}
	return projects, nil
}

func (c *DTrackClient) LookupProject(ctx context.Context, name, version string) (*Project, error) {
	_ = ctx
	q := "/api/v1/project/lookup?name=" + url.QueryEscape(name)
	if version != "" {
		q += "&version=" + url.QueryEscape(version)
	}
	var item projectListItem
	if err := c.get(q, &item); err != nil {
		msg := err.Error()
		if strings.Contains(msg, "HTTP 404") || strings.Contains(msg, "Not Found") {
			return nil, nil
		}
		return nil, err
	}
	if item.UUID == "" {
		return nil, nil
	}
	p := &Project{UUID: item.UUID, Name: item.Name, Version: item.Version}
	return p, nil
}

func (c *DTrackClient) CreateProject(ctx context.Context, name, version string) (*Project, error) {
	_ = ctx
	uuid, err := c.inner.EnsureProject(name, version)
	if err != nil {
		return nil, err
	}
	return &Project{UUID: uuid, Name: name, Version: version}, nil
}

// UploadBOM uploads a BOM file from disk and waits for processing.
func (c *DTrackClient) UploadBOMFile(ctx context.Context, projectUUID, bomPath string) error {
	_ = ctx
	token, err := c.inner.UploadBOM(projectUUID, bomPath)
	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}
	return c.inner.WaitForProcessing(token)
}

func (c *DTrackClient) GetFindings(ctx context.Context, projectUUID string) ([]Finding, error) {
	_ = ctx
	raw, err := c.inner.GetFindings(projectUUID)
	if err != nil {
		return nil, err
	}
	findings := make([]Finding, 0, len(raw))
	for _, r := range raw {
		f := Finding{
			Component: FindingComponent{
				Name:    r.Component.Name,
				Group:   r.Component.Group,
				Version: r.Component.Version,
				PURL:    r.Component.Purl,
			},
			Vulnerability: FindingVuln{
				VulnID:         r.Vulnerability.VulnId,
				Severity:       r.Vulnerability.Severity,
				CVSSv3:         r.Vulnerability.CvssV3,
				CVSSv2:         r.Vulnerability.CvssV2,
				Recommendation: r.Vulnerability.Recommendation,
			},
			Analysis: FindingAnalysis{Suppressed: r.Analysis.IsSuppressed},
		}
		// Fetch latest version for each component that has a PURL
		if r.Component.Purl != "" {
			latest, _ := c.inner.GetLatestVersion(r.Component.Purl)
			f.Component.LatestVersion = latest
		}
		findings = append(findings, f)
	}
	return findings, nil
}

func (c *DTrackClient) GetMetrics(ctx context.Context, projectUUID string) (*ProjectMetrics, error) {
	_ = ctx
	proj, err := c.inner.GetProjectMetrics(projectUUID)
	if err != nil {
		return nil, err
	}
	m := &ProjectMetrics{}
	if proj.Metrics != nil {
		m.Critical = proj.Metrics.Critical
		m.High = proj.Metrics.High
		m.Medium = proj.Metrics.Medium
		m.Low = proj.Metrics.Low
	}
	return m, nil
}

// RawClient returns the underlying dtrack-submit client.
func (c *DTrackClient) RawClient() *dtclient.Client {
	return c.inner
}
