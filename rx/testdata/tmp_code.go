package generator

import (
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/valyala/fasttemplate"
)

type Column struct {
	Name         string
	SQLType      string
	GoType       string
	Enum         bool
	EnumVals     []string
	IsStringEnum bool
}

type Table struct {
	Name    string
	Columns []Column
}

func Generate(dbPath, outputPath, pkg string) error {
	db, err := sqlx.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	tables, err := extractTables(db)
	if err != nil {
		return err
	}

	for i := range tables {
		tables[i].Columns, err = extractColumns(db, tables[i].Name, db)
		if err != nil {
			return err
		}
	}

	code := renderAll(tables, pkg)
	return os.WriteFile(outputPath, []byte(code), 0644)
}

func extractTables(db *sqlx.DB) ([]Table, error) {
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []Table
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		res = append(res, Table{Name: name})
	}
	return res, nil
}

func extractColumns(db *sqlx.DB, table string, conn *sqlx.DB) ([]Column, error) {
	rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []Column
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}

		goType, isEnum := mapToGoType(name, ctype)
		col := Column{
			Name:    name,
			SQLType: ctype,
			GoType:  goType,
			Enum:    isEnum,
		}

		if isEnum {
			vals, strEnum := detectEnumValues(conn, table, name)
			if len(vals) > 0 {
				col.EnumVals = vals
				col.IsStringEnum = strEnum
				if strEnum {
					col.GoType = toCamel(name)
				}
			}
		}

		cols = append(cols, col)
	}
	return cols, nil
}

func detectEnumValues(db *sqlx.DB, table, column string) ([]string, bool) {
	// 1) CHECK constraint
	checkVals := parseCheckConstraint(db, table, column)
	if len(checkVals) > 0 {
		return checkVals, !allNumeric(checkVals)
	}

	// 2) DISTINCT values
	query := fmt.Sprintf(`SELECT DISTINCT %s FROM %s ORDER BY %s`, column, table, column)
	rows, err := db.Query(query)
	if err != nil {
		return nil, false
	}
	defer rows.Close()

	var vals []string
	var hasString bool
	for rows.Next() {
		var v interface{}
		if err := rows.Scan(&v); err != nil {
			return nil, false
		}
		switch vv := v.(type) {
		case int64:
			vals = append(vals, fmt.Sprintf("%d", vv))
		case []byte:
			vals = append(vals, string(vv))
			hasString = true
		case string:
			vals = append(vals, vv)
			hasString = true
		}
	}

	if len(vals) > 0 && len(vals) <= 20 {
		return uniqueSorted(vals), hasString || !allNumeric(vals)
	}
	return nil, false
}

func parseCheckConstraint(db *sqlx.DB, table, column string) []string {
	row := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name=?`, table)
	var sqlText string
	if err := row.Scan(&sqlText); err != nil {
		return nil
	}

	// CHECK(col IN (...))
	re := regexp.MustCompile(fmt.Sprintf(`(?i)CHECK\s*\(\s*%s\s+IN\s*\(([^)]+)\)\)`, column))
	m := re.FindStringSubmatch(sqlText)
	if len(m) < 2 {
		return nil
	}

	parts := strings.Split(m[1], ",")
	var vals []string
	for _, p := range parts {
		p = strings.TrimSpace(strings.Trim(p, "'\""))
		if p != "" {
			vals = append(vals, p)
		}
	}
	return uniqueSorted(vals)
}

func mapToGoType(name, sqlType string) (string, bool) {
	t := strings.ToUpper(sqlType)
	if strings.Contains(t, "INT") {
		if strings.HasPrefix(strings.ToLower(name), "is_") || strings.Contains(strings.ToLower(name), "flag") {
			return "bool", false
		}
		return toCamel(name), true // candidate enum
	}
	if strings.Contains(t, "CHAR") || strings.Contains(t, "TEXT") {
		return "string", false
	}
	if strings.Contains(t, "REAL") || strings.Contains(t, "FLOA") || strings.Contains(t, "DOUB") {
		return "float64", false
	}
	if strings.Contains(t, "BLOB") {
		return "[]byte", false
	}
	return "string", false
}

func renderAll(tables []Table, pkg string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("package %s\n\n", pkg))
	b.WriteString("import \"fmt\"\n\n")

	// enums
	for _, t := range tables {
		for _, c := range t.Columns {
			if c.Enum && len(c.EnumVals) > 0 {
				enumName := c.GoType
				if c.IsStringEnum {
					b.WriteString(fmt.Sprintf("type %s string\n\n", enumName))
					b.WriteString("const (\n")
					for _, v := range c.EnumVals {
						b.WriteString(fmt.Sprintf("\t%s%s %s = \"%s\"\n", enumName, toCamel(v), enumName, v))
					}
					b.WriteString(")\n\n")
					b.WriteString(fmt.Sprintf("func (v %s) String() string { return string(v) }\n\n", enumName))
				} else {
					b.WriteString(fmt.Sprintf("type %s int\n\n", enumName))
					b.WriteString("const (\n")
					for _, v := range c.EnumVals {
						b.WriteString(fmt.Sprintf("\t%s%s %s = %s\n", enumName, v, enumName, v))
					}
					b.WriteString(")\n\n")
					b.WriteString(fmt.Sprintf("func (v %s) String() string {\n", enumName))
					b.WriteString("\tswitch v {\n")
					for _, v := range c.EnumVals {
						b.WriteString(fmt.Sprintf("\tcase %s%s:\n\t\treturn \"%s\"\n", enumName, v, v))
					}
					b.WriteString("\tdefault:\n\t\treturn fmt.Sprintf(\"%d\", int(v))\n\t}\n}\n\n")
				}
			}
		}
	}

	// structs
	for _, t := range tables {
		tpl := fasttemplate.New(`
type {{StructName}} struct {
{{Fields}}
}
`, "{{", "}}")

		fields := strings.Builder{}
		for _, c := range t.Columns {
			fields.WriteString(fmt.Sprintf("\t%s %s `rx:\"%s\"`\n", toCamel(c.Name), c.GoType, c.Name))
		}

		data := map[string]interface{}{
			"StructName": toCamel(t.Name),
			"Fields":     fields.String(),
		}
		b.WriteString(tpl.ExecuteString(data))
		b.WriteString("\n")
	}

	return b.String()
}

func toCamel(s string) string {
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + strings.ToLower(p[1:])
		}
	}
	return strings.Join(parts, "")
}

func uniqueSorted(vals []string) []string {
	m := map[string]struct{}{}
	for _, v := range vals {
		m[v] = struct{}{}
	}
	uniq := make([]string, 0, len(m))
	for v := range m {
		uniq = append(uniq, v)
	}
	sort.Strings(uniq)
	return uniq
}

func allNumeric(vals []string) bool {
	for _, v := range vals {
		if _, err := strconv.Atoi(v); err != nil {
			return false
		}
	}
	return true
}
