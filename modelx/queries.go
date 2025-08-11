package modelx

import (
	"strings"
	"unicode"

	"github.com/valyala/fasttemplate"
)

// SQLMap is a map of name/query. Each entry has a name and an SQL query used
// in some method.
type SQLMap map[string]any

var (
	/*
		QueryTemplates is an SQLMap (~map[string]any), containing templates
		from which the queries are built. Some of the values may be parts of
		other templates and may be used for replacement in other entries, used
		as templates. We use [fasttemplate.ExecuteStringStd] to construct ready
		for use by [sqlx] queries.
	*/
	QueryTemplates = SQLMap{
		`GetByID`: `SELECT * FROM ${table} WHERE id=:id`,
		`INSERT`:  `INSERT INTO ${table} (${columns}) VALUES ${placeholders}`,
		`SELECT`:  `SELECT ${columns} FROM ${table} ${WHERE} LIMIT ${limit} OFFSET ${offset}`,
		`UPDATE`:  `UPDATE ${table} ${SET} ${WHERE}`,
		`DELETE`:  `DELETE FROM ${table} ${WHERE}`,
	}
	replace = fasttemplate.ExecuteStringStd
)

/*
RenderSQLTemplate gets the template from [QueryTemplates], replaces potential
partial SQL keys from [QueryTemplates] and then the keys from the given stash
with values. Returns the produced SQL. Panics if key not found or not of the expected type (string).
*/
func RenderSQLTemplate(key string, stash map[string]any) string {
	return replace(replace(QueryTemplates[key].(string), "${", "}", QueryTemplates), "${", "}", stash)
}

/*
SQLForSET produces the `SET column = :column,...` for an UPDATE query from
a list of columns. It also makes each column snake_case if it starts with a
capital letter.
*/
func SQLForSET(columns []string) string {
	var set strings.Builder
	set.WriteString(`SET`)
	for _, v := range columns {
		for _, r := range v {
			if unicode.IsUpper(r) {
				v = camelToSnakeCase(v)
				break
			}
			break
		}

		set.WriteString(sprintf(` %s = :%[1]s,`, v))
	}
	setStr := set.String()
	Logger.Debugf(`SQL from SQLForSET:'%s'`, setStr)
	// s[:len(s)-1] == return strings.TrimRight(set.String(), `,`)
	return setStr[:len(setStr)-1]
}
