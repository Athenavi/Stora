// 本地扫描：把磁盘上的文件树变成可比较的清单。
package main

import (
	"io/fs"
	"path/filepath"
	"strings"
)

// ScanLocal 遍历 root，产出本地文件清单。
//
// 每个文件都算一次 SHA-256 代价很高（万级文件要读完整数据），所以先用
// size + mtime 与基线比对：没变过的文件直接沿用基线里的 hash，一个字节都不读。
// 只有「可能变了」的文件才真正哈希。
//
// ponytail: size+mtime 是启发式，理论上存在「改了内容但大小与 mtime 都没变」的
// 场景（例如程序回写原时间戳）。要绝对可靠就得每次全量哈希，代价是首次同步
// 之后的每一轮都要重读整个目录树——对个人网盘不划算。
func ScanLocal(root string, st *State) ([]LocalFile, error) {
	var out []LocalFile

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err // 权限不足等问题如实上报：静默跳过等于悄悄漏同步
		}
		name := d.Name()

		if d.IsDir() {
			if name == ".stora-sync" { // 客户端自己的状态目录
				return fs.SkipDir
			}
			return nil
		}
		// 下载中的半成品不是用户数据，别让它们进清单。
		if strings.HasSuffix(name, ".stora-part") {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relSlash := filepath.ToSlash(rel)

		base := st.Get(relSlash)
		if base != nil && base.Size == info.Size() && base.Mtime == info.ModTime().UnixNano() {
			out = append(out, LocalFile{
				Rel:   relSlash,
				Hash:  base.Hash,
				Size:  info.Size(),
				Mtime: base.Mtime,
			})
			return nil
		}

		hash, err := HashFile(path)
		if err != nil {
			return err
		}
		out = append(out, LocalFile{
			Rel:   relSlash,
			Hash:  hash,
			Size:  info.Size(),
			Mtime: info.ModTime().UnixNano(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
