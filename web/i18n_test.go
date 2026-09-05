package web

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func locales(t *testing.T) map[string]map[string]string {
	t.Helper()
	out := map[string]map[string]string{}
	for _, l := range []string{"en", "fr"} {
		data, err := os.ReadFile(filepath.Join("src", "locales", l+".json"))
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]string{}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("%s.json unreadable: %v", l, err)
		}
		out[l] = m
	}
	return out
}

func sources(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	entries, err := os.ReadDir("src")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || (!strings.HasSuffix(e.Name(), ".svelte") && !strings.HasSuffix(e.Name(), ".js")) {
			continue
		}
		data, err := os.ReadFile(filepath.Join("src", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out[e.Name()] = string(data)
	}
	return out
}

var (
	callRE        = regexp.MustCompile(`\bt\(\s*'([a-z][a-zA-Z]*\.[a-zA-Z]+)'`)
	dynamicRE     = regexp.MustCompile(`\bt\(` + "`" + `([a-z][a-zA-Z]*)\.\$\{`)
	placeholderRE = regexp.MustCompile(`\{(\w+)\}`)
)

func TestFrontEveryCalledKeyExists(t *testing.T) {
	cat := locales(t)
	for name, src := range sources(t) {
		for _, m := range callRE.FindAllStringSubmatch(src, -1) {
			for _, l := range []string{"en", "fr"} {
				if _, ok := cat[l][m[1]]; !ok {
					t.Errorf("%s calls %q, missing from %s.json", name, m[1], l)
				}
			}
		}
	}
}

func TestFrontNoOrphanKey(t *testing.T) {
	cat := locales(t)
	src := sources(t)

	seen := map[string]bool{}
	families := map[string]bool{}
	for name, s := range src {
		_ = name
		for _, m := range callRE.FindAllStringSubmatch(s, -1) {
			seen[m[1]] = true
		}
		for _, m := range dynamicRE.FindAllStringSubmatch(s, -1) {
			families[m[1]] = true
		}
	}
	for key := range cat["en"] {
		if seen[key] || families[strings.SplitN(key, ".", 2)[0]] {
			continue
		}
		t.Errorf("key %q sits in the catalog but is never called", key)
	}
}

func TestFrontBothLanguagesCarryTheSameKeys(t *testing.T) {
	cat := locales(t)
	for key := range cat["en"] {
		if _, ok := cat["fr"][key]; !ok {
			t.Errorf("%q exists in English, not in French", key)
		}
	}
	for key := range cat["fr"] {
		if _, ok := cat["en"][key]; !ok {
			t.Errorf("%q exists in French, not in English", key)
		}
	}
}

func TestFrontPlaceholdersSuppliedByCaller(t *testing.T) {
	cat := locales(t)
	for name, s := range sources(t) {
		for _, call := range regexp.MustCompile(`\bt\(\s*'([a-z][a-zA-Z]*\.[a-zA-Z]+)'\s*,\s*\{([^}]*)\}`).FindAllStringSubmatch(s, -1) {
			key, supplied := call[1], call[2]
			for _, l := range []string{"en", "fr"} {
				for _, v := range placeholderRE.FindAllStringSubmatch(cat[l][key], -1) {
					if !strings.Contains(supplied, v[1]+":") {
						t.Errorf("%s: %q expects {%s}, not supplied (%s)", name, key, v[1], l)
					}
				}
			}
		}
	}
}
