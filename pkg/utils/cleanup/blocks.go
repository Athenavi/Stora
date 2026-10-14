package cleanup

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/Athenavi/Stora/pkg/storage"
)

// OrphanBlockSweeper 回收块池里已经没有任何 file_blocks 引用的块。
//
// 背景：块按内容寻址去重，删文件时只递减 file_fingerprints 的引用计数、不立刻
// 删块内容（同一份内容可能被别的文件共用）。但引用降到零的块如果不回收，
// block_store 与磁盘上的 objects/ 就只增不减——这是块池唯一的下游清理者。
//
// 之所以不走 CleanupObserver：那套是事件驱动的（事件里带 upload_id / temp_dir），
// 而"哪个块成了孤儿"只能靠全表比对得出，没有对应的事件可挂。
type OrphanBlockSweeper struct {
	DB      *sql.DB
	Storage storage.Driver
	// Interval 两次扫描的间隔；<=0 时取 1 小时。
	Interval time.Duration
}

// Sweep 扫一轮，返回回收掉的块数。
func (s *OrphanBlockSweeper) Sweep(ctx context.Context) (int, error) {
	if s.DB == nil || s.Storage == nil {
		return 0, nil
	}

	rows, err := s.DB.QueryContext(ctx, `
		SELECT bs.hash, bs.storage_path
		FROM block_store bs
		WHERE NOT EXISTS (SELECT 1 FROM file_blocks fb WHERE fb.block_hash = bs.hash)`)
	if err != nil {
		return 0, err
	}

	type orphan struct{ hash, path string }
	var orphans []orphan
	for rows.Next() {
		var o orphan
		if err := rows.Scan(&o.hash, &o.path); err != nil {
			continue
		}
		orphans = append(orphans, o)
	}
	rows.Close()

	swept := 0
	for _, o := range orphans {
		// 删除时再确认一次"仍未被引用"：扫描到删除之间可能有文件重新引用了它。
		res, err := s.DB.ExecContext(ctx, `
			DELETE FROM block_store
			WHERE hash = $1
			  AND NOT EXISTS (SELECT 1 FROM file_blocks WHERE block_hash = $1)`, o.hash)
		if err != nil {
			continue
		}
		if n, _ := res.RowsAffected(); n == 0 {
			continue // 期间又被引用了，放过
		}

		// 记录已删，再删内容。反过来的话一旦内容先没了而记录还在，
		// 读清单就会指向一个不存在的块。
		if err := s.Storage.Delete(o.path); err != nil {
			log.Printf("[BlockGC] 块记录已删但内容删除失败 hash=%s path=%s: %v", o.hash, o.path, err)
		}
		swept++
	}

	return swept, nil
}

// Start 起一个后台周期任务；ctx 结束时退出。
func (s *OrphanBlockSweeper) Start(ctx context.Context) {
	interval := s.Interval
	if interval <= 0 {
		interval = time.Hour
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				n, err := s.Sweep(ctx)
				if err != nil {
					log.Printf("[BlockGC] 扫描失败: %v", err)
					continue
				}
				if n > 0 {
					log.Printf("[BlockGC] 回收了 %d 个孤儿块", n)
				}
			}
		}
	}()
}
