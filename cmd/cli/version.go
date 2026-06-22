package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// ── version.ini [stora] 结构 ──────────────────────

type VersionInfo struct {
	Version     string
	ReleaseDate string
	Channel     string
	GoVersion   string
}

// CurrentVersion 从 version.ini 的 [stora] 段读取版本信息
func CurrentVersion() (*VersionInfo, error) {
	data, err := os.ReadFile("version.ini")
	if err != nil {
		return nil, fmt.Errorf("读取 version.ini 失败: %w", err)
	}

	v := &VersionInfo{}
	inStora := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "[stora]" {
			inStora = true
			continue
		}
		if inStora && len(line) > 0 && line[0] == '[' {
			break
		}
		if !inStora {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "version":
			v.Version = val
		case "release_date":
			v.ReleaseDate = val
		case "channel":
			v.Channel = val
		case "go_version":
			v.GoVersion = val
		}
	}
	if v.Version == "" {
		return nil, fmt.Errorf("version.ini 中未找到 [stora] 版本信息")
	}
	return v, nil
}

// ── 版本比较 ──────────────────────────────────────

func parseVersion(ver string) (int, int, int, int) {
	v := strings.TrimPrefix(ver, "V")
	v = strings.TrimPrefix(v, "v")
	var major, minor, date, build int
	fmt.Sscanf(v, "%d.%d.%d.%d", &major, &minor, &date, &build)
	return major, minor, date, build
}

// IsNewer 判断 remoteVer 是否比 localVer 新
func IsNewer(localVer, remoteVer string) bool {
	lmaj, lmin, ldate, lbuild := parseVersion(localVer)
	rmaj, rmin, rdate, rbuild := parseVersion(remoteVer)
	if rmaj != lmaj {
		return rmaj > lmaj
	}
	if rmin != lmin {
		return rmin > lmin
	}
	if rdate != ldate {
		return rdate > ldate
	}
	return rbuild > lbuild
}

// ── GitHub Release API ───────────────────────────

type GitHubRelease struct {
	TagName     string        `json:"tag_name"`
	Name        string        `json:"name"`
	Body        string        `json:"body"`
	Prerelease  bool          `json:"prerelease"`
	PublishedAt string        `json:"published_at"`
	HTMLURL     string        `json:"html_url"`
	Assets      []GitHubAsset `json:"assets"`
}

type GitHubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int    `json:"size"`
}

// CheckLatestRelease 查询 GitHub 最新 Release
func CheckLatestRelease(owner, repo string) (*GitHubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "Stora-CLI/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GitHub API 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 403 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API 限流: %s", string(body))
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API 返回 %d", resp.StatusCode)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("解析 GitHub Release 失败: %w", err)
	}
	return &release, nil
}

// ── 格式化 ────────────────────────────────────────

func (v *VersionInfo) FormatVersion() string {
	return v.Version
}

func FormatReleaseNotes(r *GitHubRelease) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("\n  📦 %s\n", r.TagName))
	b.WriteString(fmt.Sprintf("  📅 %s\n", r.PublishedAt[:10]))
	if len(r.Body) > 500 {
		b.WriteString(fmt.Sprintf("  📝 %s...\n", r.Body[:500]))
	} else {
		b.WriteString(fmt.Sprintf("  📝 %s\n", r.Body))
	}
	if len(r.Assets) > 0 {
		b.WriteString(fmt.Sprintf("  📎 %d 个附件\n", len(r.Assets)))
		for _, a := range r.Assets {
			b.WriteString(fmt.Sprintf("     • %s (%d MB)\n", a.Name, a.Size/(1024*1024)))
		}
	}
	b.WriteString(fmt.Sprintf("  🔗 %s\n", r.HTMLURL))
	return b.String()
}
