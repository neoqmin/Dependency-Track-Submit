package main

import (
	"context"
	"crypto"
	_ "crypto/sha256" // registers SHA256 for selfupdate's checksum verification
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/minio/selfupdate"
)

// version is the build version, set via -ldflags "-X main.version=vX.Y.Z" at
// release time. Dev builds report "dev" and never consider themselves outdated.
var version = "dev"

// updateRepo is the GitHub owner/repo that releases are published to. This is
// the hosting repo (case-sensitive in the API path), not the Go module path.
const updateRepo = "neoqmin/Dependency-Track-Submit"

// releaseInfo is the subset of the GitHub Releases API we care about.
type releaseInfo struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
		Size               int64  `json:"size"`
	} `json:"assets"`
}

// latestRelease fetches the latest published release from GitHub.
func latestRelease(ctx context.Context) (*releaseInfo, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", updateRepo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var rel releaseInfo
	if err := json.Unmarshal(body, &rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

// assetName is the raw binary name this build looks for in a release, matching
// the naming used by the release workflow, e.g. dtrack-mcp-server-windows-amd64.exe.
func assetName() string {
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	return fmt.Sprintf("dtrack-mcp-server-%s-%s%s", runtime.GOOS, runtime.GOARCH, suffix)
}

// parseSemver turns "v1.2.3" or "1.2.3" into [1,2,3]. Extra/non-numeric parts
// are treated as 0 so pre-release tags degrade gracefully.
func parseSemver(s string) [3]int {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	var out [3]int
	for i, p := range strings.SplitN(s, ".", 3) {
		if i > 2 {
			break
		}
		n, _ := strconv.Atoi(p)
		out[i] = n
	}
	return out
}

// isNewer reports whether tag (e.g. "v1.2.3") is a higher semver than the
// running build. Dev builds are always considered outdated by a real release.
func isNewer(tag string) bool {
	if version == "dev" {
		return true
	}
	latest := parseSemver(tag)
	cur := parseSemver(version)
	for i := 0; i < 3; i++ {
		if latest[i] != cur[i] {
			return latest[i] > cur[i]
		}
	}
	return false
}

// checkForUpdate returns the latest release if it is newer than the running
// build, or nil if up to date.
func checkForUpdate(ctx context.Context) (*releaseInfo, error) {
	rel, err := latestRelease(ctx)
	if err != nil {
		return nil, err
	}
	if rel.TagName == "" {
		return nil, fmt.Errorf("latest release has no tag")
	}
	if !isNewer(rel.TagName) {
		return nil, nil
	}
	return rel, nil
}

// applyUpdate downloads the matching raw binary from the release and replaces
// the running executable in place. On Windows the live exe is moved aside
// (handled by selfupdate); the new version takes effect on next launch.
func applyUpdate(ctx context.Context, rel *releaseInfo) error {
	want := assetName()
	var dlURL string
	for _, a := range rel.Assets {
		if a.Name == want {
			dlURL = a.BrowserDownloadURL
			break
		}
	}
	if dlURL == "" {
		return fmt.Errorf("release %s has no asset %q (platform not built?)", rel.TagName, want)
	}

	checksum, _ := fetchChecksum(ctx, rel, want)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dlURL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download HTTP %d", resp.StatusCode)
	}

	opts := selfupdate.Options{}
	if len(checksum) > 0 {
		opts.Checksum = checksum
		opts.Hash = crypto.SHA256
	}
	if err := selfupdate.Apply(resp.Body, opts); err != nil {
		if rerr := selfupdate.RollbackError(err); rerr != nil {
			return fmt.Errorf("update failed and rollback also failed: %v (rollback: %v)", err, rerr)
		}
		return fmt.Errorf("update failed (rolled back): %w", err)
	}
	return nil
}

// fetchChecksum looks for a checksums.txt asset and returns the SHA256 for the
// named binary, if present. Returns nil (no error) when absent — checksum
// verification is a best-effort integrity guard, not a hard requirement.
func fetchChecksum(ctx context.Context, rel *releaseInfo, binName string) ([]byte, error) {
	var url string
	for _, a := range rel.Assets {
		if a.Name == "checksums.txt" {
			url = a.BrowserDownloadURL
			break
		}
	}
	if url == "" {
		return nil, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("checksums HTTP %d", resp.StatusCode)
	}
	data, _ := io.ReadAll(resp.Body)
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		// Format: "<sha256>  <filename>" (sha256sum output).
		if len(fields) == 2 && fields[1] == binName {
			sum, err := hex.DecodeString(fields[0])
			if err != nil {
				return nil, err
			}
			return sum, nil
		}
	}
	return nil, nil
}

// startupCheckInterval throttles the background check so frequent MCP server
// respawns don't burn GitHub's 60 req/hr unauthenticated rate limit.
const startupCheckInterval = 6 * time.Hour

// updateCheckStamp is the cache file recording when the last check ran.
func updateCheckStamp() string {
	return filepath.Join(os.TempDir(), "dtrack-mcp-update-check")
}

// recentlyChecked reports whether a startup check ran within the throttle window.
func recentlyChecked() bool {
	fi, err := os.Stat(updateCheckStamp())
	if err != nil {
		return false
	}
	return time.Since(fi.ModTime()) < startupCheckInterval
}

// startupUpdateCheck runs in the background at launch and logs (to stderr only)
// whether a newer release is available. It never blocks server startup, never
// writes to stdout (which carries JSON-RPC), is throttled to once per
// startupCheckInterval across respawns, and is disabled by
// DTRACK_NO_UPDATE_CHECK=1.
func startupUpdateCheck() {
	if os.Getenv("DTRACK_NO_UPDATE_CHECK") == "1" {
		return
	}
	if recentlyChecked() {
		return
	}
	// Touch the stamp up front so concurrent/rapid respawns don't all race to
	// the network before the first check finishes.
	_ = os.WriteFile(updateCheckStamp(), []byte(version), 0600)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		rel, err := checkForUpdate(ctx)
		if err != nil {
			logf("update check skipped: %v", err)
			return
		}
		if rel == nil {
			logf("up to date (version %s)", version)
			return
		}
		logf("UPDATE AVAILABLE: %s → %s. Ask the assistant to run dtrack_check_update to install.", version, rel.TagName)
	}()
}
