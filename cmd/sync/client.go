// 与服务端 /api/v2 的通信层。
//
// 两条上传路径是有意区分的：
//   - 小文件走 /sync/upload（multipart 直传）；
//   - 大文件走 /sync/upload/session 分片会话，因为服务端的直传会把整个文件
//     读进内存、且 ParseMultipartForm 有 100MB 上限。
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// smallUploadLimit 以下走直传；超过就走分片会话。
	smallUploadLimit = 8 << 20
	// fragmentSize 是分片上传的块大小。
	fragmentSize = 8 << 20
	// requestTimeout 单次请求的上限。给得宽是因为大文件的上传/下载是一次请求，
	// 但也不能无限等——否则网络假死会挂住整个同步循环。
	requestTimeout = 30 * time.Minute
)

type Client struct {
	base  string
	token string
	http  *http.Client
}

func NewClient(server, token string) *Client {
	return &Client{
		base:  strings.TrimRight(server, "/"),
		token: token,
		http:  &http.Client{Timeout: requestTimeout},
	}
}

func (c *Client) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.base+"/api/v2"+path, body)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return req, nil
}

// do 发请求并把 JSON 响应解到 out（out 为 nil 表示不关心响应体）。
// 非 2xx 一律转成带上下文与响应片段的错误——同步失败时用户最需要知道的是
// 「哪一步、为什么」，而不是一个孤零零的 status code。
func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%s %s: HTTP %d %s", req.Method, req.URL.Path, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Login 用账号密码换一个 access token。
func (c *Client) Login(ctx context.Context, username, password string) (string, error) {
	body, err := json.Marshal(map[string]string{"username": username, "password": password})
	if err != nil {
		return "", err
	}
	req, err := c.newRequest(ctx, http.MethodPost, "/auth/login", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := c.do(req, &out); err != nil {
		return "", err
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("服务端没有返回 access_token")
	}
	return out.AccessToken, nil
}

// Changes 拉取 since 之后的远端变更，同时返回服务端给的下一轮游标。
func (c *Client) Changes(ctx context.Context, since string) ([]RemoteChange, string, error) {
	q := url.Values{}
	q.Set("limit", "500")
	if since != "" {
		q.Set("since", since)
	}
	req, err := c.newRequest(ctx, http.MethodGet, "/sync/changes?"+q.Encode(), nil)
	if err != nil {
		return nil, since, err
	}

	var out struct {
		Changes []struct {
			ID       int64  `json:"id"`
			Filename string `json:"filename"`
			Path     string `json:"path"`
			Hash     string `json:"hash"`
			Size     int64  `json:"size"`
			Action   string `json:"action"`
		} `json:"changes"`
		Next string `json:"next"`
	}
	if err := c.do(req, &out); err != nil {
		return nil, since, err
	}

	changes := make([]RemoteChange, 0, len(out.Changes))
	for _, ch := range out.Changes {
		rel := ch.Path
		if rel == "" { // 老服务端不返回 path 时退化成文件名，至少不会 panic
			rel = ch.Filename
		}
		changes = append(changes, RemoteChange{
			ID:     ch.ID,
			Rel:    rel,
			Hash:   ch.Hash,
			Size:   ch.Size,
			Action: ch.Action,
		})
	}
	next := out.Next
	if next == "" {
		next = since
	}
	return changes, next, nil
}

// Upload 按大小选择直传或分片，返回服务端的 file_id 与内容 hash。
func (c *Client) Upload(ctx context.Context, rel string, f *os.File, size int64) (int64, string, error) {
	if size <= smallUploadLimit {
		return c.uploadSmall(ctx, rel, f)
	}
	return c.uploadSession(ctx, rel, f, size)
}

func (c *Client) uploadSmall(ctx context.Context, rel string, f *os.File) (int64, string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filepath.Base(rel))
	if err != nil {
		return 0, "", err
	}
	if _, err := io.Copy(fw, f); err != nil {
		return 0, "", err
	}
	if err := mw.WriteField("path", rel); err != nil {
		return 0, "", err
	}
	if err := mw.Close(); err != nil {
		return 0, "", err
	}

	req, err := c.newRequest(ctx, http.MethodPost, "/sync/upload", &buf)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	var out struct {
		FileID int64  `json:"file_id"`
		Hash   string `json:"file_hash"`
	}
	if err := c.do(req, &out); err != nil {
		return 0, "", err
	}
	return out.FileID, out.Hash, nil
}

// uploadSession 逐段上传并在服务端确认完成时拿到基线。
// 每段都是独立的 PUT，中断后重跑会从会话已收字节之后继续（服务端按
// Content-Range 落盘），这是大文件能续传的原因。
func (c *Client) uploadSession(ctx context.Context, rel string, f *os.File, size int64) (int64, string, error) {
	body, err := json.Marshal(map[string]any{
		"filename":  filepath.Base(rel),
		"file_size": size,
		"path":      rel,
	})
	if err != nil {
		return 0, "", err
	}
	req, err := c.newRequest(ctx, http.MethodPost, "/sync/upload/session", bytes.NewReader(body))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", "application/json")

	var created struct {
		SessionID string `json:"session_id"`
	}
	if err := c.do(req, &created); err != nil {
		return 0, "", err
	}
	if created.SessionID == "" {
		return 0, "", fmt.Errorf("服务端没有返回 session_id")
	}

	for offset := int64(0); offset < size; offset += fragmentSize {
		end := offset + fragmentSize
		if end > size {
			end = size
		}
		part := make([]byte, end-offset)
		if _, err := f.ReadAt(part, offset); err != nil && err != io.EOF {
			return 0, "", err
		}

		req, err := c.newRequest(ctx, http.MethodPut, "/sync/upload/session/"+created.SessionID, bytes.NewReader(part))
		if err != nil {
			return 0, "", err
		}
		req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, end-1, size))

		var st struct {
			Status string `json:"status"`
			FileID int64  `json:"file_id"`
			Hash   string `json:"file_hash"`
		}
		if err := c.do(req, &st); err != nil {
			return 0, "", err
		}
		if st.Status == "completed" {
			return st.FileID, st.Hash, nil
		}
	}
	return 0, "", fmt.Errorf("分片已全部发出但服务端未确认完成（会话 %s）", created.SessionID)
}

// Download 下载到 dst。
//
// 先写 .stora-part 再 rename：中断留下的半个文件会在下一轮被当成「本地修改」
// 而推回服务端，等于把损坏内容同步上去。
func (c *Client) Download(ctx context.Context, fileID int64, dst string) error {
	req, err := c.newRequest(ctx, http.MethodGet, "/files/"+strconv.FormatInt(fileID, 10)+"/download", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("下载 %d 失败: HTTP %d %s", fileID, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	tmp := dst + ".stora-part"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

// Delete 删除服务端文件（服务端是软删除）。
func (c *Client) Delete(ctx context.Context, fileID int64) error {
	req, err := c.newRequest(ctx, http.MethodDelete, "/files/"+strconv.FormatInt(fileID, 10), nil)
	if err != nil {
		return err
	}
	return c.do(req, nil)
}

// HashFile 计算文件的 SHA-256（十六进制），与服务端存储的 file_hash 同口径。
func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
