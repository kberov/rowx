package modelx

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// TypeToSnakeCase converts struct type name like *model.Users to
// 'users' and returns it. Panics if unsuccessful.
func TypeToSnakeCase[R SqlxRows](rows R) string {
	typestr := sprintf("%T", rows)
	_, table, ok := strings.Cut(typestr, ".")
	if !ok {
		Logger.Panicf("Could not derive table name from type '%s'!", typestr)
	}
	return CamelToSnakeCase(table)
}

/*
CamelToSnakeCase is used to convert table names and structure fields to snake
case table columns. We also pass it to [sqlx.DB.MapperFunc].
UserLastFiveComments becomes user_last_five_comments.
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
