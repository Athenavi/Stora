// 同步客户端的配置：服务端地址、凭据、本地根目录。
//
// 配置与状态分开存放（config.json / state.json）：配置含 access token 且很少变，
// 状态每次同步都重写。混在一起会让「每次同步都重写 token 文件」变成常态。
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Config 描述一次同步的双方：本地的某个目录 ↔ 服务端的某个账号。
type Config struct {
	Server     string `json:"server"`                // 形如 http://127.0.0.1:9421
	Username   string `json:"username"`              //
	Token      string `json:"token"`                 // 登录后拿到的 access token
	Root       string `json:"root"`                  // 本地同步根目录（建议写绝对路径）
	RemoteRoot string `json:"remote_root,omitempty"` // 远端对应目录，留空 = 账号根目录
	Interval   string `json:"interval,omitempty"`    // watch 模式的轮询间隔，默认 3s
}

const defaultInterval = "3s"

// DefaultDir 返回配置与状态文件的存放目录。
func DefaultDir() string {
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "stora-sync")
	}
	return ".stora-sync" // 取不到用户配置目录时的兜底：当前目录
}

func DefaultConfigPath() string { return filepath.Join(DefaultDir(), "config.json") }

func DefaultStatePath() string { return filepath.Join(DefaultDir(), "state.json") }

func LoadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	if c.Interval == "" {
		c.Interval = defaultInterval
	}
	return &c, nil
}

// Save 写入配置。文件含 access token，权限收紧到 0600。
func (c *Config) Save(path string) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	return os.WriteFile(path, b, 0o600)
}

// remoteRel 把本地绝对路径换算成服务端相对路径。
// 服务端所有 /sync/* 端点都以这个相对路径为键，并自行创建中间目录。
func (c *Config) remoteRel(abs string) (string, error) {
	rel, err := filepath.Rel(c.Root, abs)
	if err != nil {
		return "", err
	}
	rel = filepath.ToSlash(rel)
	if prefix := strings.Trim(c.RemoteRoot, "/"); prefix != "" {
		rel = prefix + "/" + rel
	}
	return relKey(rel), nil
}

// localAbs 是 remoteRel 的逆运算：把服务端的相对路径映射回本地绝对路径。
func (c *Config) localAbs(rel string) string {
	rel = relKey(rel)
	if prefix := strings.Trim(c.RemoteRoot, "/"); prefix != "" && strings.HasPrefix(rel, prefix+"/") {
		rel = strings.TrimPrefix(rel, prefix+"/")
	}
	return filepath.Join(c.Root, filepath.FromSlash(rel))
}
