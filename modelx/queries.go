package modelx

import (
	"strings"

	"github.com/valyala/fasttemplate"
)

// SQLMap is a map of name/query. Each entry has a name and an SQL query used
// in some method.
type SQLMap map[string]any

var (
	/*
		QueryTemplates is an SQLMap (~map[string]any), containing templates from which
		the queries are built. Some of the values are parts of other queries and may be
		used for replacement in other entries, used as templates. We use
		[fasttemplate.ExecuteStringStd] to construct ready for use by [sqlx] queries.
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
SQLFor composes an SQL query for the given key. Returns the composed query.
Deprecated: Use [RenderSQLFor] instead.
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

/*
RenderSQLFor gets the template from [QueryTemplates], replaces potential
partial SQL keys from [QueryTemplates] and then the keys from the given stash
with values. Returns the produced SQL.
*/
func RenderSQLFor(key string, stash map[string]any) string {
	// Replace also potential values from QueryTemplates.
	// TODO: Can we minimize memory realocation for strings here?
	return replace(replace(QueryTemplates[key].(string), "${", "}", QueryTemplates), "${", "}", stash)
}
