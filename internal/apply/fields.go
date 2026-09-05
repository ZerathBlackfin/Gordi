package apply

import (
	"regexp"
	"strings"

	"gordi/internal/i18n"
)

type Field struct {
	Name    string
	Numeric bool
}

var Fields = []Field{
	{Name: "artist"},
	{Name: "album"},
	{Name: "year"},
	{Name: "track", Numeric: true},
	{Name: "title"},
	{Name: "disc", Numeric: true},
	{Name: "format"},
}

var byName = func() map[string]Field {
	m := map[string]Field{}
	for _, f := range Fields {
		m[f.Name] = f
		for _, l := range i18n.Langs {
			m[strings.ToLower(DisplayName(f, l))] = f
		}
	}
	return m
}()

func Resolve(name string) (Field, bool) {
	f, ok := byName[strings.ToLower(name)]
	return f, ok
}

func DisplayName(f Field, l i18n.Lang) string {
	return i18n.T(l, "field."+f.Name)
}

func DisplayFields(l i18n.Lang) []string {
	out := make([]string, 0, len(Fields))
	for _, f := range Fields {
		out = append(out, "{"+DisplayName(f, l)+"}")
	}
	return out
}

var FieldRE = regexp.MustCompile(`\{([a-zA-Z]+)(?::(0+))?\}`)

var BracesRE = regexp.MustCompile(`\{[^{}]*\}`)

const DefaultWidth = 2

func HasField(pattern, name string) bool {
	for _, m := range FieldRE.FindAllStringSubmatch(pattern, -1) {
		if f, ok := Resolve(m[1]); ok && f.Name == name {
			return true
		}
	}
	return false
}

func ValidateFields(pattern string, l i18n.Lang) error {
	for _, m := range FieldRE.FindAllStringSubmatch(pattern, -1) {
		token, name, padding := m[0], m[1], m[2]
		f, ok := Resolve(name)
		if !ok {
			return i18n.Errorf(l, "pattern.unknownField", token)
		}
		if padding != "" && !f.Numeric {
			return i18n.Errorf(l, "pattern.paddingOnText", token)
		}
	}
	for _, token := range BracesRE.FindAllString(pattern, -1) {
		if !FieldRE.MatchString(token) {
			return i18n.Errorf(l, "pattern.malformedField", token)
		}
	}
	return nil
}
