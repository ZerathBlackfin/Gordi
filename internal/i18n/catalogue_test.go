package i18n

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var keyRE = regexp.MustCompile(`"([a-z][a-zA-Z]*\.[a-zA-Z]*)"`)

var prefixes = func() map[string]bool {
	out := map[string]bool{}
	for key := range catalog[EN] {
		out[strings.SplitN(key, ".", 2)[0]] = true
	}
	return out
}()

func usedKeys(t *testing.T) map[string]string {
	t.Helper()
	root := filepath.Join("..", "..")
	out := map[string]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		for _, m := range keyRE.FindAllStringSubmatch(string(data), -1) {
			if prefixes[strings.SplitN(m[1], ".", 2)[0]] {
				out[m[1]] = path
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatal("no call found: the detection pattern is broken")
	}
	return out
}

func TestEveryCalledKeyExists(t *testing.T) {
	for key, path := range usedKeys(t) {
		if strings.HasSuffix(key, ".") {
			continue
		}
		for _, l := range Langs {
			if _, ok := catalog[l][key]; !ok {
				t.Errorf("%s calls %q, missing from the %s catalog", path, key, l)
			}
		}
	}
}

func TestNoOrphanKey(t *testing.T) {
	used := usedKeys(t)
	for key := range catalog[EN] {
		if _, ok := used[strings.SplitN(key, ".", 2)[0]+"."]; ok {
			continue
		}
		if _, ok := used[key]; !ok {
			t.Errorf("key %q sits in the catalog but is never called", key)
		}
	}
}

func TestBothLanguagesCarryTheSameKeys(t *testing.T) {
	for key := range catalog[EN] {
		if _, ok := catalog[FR][key]; !ok {
			t.Errorf("%q exists in English, not in French", key)
		}
	}
	for key := range catalog[FR] {
		if _, ok := catalog[EN][key]; !ok {
			t.Errorf("%q exists in French, not in English", key)
		}
	}
}
