package apply

import (
	"strings"
	"unicode"

	"gordi/internal/i18n"
	"gordi/internal/library"
	"gordi/internal/musicbrainz"
)

type MatchMethod string

const (
	MatchByNumber MatchMethod = "track number"
	MatchByTitle  MatchMethod = "title"
	MatchByOrder  MatchMethod = "file order"
)

func match(files []library.Track, tracks []musicbrainz.Track, lang i18n.Lang) ([]int, []MatchMethod, error) {
	picks := make([]int, len(files))
	methods := make([]MatchMethod, len(files))
	for i := range picks {
		picks[i] = -1
	}
	taken := make([]bool, len(tracks))

	parNumero := map[[2]int]int{}
	byTitle := map[string]int{}
	for i, p := range tracks {
		parNumero[[2]int{max(p.Disc, 1), p.Position}] = i
		if key := normalize(p.Title); key != "" {
			if _, already := byTitle[key]; !already {
				byTitle[key] = i
			}
		}
	}

	take := func(f, p int, m MatchMethod) {
		picks[f], methods[f], taken[p] = p, m, true
	}

	for i, f := range files {
		if f.TrackNo <= 0 {
			continue
		}
		if p, ok := parNumero[[2]int{max(f.DiscNo, 1), f.TrackNo}]; ok && !taken[p] {
			take(i, p, MatchByNumber)
		}
	}

	for i, f := range files {
		if picks[i] >= 0 {
			continue
		}
		if p, ok := byTitle[normalize(f.Title)]; ok && !taken[p] {
			take(i, p, MatchByTitle)
		}
	}

	next := 0
	for i := range files {
		if picks[i] >= 0 {
			continue
		}
		for next < len(tracks) && taken[next] {
			next++
		}
		if next >= len(tracks) {
			return nil, nil, i18n.Errorf(lang, "filing.unmatchedFile", files[i].File)
		}
		take(i, next, MatchByOrder)
	}
	return picks, methods, nil
}

func normalize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
