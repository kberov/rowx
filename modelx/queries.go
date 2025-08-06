package modelx

import (
	"strings"

	"github.com/valyala/fasttemplate"
)

// SQLMap is a map of name/query. Each entry has a name and an SQL query used
// in some method.
type SQLMap map[string]any

// QueryTemplates is an SQLMap (~map[string]any), containing templates from which the
// queries are built. Some of the values are parts of other queries and may be
// used for replacement in other entries, used as templates. We use
// [fasttemplate.ExecuteStringStd] to construct ready for use by [sqlx]
// queries.
var QueryTemplates = SQLMap{
	"INSERT":  `INSERT INTO ${table} (${columns}) VALUES ${placeholders}`,
	"GetByID": `SELECT * FROM ${table} WHERE id=?`,
	"SELECT":  `SELECT ${columns} FROM ${table} ${WHERE} LIMIT ${limit} OFFSET ${offset}`,
}

/*
SQLFor composes an SQL query for the given key. Returns the composed query.
*/
func SQLFor(query, table string) string {
	q := QueryTemplates[query].(string)
	QueryTemplates["table"] = table
	for strings.Contains(q, "${") {
		q = fasttemplate.ExecuteStringStd(q, "${", "}", QueryTemplates)
	}
	delete(QueryTemplates, "table")
	return q
}
