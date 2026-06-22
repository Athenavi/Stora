package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ── YAML 模型定义结构 ──────────────────────────────

type SchemaDef struct {
	Models map[string]ModelDef `yaml:"models"`
}

type ModelDef struct {
	Table       string               `yaml:"table"`
	Description string               `yaml:"description"`
	Columns     map[string]ColumnDef `yaml:"columns"`
	Indexes     []IndexDef           `yaml:"indexes"`
}

type ColumnDef struct {
	Type          string `yaml:"type"`
	Length        int    `yaml:"length"`
	PrimaryKey    bool   `yaml:"primary_key"`
	AutoIncrement bool   `yaml:"autoincrement"`
	Unique        bool   `yaml:"unique"`
	Nullable      bool   `yaml:"nullable"`
	Default       string `yaml:"default"`
	ForeignKey    string `yaml:"foreign_key"`
	OnDelete      string `yaml:"on_delete"`
}

type IndexDef struct {
	Columns []string `yaml:"columns"`
	Order   string   `yaml:"order"`
	Where   string   `yaml:"where"`
	Unique  bool     `yaml:"unique"`
}

// ── 解析 ──────────────────────────────────────────

func ParseSchema(path string) (*SchemaDef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 %s 失败: %w", path, err)
	}
	var s SchemaDef
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("解析 %s 失败: %w", path, err)
	}
	return &s, nil
}

// ── 完整迁移生成（UPGRADE + DOWNGRADE）─────────────

// GenerateFullMigration 从 models.yaml 生成一整个迁移文件内容。
// 返回 (upgradeSQL, downgradeSQL)。
func (s *SchemaDef) GenerateFullMigration() (upgrade, downgrade string) {
	var upB, downB strings.Builder

	var names []string
	for n := range s.Models {
		names = append(names, n)
	}
	sort.Strings(names)

	// 先建 schema_version 表（如果还不存在）
	upB.WriteString("-- schema_version tracking table\n")
	upB.WriteString(`CREATE TABLE IF NOT EXISTS schema_version (
    revision    VARCHAR(64) PRIMARY KEY,
    description TEXT NOT NULL DEFAULT '',
    applied_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

`)
	downB.WriteString("-- DROP schema_version (moved to end)\n")

	// 表创建顺序：先建被引用的表，再建引用它们的表
	ordered := s.orderModels(names)
	for _, name := range ordered {
		model := s.Models[name]
		upB.WriteString(fmt.Sprintf("-- %s: %s\n", name, model.Description))
		upB.WriteString(model.GenerateCreateSQL())
		upB.WriteString("\n")

		downB.WriteString(fmt.Sprintf("-- %s\n", name))
		downB.WriteString(model.GenerateDropSQL())
	}

	// schema_version dropped last (after all FK-dependent tables)
	downB.WriteString("\n-- schema_version\n")
	downB.WriteString("DROP TABLE IF EXISTS schema_version CASCADE;\n")

	return upB.String(), downB.String()
}

// orderModels 拓扑排序：被引用的表排在前面（FK 目标先建）
func (s *SchemaDef) orderModels(names []string) []string {
	// dependencies[modelName] = list of model names this model depends on (FK targets)
	deps := make(map[string][]string)
	for _, name := range names {
		model := s.Models[name]
		for _, col := range model.Columns {
			if col.ForeignKey != "" {
				fk := col.ForeignKey
				if paren := strings.Index(fk, "("); paren > 0 {
					fk = fk[:paren]
				}
				// Skip self-references (e.g. folders.parent_id → folders)
				if fk == model.Table {
					continue
				}
				// Find the model name that has this table name
				for _, n := range names {
					if s.Models[n].Table == fk {
						deps[name] = append(deps[name], n)
					}
				}
			}
		}
	}

	// Kahn's algorithm: in-degree = number of FK dependencies
	inDegree := make(map[string]int)
	for _, name := range names {
		inDegree[name] = len(deps[name])
	}

	var queue []string
	for _, name := range names {
		if inDegree[name] == 0 {
			queue = append(queue, name)
		}
	}

	// Build reverse map: who depends on X?
	dependents := make(map[string][]string)
	for name, depList := range deps {
		for _, dep := range depList {
			dependents[dep] = append(dependents[dep], name)
		}
	}

	var result []string
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		result = append(result, n)
		for _, dependent := range dependents[n] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}

	// Append any remaining (circular refs — shouldn't happen)
	for _, name := range names {
		found := false
		for _, r := range result {
			if r == name {
				found = true
				break
			}
		}
		if !found {
			result = append(result, name)
		}
	}
	return result
}

// ── SQL 生成 ──────────────────────────────────────

func (m ModelDef) GenerateCreateSQL() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (\n", m.Table))

	var colNames []string
	for n := range m.Columns {
		colNames = append(colNames, n)
	}
	sort.Strings(colNames)

	var parts []string
	for _, name := range colNames {
		col := m.Columns[name]
		parts = append(parts, "    "+m.colToSQL(name, col))
	}
	b.WriteString(strings.Join(parts, ",\n"))
	b.WriteString("\n);\n")

	for _, idx := range m.Indexes {
		b.WriteString(m.idxToSQL(m.Table, idx))
	}

	return b.String()
}

func (m ModelDef) GenerateDropSQL() string {
	return fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE;\n", m.Table)
}

func (m ModelDef) colToSQL(name string, col ColumnDef) string {
	var b strings.Builder
	b.WriteString(name)

	switch col.Type {
	case "bigint":
		b.WriteString(" BIGINT")
	case "integer", "int":
		b.WriteString(" INTEGER")
	case "boolean", "bool":
		b.WriteString(" BOOLEAN")
	case "varchar":
		if col.Length > 0 {
			b.WriteString(fmt.Sprintf(" VARCHAR(%d)", col.Length))
		} else {
			b.WriteString(" VARCHAR(255)")
		}
	case "text":
		b.WriteString(" TEXT")
	case "timestamp":
		b.WriteString(" TIMESTAMP")
	case "float", "double":
		b.WriteString(" DOUBLE PRECISION")
	default:
		b.WriteString(" " + strings.ToUpper(col.Type))
	}

	if col.PrimaryKey {
		b.WriteString(" PRIMARY KEY")
		if col.AutoIncrement {
			b.WriteString(" GENERATED BY DEFAULT AS IDENTITY")
		}
	}
	if col.Unique && !col.PrimaryKey {
		b.WriteString(" UNIQUE")
	}
	if col.PrimaryKey {
		b.WriteString(" NOT NULL")
	} else if col.Nullable {
		b.WriteString(" NULL")
	} else {
		b.WriteString(" NOT NULL")
	}
	if col.Default != "" {
		b.WriteString(fmt.Sprintf(" DEFAULT %s", col.Default))
	}
	if col.ForeignKey != "" {
		ref := col.ForeignKey
		refTable := ref
		refCol := "id"
		if paren := strings.Index(ref, "("); paren > 0 {
			refTable = ref[:paren]
			refCol = ref[paren+1 : len(ref)-1]
		}
		b.WriteString(fmt.Sprintf(" REFERENCES %s(%s)", refTable, refCol))
		if col.OnDelete != "" {
			b.WriteString(fmt.Sprintf(" ON DELETE %s", strings.ToUpper(col.OnDelete)))
		}
	}
	return b.String()
}

func (m ModelDef) idxToSQL(table string, idx IndexDef) string {
	cols := make([]string, len(idx.Columns))
	for i, c := range idx.Columns {
		order := ""
		if idx.Order == "desc" {
			order = " DESC"
		}
		cols[i] = c + order
	}
	colList := strings.Join(cols, ", ")
	idxName := fmt.Sprintf("idx_%s_%s", table, strings.Join(idx.Columns, "_"))

	var b strings.Builder
	if idx.Unique {
		b.WriteString(fmt.Sprintf("CREATE UNIQUE INDEX IF NOT EXISTS %s ON %s (%s)", idxName, table, colList))
	} else {
		b.WriteString(fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON %s (%s)", idxName, table, colList))
	}
	if idx.Where != "" {
		b.WriteString(fmt.Sprintf(" WHERE %s", idx.Where))
	}
	b.WriteString(";\n")
	return b.String()
}

// ── version.ini 管理（revision 版）─────────────────

func readRevisionINI(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "revision") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1]), nil
			}
		}
	}
	return "", nil
}

func writeRevisionINI(path, revision string) error {
	// 读取原有文件，保留 [stora] 段
	var storaBlock strings.Builder
	inStora := false
	if data, err := os.ReadFile(path); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "[stora]" {
				inStora = true
				storaBlock.WriteString(line + "\n")
				continue
			}
			if inStora && len(trimmed) > 0 && trimmed[0] == '[' {
				inStora = false
			}
			if inStora {
				storaBlock.WriteString(line + "\n")
			}
		}
	}

	content := fmt.Sprintf(`[schema]
# 当前数据库 revision（对应 migrations/ 目录下的迁移文件名）
# 由 `+"`stora-cli migrate up/down`"+` 自动维护
revision = %s
`, revision)
	if storaBlock.Len() > 0 {
		content += "\n" + strings.TrimRight(storaBlock.String(), "\n") + "\n"
	}
	return os.WriteFile(path, []byte(content), 0644)
}
