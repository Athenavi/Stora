// 同步编排：把一次「扫描 → 拉取 → 决策 → 执行」串起来。
package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
)

// RunOnce 执行一轮双向同步。
//
// 有一条不显眼但关键的规则：只要存在未处理的冲突，就不推进时间游标。
// 否则那些变更会被当作「已处理」，下一轮再也拉不到，冲突就永久静默了——
// 用户既没看到提示，双方也没有同步。
func RunOnce(ctx context.Context, cfg *Config, st *State, c *Client, out io.Writer) error {
	local, err := ScanLocal(cfg.Root, st)
	if err != nil {
		return fmt.Errorf("扫描本地目录: %w", err)
	}
	remote, next, err := c.Changes(ctx, st.Since)
	if err != nil {
		return fmt.Errorf("拉取远端变更: %w", err)
	}

	decisions := Plan(local, remote, st)

	var conflicts []Decision
	acted := 0
	for _, d := range decisions {
		did, err := apply(ctx, cfg, st, c, d, out)
		if err != nil {
			return fmt.Errorf("%s（%s）: %w", d.Rel, d.Reason, err)
		}
		if did {
			acted++
		}
		if d.Action == Conflict {
			conflicts = append(conflicts, d)
		}
	}

	if len(conflicts) > 0 {
		if err := ResolveConflicts(ctx, cfg, st, c, conflicts, os.Stdin, out); err != nil {
			return err
		}
	} else {
		st.Since = next
	}

	if err := st.Save(); err != nil {
		return fmt.Errorf("保存状态: %w", err)
	}
	if acted == 0 && len(conflicts) == 0 {
		fmt.Fprintln(out, "已是最新。")
	}
	return nil
}

// apply 执行单个决策，返回是否真的动了数据。
func apply(ctx context.Context, cfg *Config, st *State, c *Client, d Decision, out io.Writer) (bool, error) {
	switch d.Action {
	case Skip:
		// 即使跳过也要把基线补齐：首次同步时远端已有、本地也存在且内容一致的
		// 文件，如果不记录 base，下一轮又会被判成「本地新增」而重复上传。
		if d.LocalHash != "" && d.LocalHash == d.RemoteHash {
			e := st.Get(d.Rel)
			if e == nil {
				e = &Entry{}
			}
			e.ID = d.RemoteID
			e.Hash = d.LocalHash
			st.Set(d.Rel, e)
		}
		return false, nil

	case Conflict:
		return false, nil // 交给 ResolveConflicts

	case Upload:
		return true, doUpload(ctx, cfg, st, c, d.Rel, out)

	case Download:
		return true, doDownload(ctx, cfg, st, c, d, out)

	case DeleteLocal:
		abs := cfg.localAbs(d.Rel)
		if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
			return false, err
		}
		st.Delete(d.Rel)
		fmt.Fprintf(out, "  × 本地删除 %s（远端已删）\n", d.Rel)
		return true, nil

	case DeleteRemote:
		if d.RemoteID == 0 { // 还没上传过就删了，本地抹掉基线即可
			st.Delete(d.Rel)
			return false, nil
		}
		if err := c.Delete(ctx, d.RemoteID); err != nil {
			return false, err
		}
		st.Delete(d.Rel)
		fmt.Fprintf(out, "  × 远端删除 %s（本地已删）\n", d.Rel)
		return true, nil
	}
	return false, nil
}

func doUpload(ctx context.Context, cfg *Config, st *State, c *Client, rel string, out io.Writer) error {
	abs := cfg.localAbs(rel)
	f, err := os.Open(abs)
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}
	id, hash, err := c.Upload(ctx, rel, f, info.Size())
	if err != nil {
		return err
	}
	// 以服务端算出的 hash 作为新基线：它才是双方公认的那个值。
	st.Set(rel, &Entry{ID: id, Hash: hash, Size: info.Size(), Mtime: info.ModTime().UnixNano()})
	fmt.Fprintf(out, "  ↑ %s（%d 字节）\n", rel, info.Size())
	return nil
}

func doDownload(ctx context.Context, cfg *Config, st *State, c *Client, d Decision, out io.Writer) error {
	abs := cfg.localAbs(d.Rel)
	if err := c.Download(ctx, d.RemoteID, abs); err != nil {
		return err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	// 刚落地的内容必然等于远端 hash，直接把它记为基线。
	st.Set(d.Rel, &Entry{ID: d.RemoteID, Hash: d.RemoteHash, Size: info.Size(), Mtime: info.ModTime().UnixNano()})
	fmt.Fprintf(out, "  ↓ %s\n", d.Rel)
	return nil
}

// ResolveConflicts 逐条询问用户保留哪一边。
//
// 「跳过」是默认项且永远安全：它不推进游标，所以这条冲突下一轮还会再问，
// 不会因为一次误按键就丢掉某一侧的修改。
func ResolveConflicts(ctx context.Context, cfg *Config, st *State, c *Client, conflicts []Decision, in io.Reader, out io.Writer) error {
	r := bufio.NewReader(in)
	unresolved := false

	for _, d := range conflicts {
		fmt.Fprintf(out, "\n冲突：%s\n  原因：%s\n  本地：%s\n  远端：%s\n", d.Rel, d.Reason, shortHash(d.LocalHash), shortHash(d.RemoteHash))
		fmt.Fprint(out, "  [l] 保留本地（覆盖远端）  [r] 保留远端（覆盖本地）  [s] 稍后再说： ")

		line, err := r.ReadString('\n')
		if err != nil && line == "" {
			// 非交互场景（管道、后台）读不到输入：按「稍后再说」处理，绝不自作主张。
			fmt.Fprintln(out, "\n（读不到输入，保持两边不动）")
			unresolved = true
			continue
		}

		switch strings.ToLower(strings.TrimSpace(line)) {
		case "l", "local":
			if err := doUpload(ctx, cfg, st, c, d.Rel, out); err != nil {
				return err
			}
		case "r", "remote":
			if d.RemoteHash == "" { // 远端已删除：保留远端 = 删掉本地
				if err := os.Remove(cfg.localAbs(d.Rel)); err != nil && !os.IsNotExist(err) {
					return err
				}
				st.Delete(d.Rel)
				fmt.Fprintf(out, "  × 本地删除 %s（按你的选择保留远端）\n", d.Rel)
				break
			}
			if err := doDownload(ctx, cfg, st, c, d, out); err != nil {
				return err
			}
		default:
			fmt.Fprintf(out, "  已跳过 %s，下次同步会再问。\n", d.Rel)
			unresolved = true
		}
	}

	if unresolved {
		return fmt.Errorf("还有未解决的冲突，时间游标未推进")
	}
	return nil
}

func shortHash(h string) string {
	if h == "" {
		return "(无)"
	}
	if len(h) > 12 {
		return h[:12]
	}
	return h
}
