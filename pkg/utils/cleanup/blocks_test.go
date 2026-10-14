package cleanup

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"os"
	"testing"

	"github.com/Athenavi/Stora/pkg/storage"

	_ "github.com/lib/pq"
)

// 这一组测试需要一个真实 PostgreSQL：block_store / file_blocks / file_fingerprints
// 之间有外键，判定孤儿靠的是 NOT EXISTS 子查询，用 sqlmock 测等于自欺。
// 通过 STORA_TEST_DSN 提供连接串；没配就跳过，不会让 CI 变红。
//
//	STORA_TEST_DSN="host=localhost port=5432 user=postgres password=... dbname=stora2606 sslmode=disable" go test ./pkg/utils/cleanup/
//
// 注意：Sweep 是全表扫描，没有"只扫我造的数据"这回事。因此在非空库上跑这组测试，
// 会**顺带清理掉库里本就存在的孤儿块**（那些确实没人引用的块）。这是回收器的职责，
// 但对"跑测试的库"要有心理准备——别拿生产库的 DSN 来跑。
// 测试用 t.TempDir() 当 storage root，所以真实块的内容不会被触碰，只会看到
// "内容删除失败"的日志（路径对不上），属预期。
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("STORA_TEST_DSN")
	if dsn == "" {
		t.Skip("未设置 STORA_TEST_DSN，跳过块池回收的集成测试")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("连接测试库失败: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("测试库不可达: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// 生成 40 位随机 hex，当作假 hash（列宽 varchar(64)，够用）
func fakeHash(prefix string) string {
	b := make([]byte, 19)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

func TestOrphanBlockSweeper_回收孤儿块但保留被引用的块(t *testing.T) {
	db := testDB(t)
	store := storage.NewLocalDriver(t.TempDir(), "/files")
	sweeper := &OrphanBlockSweeper{DB: db, Storage: store}

	orphan := fakeHash("orphan")
	used := fakeHash("used")
	fileHash := fakeHash("file")

	// 造两条件：一条只存在于 block_store（孤儿），一条被 file_blocks 引用。
	if _, err := db.Exec(
		`INSERT INTO block_store (hash, size, storage_path, ref_count) VALUES ($1, 1, $2, 1)`,
		orphan, "/objects/orphan"); err != nil {
		t.Fatalf("插入孤儿块失败: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM block_store WHERE hash = $1`, orphan) })

	if _, err := db.Exec(
		`INSERT INTO block_store (hash, size, storage_path, ref_count) VALUES ($1, 1, $2, 1)`,
		used, "/objects/used"); err != nil {
		t.Fatalf("插入被引用块失败: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO file_fingerprints (hash, file_size, storage_path, reference_count) VALUES ($1, 1, $2, 1)`,
		fileHash, "/objects/file"); err != nil {
		t.Fatalf("插入 fingerprint 失败: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO file_blocks (file_hash, block_index, block_hash, block_offset, block_size) VALUES ($1, 0, $2, 0, 1)`,
		fileHash, used); err != nil {
		t.Fatalf("插入 file_blocks 失败: %v", err)
	}
	t.Cleanup(func() {
		db.Exec(`DELETE FROM file_blocks WHERE file_hash = $1`, fileHash)
		db.Exec(`DELETE FROM file_fingerprints WHERE hash = $1`, fileHash)
		db.Exec(`DELETE FROM block_store WHERE hash = $1`, used)
	})

	n, err := sweeper.Sweep(context.Background())
	if err != nil {
		t.Fatalf("Sweep 报错: %v", err)
	}
	if n < 1 {
		t.Fatalf("期望至少回收 1 个孤儿块，实际回收 %d 个", n)
	}

	if blockExists(t, db, orphan) {
		t.Error("孤儿块应被回收，但它还在 block_store 里")
	}
	if !blockExists(t, db, used) {
		t.Error("仍被 file_blocks 引用的块不该被删——这是这个回收器最危险的失效方式")
	}
}

func TestOrphanBlockSweeper_无可回收时返回零(t *testing.T) {
	db := testDB(t)
	store := storage.NewLocalDriver(t.TempDir(), "/files")
	sweeper := &OrphanBlockSweeper{DB: db, Storage: store}

	// 先清干净历史孤儿，再扫一次应该是 0
	if _, err := sweeper.Sweep(context.Background()); err != nil {
		t.Fatalf("第一次 Sweep 报错: %v", err)
	}
	n, err := sweeper.Sweep(context.Background())
	if err != nil {
		t.Fatalf("第二次 Sweep 报错: %v", err)
	}
	if n != 0 {
		t.Errorf("已无孤儿，第二次应回收 0 个，实际 %d", n)
	}
}

func TestOrphanBlockSweeper_未配置依赖时不炸(t *testing.T) {
	s := &OrphanBlockSweeper{}
	n, err := s.Sweep(context.Background())
	if err != nil {
		t.Fatalf("缺 DB/Storage 时不应报错: %v", err)
	}
	if n != 0 {
		t.Errorf("缺依赖时应返回 0，实际 %d", n)
	}
}

func blockExists(t *testing.T, db *sql.DB, hash string) bool {
	t.Helper()
	var one int
	err := db.QueryRow(`SELECT 1 FROM block_store WHERE hash = $1`, hash).Scan(&one)
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		t.Fatalf("查询 block_store 失败: %v", err)
	}
	return true
}
