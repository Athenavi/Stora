// Stora 同步客户端的本地状态。
//
// 同步的正确性取决于能否区分「谁改的」。只记录当前状态是不够的，必须保留上一次
// 同步完成时的基线（base），才能做三方比较（base / local / remote）——缺了 base，
// 双方都改就只能退化成「后写覆盖先写」，而那正是设计文档明确要避免的。
//
// 存储用 JSON 而不是 SQLite：避免引入 cgo 依赖，且状态规模（万级文件约 1–2 MB）
// 全量重写完全够用。
//
// ponytail: 状态涨到约 10 万文件后全量重写的开销才会显现；届时换纯 Go 的
// modernc.org/sqlite 或按目录分片即可，本文件的对外方法不用变。
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Entry 是「上次同步完成时」该路径在双方共同认可的基线状态。
type Entry struct {
	ID    int64  `json:"id,omitempty"`    // 服务端 file_id；0 表示尚未上传过
	Hash  string `json:"hash,omitempty"`  // 基线内容的 SHA-256（十六进制）
	Mtime int64  `json:"mtime,omitempty"` // 基线本地修改时间（Unix 纳秒）
	Size  int64  `json:"size,omitempty"`
}

// State 是同步客户端的全部持久状态。
type State struct {
	// Since 是 /sync/changes 的时间游标：下次只需拉取它之后的变更。
	Since string `json:"since,omitempty"`
	// Files 以规范化后的相对路径为键。
	Files map[string]*Entry `json:"files"`

	path string // 磁盘位置，不参与序列化
}

// LoadState 读取状态文件；文件不存在时返回空状态（首次运行属正常路径）。
func LoadState(path string) (*State, error) {
	s := &State{Files: map[string]*Entry{}, path: path}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, s); err != nil {
		return nil, err
	}
	if s.Files == nil {
		s.Files = map[string]*Entry{}
	}
	s.path = path
	return s, nil
}

// Save 原子落盘：先写同目录临时文件再 rename。
//
// 这样做是有必要的——状态文件损坏就等于丢掉基线，客户端只能退化成整库重新比对，
// 而「重新比对」在没有基线时无法判定冲突，可能把用户的本地修改当成新文件重传。
func (s *State) Save() error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(s.path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *State) Get(rel string) *Entry { return s.Files[relKey(rel)] }

func (s *State) Set(rel string, e *Entry) { s.Files[relKey(rel)] = e }

func (s *State) Delete(rel string) { delete(s.Files, relKey(rel)) }

// relKey 把路径规范化成状态键：统一为 / 分隔、去掉 "./" 前缀与首尾分隔符。
// 服务端也做同样的规范化（filepath.Clean），两边对齐才能稳定配对。
func relKey(rel string) string {
	rel = filepath.ToSlash(rel)
	rel = strings.TrimPrefix(rel, "./")
	rel = strings.Trim(rel, "/")
	return rel
}
