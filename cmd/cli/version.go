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

// ── version.txt 结构 ─────────────────────────────

type VersionInfo struct {
	Version     string `ini:"version"`
	ReleaseDate string `ini:"release_date"`
	Channel     string `ini:"channel"`
	GoVersion   string `ini:"go_version"`
}

// CurrentVersion 读取 version.txt 返回当前版本信息
func CurrentVersion() (*VersionInfo, error) {
	data, err := os.ReadFile("version.txt")
	if err != nil {
		return nil, fmt.Errorf("读取 version.txt 失败: %w", err)
	}

	v := &VersionInfo{}
	section := ""
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = line[1 : len(line)-1]
			continue
		}
		if section != "stora" {
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
		return nil, fmt.Errorf("version.txt 中未找到 [stora] 版本信息")
	}
	return v, nil
}

// ── 版本比较 ──────────────────────────────────────

// Semver 将 "V0.1.260622.01" 解析为可比较的整数
// 返回 (major, minor, date, build)
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

// GitHubRelease 对应 GitHub API /releases/latest 的响应
type GitHubRelease struct {
	TagName    string         `json:"tag_name"`
	Name       string         `json:"name"`
	Body       string         `json:"body"`
	Prerelease bool           `json:"prerelease"`
	PublishedAt string        `json:"published_at"`
	HTMLURL    string         `json:"html_url"`
	Assets     []GitHubAsset  `json:"assets"`
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

// FormatVersion 美化版本信息显示
func (v *VersionInfo) FormatVersion() string {
	return v.Version
}

// FormatReleaseNotes 格式化 Release 信息
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
