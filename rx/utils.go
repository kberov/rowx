package rx

import (
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"
)

func type2str[R Rowx](row R) string {
	return reflect.TypeOf(row).Elem().Name()
}

const type2StrPanicFmtStr = "Could not derive table name from type '%s'!"

/*
TypeToSnake converts struct type name like
AVeryLongAndComplexTableName to 'a_very_long_and_complex_table_name' and
returns it. Panics if the structure is annonimous or there are nonalphanumeric
characters.
*/
func TypeToSnake[R Rowx](row R) string {
	typestr := type2str(row)
	Logger.Debugf("TypeToSnakeCase typestr: %s", typestr)
	// Anonimous struct
	if typestr == `` {
		Logger.Panicf(type2StrPanicFmtStr, typestr)
	}
	return CamelToSnake(typestr)
}

/*
CamelToSnake is used to convert type names and structure fields to snake
case table columns. We pass it to [reflectx.NewMapperFunc] together with
[ReflectXTag]. The string `UserLastFiveComments` is transformed to
`user_last_five_comments`.
*/
func CamelToSnake(text string) string {
	if utf8.RuneCountInString(text) == 2 {
		return strings.ToLower(text)
	}
	var snake strings.Builder
	var begins = true
	var wasUpper = true
	for _, r := range text {
		begins, wasUpper = lowerLetter(&snake, r, begins, wasUpper, "_")
	}
	return snake.String()
}

func lowerLetter(snake *strings.Builder, r rune, begins, wasUpper bool, connector string) (bool, bool) {
	if unicode.IsUpper(r) && !begins {
		snake.WriteString(connector)
		snake.WriteRune(unicode.ToLower(r))
		begins, wasUpper = true, true
		return begins, wasUpper
	}
	if begins && wasUpper {
		snake.WriteRune(unicode.ToLower(r))
		begins, wasUpper = false, false
		return begins, wasUpper
	}
	snake.WriteRune(r)
	return begins, wasUpper
}

/*
SnakeToCamel converts words from snake_case to CamelCase. It will be used to
convert table_name to TableName and column_names to ColumnNames. This will be
done during generation of structures out from tables.
*/
func SnakeToCamel(snake_case_word string) string { //nolint:all //  should be snakeCaseWord
	runes := []rune(snake_case_word)
	if len(runes) == 2 {
		return strings.ToUpper(snake_case_word)
	}
	var words strings.Builder
	nextUp := false

	words.WriteRune(unicode.ToUpper(runes[0]))
	for i := 1; i < len(runes); i++ {
		if runes[i] == '_' {
			nextUp = true
			continue
		}
		if nextUp {
			words.WriteRune(unicode.ToUpper(runes[i]))
			nextUp = false
			continue
		}
		words.WriteRune(runes[i])
	}
	return words.String()
}
