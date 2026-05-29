package dtrack

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type Finding struct {
	Component struct {
		Name    string `json:"name"`
		Group   string `json:"group"`
		Version string `json:"version"`
		Purl    string `json:"purl"`
	} `json:"component"`
	Vulnerability struct {
		VulnId         string  `json:"vulnId"`
		Severity       string  `json:"severity"`
		CvssV3         float64 `json:"cvssV3BaseScore"`
		CvssV2         float64 `json:"cvssV2BaseScore"`
		Recommendation string  `json:"recommendation"`
	} `json:"vulnerability"`
	Analysis struct {
		IsSuppressed bool `json:"isSuppressed"`
	} `json:"analysis"`
}

// GetFindings returns all non-suppressed findings for a project.
func (c *Client) GetFindings(projectUUID string) ([]Finding, error) {
	req, _ := http.NewRequest(http.MethodGet,
		c.BaseURL+"/api/v1/finding/project/"+projectUUID+"?suppressed=false", nil)
	req.Header.Set("X-Api-Key", c.APIKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get findings request failed: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get findings failed (%d): %s", resp.StatusCode, string(data))
	}

	var findings []Finding
	if err := json.Unmarshal(data, &findings); err != nil {
		return nil, fmt.Errorf("parse findings response: %w", err)
	}
	return findings, nil
}

// GetLatestVersion queries the repository metadata for the latest known version of a component.
// Returns empty string if no metadata is available.
func (c *Client) GetLatestVersion(purl string) (string, error) {
	req, _ := http.NewRequest(http.MethodGet,
		c.BaseURL+"/api/v1/repository/latest?purl="+url.QueryEscape(purl), nil)
	req.Header.Set("X-Api-Key", c.APIKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("get latest version request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusNoContent {
		return "", nil
	}

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("get latest version failed (%d): %s", resp.StatusCode, string(data))
	}

	var meta struct {
		LatestVersion string `json:"latestVersion"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return "", fmt.Errorf("parse repository metadata: %w", err)
	}
	return meta.LatestVersion, nil
}
