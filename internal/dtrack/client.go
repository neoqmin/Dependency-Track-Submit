package dtrack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Client struct {
	BaseURL string
	APIKey  string
	http    *http.Client
}

func New(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		http:    &http.Client{Timeout: 60 * time.Second},
	}
}

type projectResponse struct {
	UUID    string `json:"uuid"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Metrics *struct {
		Components          int `json:"components"`
		Vulnerabilities     int `json:"vulnerabilities"`
		Critical            int `json:"critical"`
		High                int `json:"high"`
		Medium              int `json:"medium"`
		Low                 int `json:"low"`
	} `json:"metrics"`
}

// EnsureProject creates a project or returns the UUID of an existing one.
func (c *Client) EnsureProject(name, version string) (string, error) {
	body, _ := json.Marshal(map[string]string{
		"name":       name,
		"version":    version,
		"classifier": "APPLICATION",
	})
	req, _ := http.NewRequest(http.MethodPut, c.BaseURL+"/api/v1/project", bytes.NewReader(body))
	req.Header.Set("X-Api-Key", c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("create project request failed: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("create project failed (%d): %s", resp.StatusCode, string(data))
	}

	var proj projectResponse
	if err := json.Unmarshal(data, &proj); err != nil {
		return "", fmt.Errorf("parse project response: %w", err)
	}
	return proj.UUID, nil
}

// UploadBOM uploads a CycloneDX JSON file and returns the processing token.
func (c *Client) UploadBOM(projectUUID, bomPath string) (string, error) {
	f, err := os.Open(bomPath)
	if err != nil {
		return "", fmt.Errorf("open bom file: %w", err)
	}
	defer f.Close()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("project", projectUUID)
	fw, err := w.CreateFormFile("bom", filepath.Base(bomPath))
	if err != nil {
		return "", err
	}
	if _, err = io.Copy(fw, f); err != nil {
		return "", err
	}
	w.Close()

	req, _ := http.NewRequest(http.MethodPost, c.BaseURL+"/api/v1/bom", &buf)
	req.Header.Set("X-Api-Key", c.APIKey)
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("upload bom request failed: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("upload bom failed (%d): %s", resp.StatusCode, string(data))
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("parse upload response: %w", err)
	}
	return result.Token, nil
}

// WaitForProcessing polls until the BOM token is processed.
func (c *Client) WaitForProcessing(token string) error {
	url := c.BaseURL + "/api/v1/bom/token/" + token
	for i := 0; i < 60; i++ {
		time.Sleep(2 * time.Second)
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("X-Api-Key", c.APIKey)
		resp, err := c.http.Do(req)
		if err != nil {
			continue
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var result struct {
			Processing bool `json:"processing"`
		}
		if err := json.Unmarshal(data, &result); err != nil {
			continue
		}
		if !result.Processing {
			return nil
		}
	}
	return fmt.Errorf("timed out waiting for BOM processing")
}

// GetProjectMetrics returns component and vulnerability counts for a project.
func (c *Client) GetProjectMetrics(projectUUID string) (*projectResponse, error) {
	req, _ := http.NewRequest(http.MethodGet, c.BaseURL+"/api/v1/project/"+projectUUID, nil)
	req.Header.Set("X-Api-Key", c.APIKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var proj projectResponse
	if err := json.Unmarshal(data, &proj); err != nil {
		return nil, err
	}
	return &proj, nil
}
