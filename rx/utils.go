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
TypeToSnakeCase converts struct type name like
AVeryLongAndComplexTableName to 'a_very_long_and_complex_table_name' and
returns it. Panics if the structure is annonimous or there are nonalphanumeric
characters.
*/
func TypeToSnakeCase[R Rowx](row R) string {
	typestr := type2str(row)
	Logger.Debugf("TypeToSnakeCase typestr: %s", typestr)
	// Anonimous struct
	if typestr == `` {
		Logger.Panicf(type2StrPanicFmtStr, typestr)
	}
	return CamelToSnakeCase(typestr)
}

/*
CamelToSnakeCase is used to convert type names and structure fields to snake
case table columns. We pass it to [reflectx.NewMapperFunc] together with
[ReflectXTag]. The string `UserLastFiveComments` is transformed to
`user_last_five_comments`.
*/
func CamelToSnakeCase(text string) string {
	if utf8.RuneCountInString(text) == 2 {
		return strings.ToLower(text)
	}
	var snakeCase strings.Builder
	var wordBegins = true
	var prevWasUpper = true
	for _, r := range text {
		wordBegins, prevWasUpper = lowerLetter(&snakeCase, r, wordBegins, prevWasUpper, "_")
	}
	return snakeCase.String()
}

func lowerLetter(snakeCase *strings.Builder, r rune, wordBegins, prevWasUpper bool, connector string) (bool, bool) {
	if unicode.IsUpper(r) && !wordBegins {
		snakeCase.WriteString(connector)
		snakeCase.WriteRune(unicode.ToLower(r))
		wordBegins, prevWasUpper = true, true
		return wordBegins, prevWasUpper
	}
	// handle case `ID` and beginning of word
	if wordBegins && prevWasUpper {
		snakeCase.WriteRune(unicode.ToLower(r))
		wordBegins, prevWasUpper = false, false
		return wordBegins, prevWasUpper
	}
	snakeCase.WriteRune(r)
	return wordBegins, prevWasUpper
}

/*
SnakeToCamel converts words from snake_case to CamelCase. It will be used to
convert table_name to StructName and column_names to ColumnNames. This will be
done during generation of structures out from tables.
*/
func SnakeToCamel(snake_case_word string) string { //nolint:all //  should be snakeCaseWord
	if utf8.RuneCountInString(snake_case_word) == 2 {
		return strings.ToUpper(snake_case_word)
	}
	var words strings.Builder
	nextToBeUpper := false
	for i, c := range snake_case_word {
		if i == 0 {
			words.WriteRune(unicode.ToTitle(c))
			continue
		}
		if c == '_' {
			nextToBeUpper = true
			continue
		}
		if nextToBeUpper {
			words.WriteRune(unicode.ToTitle(c))
			nextToBeUpper = false
			continue
		}
		words.WriteRune(c)
	}
	return words.String()
}
