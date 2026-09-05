package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
)

type Lang string

const (
	EN Lang = "en"
	FR Lang = "fr"
)

var Langs = []Lang{EN, FR}

func ParseLang(v string) Lang {
	if Lang(strings.ToLower(strings.TrimSpace(v))) == FR {
		return FR
	}
	return EN
}

func Name(l Lang) string { return T(l, "language.name") }

func T(l Lang, key string, args ...any) string {
	text, ok := catalog[l][key]
	if !ok {
		if text, ok = catalog[EN][key]; !ok {
			return key
		}
	}
	if len(args) == 0 {
		return text
	}
	return fmt.Sprintf(text, args...)
}

func Errorf(l Lang, key string, args ...any) error {
	return fmt.Errorf("%s", T(l, key, args...))
}

//go:embed locales/*.json
var files embed.FS

var catalog = load()

func load() map[Lang]map[string]string {
	out := map[Lang]map[string]string{}
	for _, l := range Langs {
		data, err := files.ReadFile("locales/" + string(l) + ".json")
		if err != nil {
			panic("catalog " + string(l) + " missing: " + err.Error())
		}
		m := map[string]string{}
		if err := json.Unmarshal(data, &m); err != nil {
			panic("catalog " + string(l) + " unreadable: " + err.Error())
		}
		out[l] = m
	}
	return out
}
