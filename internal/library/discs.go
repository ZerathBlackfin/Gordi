package library

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gordi/internal/i18n"
)

var discWords = func() string {
	seen := map[string]bool{"cd": true, "disc": true, "disk": true}
	words := []string{"cd", "disc", "disk"}
	for _, l := range i18n.Langs {
		w := strings.ToLower(i18n.T(l, "folder.disc"))
		if w != "" && !seen[w] {
			seen[w] = true
			words = append(words, regexp.QuoteMeta(w))
		}
	}
	return strings.Join(words, "|")
}()

var discFolderRE = regexp.MustCompile(`(?i)^(?:` + discWords + `)\s*[-_.]?\s*(\d{1,2})$`)

func mergeDiscFolders(tracks map[string][]Track, extras map[string][]string) {
	mergeSiblingDiscFolders(tracks, extras)

	parents := map[string][]string{}
	for dir := range tracks {
		if !discFolderRE.MatchString(filepath.Base(dir)) {
			continue
		}
		parent := filepath.Dir(dir)
		if parent == dir || len(tracks[parent]) > 0 {
			continue
		}
		parents[parent] = append(parents[parent], dir)
	}

	for parent, discs := range parents {
		for _, dir := range discs {
			numberAndTotal, _ := strconv.Atoi(discFolderRE.FindStringSubmatch(filepath.Base(dir))[1])
			for _, t := range tracks[dir] {
				if t.DiscNo == 0 {
					t.DiscNo = numberAndTotal
				}
				tracks[parent] = append(tracks[parent], t)
			}
			extras[parent] = append(extras[parent], extras[dir]...)
			delete(tracks, dir)
			delete(extras, dir)
		}
		for i := range tracks[parent] {
			if tracks[parent][i].DiscTotal == 0 {
				tracks[parent][i].DiscTotal = len(discs)
			}
		}
	}
}

var discSuffixRE = regexp.MustCompile(`(?i)[\s._-]+(?:` + discWords + `)\s*[-_.]?\s*(\d{1,2})$`)

func mergeSiblingDiscFolders(tracks map[string][]Track, extras map[string][]string) {
	type family struct {
		dirs    []string
		numbers []int
	}
	families := map[string]*family{}

	for dir := range tracks {
		m := discSuffixRE.FindStringSubmatch(filepath.Base(dir))
		if m == nil {
			continue
		}
		base := strings.TrimSpace(discSuffixRE.ReplaceAllString(filepath.Base(dir), ""))
		if base == "" {
			continue
		}
		key := filepath.Join(filepath.Dir(dir), base)
		numberAndTotal, _ := strconv.Atoi(m[1])

		f := families[key]
		if f == nil {
			f = &family{}
			families[key] = f
		}
		f.dirs = append(f.dirs, dir)
		f.numbers = append(f.numbers, numberAndTotal)
	}

	for key, f := range families {
		if len(f.dirs) < 2 || len(tracks[key]) > 0 {
			continue
		}
		for i, dir := range f.dirs {
			for _, t := range tracks[dir] {
				if t.DiscNo == 0 {
					t.DiscNo = f.numbers[i]
				}
				tracks[key] = append(tracks[key], t)
			}
			extras[key] = append(extras[key], extras[dir]...)
			delete(tracks, dir)
			delete(extras, dir)
		}
		for i := range tracks[key] {
			if tracks[key][i].DiscTotal == 0 {
				tracks[key][i].DiscTotal = len(f.dirs)
			}
		}
	}
}
