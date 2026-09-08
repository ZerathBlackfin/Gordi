package apply

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gordi/internal/i18n"
	"gordi/internal/library"
	"gordi/internal/musicbrainz"

	"go.senan.xyz/taglib"
)

type Patterns struct {
	Simple string `json:"pattern"`
	Multi  string `json:"pattern_multi"`
}

func (m Patterns) For(discs int) string {
	if discs > 1 && strings.TrimSpace(m.Multi) != "" {
		return m.Multi
	}
	return m.Simple
}

type PlannedTrack struct {
	Source      string              `json:"source"`
	Destination string              `json:"destination"`
	Index       int                 `json:"index"`
	Method      MatchMethod         `json:"method"`
	Tags        map[string][]string `json:"tags"`
}

type Plan struct {
	Mode       string         `json:"mode"`
	Tracks     []PlannedTrack `json:"tracks"`
	Ignored    []string       `json:"ignored"`
	Warnings   []string       `json:"warnings"`
	Cover      string         `json:"cover"`
	CoverEmbed bool           `json:"cover_embed"`

	Art Art `json:"-"`
}

type Art struct {
	Image []byte
	Embed bool
}

const CoverFile = "cover.jpg"

func CoverPath(p Plan) string {
	if len(p.Tracks) == 0 {
		return ""
	}
	dir := path.Dir(filepath.ToSlash(p.Tracks[0].Destination))
	for _, t := range p.Tracks[1:] {
		for dir != "." && !strings.HasPrefix(filepath.ToSlash(t.Destination), dir+"/") {
			dir = path.Dir(dir)
		}
	}
	if dir == "." {
		return CoverFile
	}
	return dir + "/" + CoverFile
}

func Prepare(album library.Album, release musicbrainz.ReleaseDetail, patterns Patterns, lang i18n.Lang) (Plan, error) {
	if len(album.Tracks) == 0 {
		return Plan{}, i18n.Errorf(lang, "filing.noFile")
	}
	if len(album.Tracks) > len(release.Tracks) {
		return Plan{}, i18n.Errorf(lang, "filing.tooManyFiles",
			len(album.Tracks), len(release.Tracks))
	}

	picks, methods, err := match(album.Tracks, release.Tracks, lang)
	if err != nil {
		return Plan{}, err
	}

	p := Plan{Ignored: album.Extras}
	if len(album.Tracks) < len(release.Tracks) {
		p.Warnings = append(p.Warnings,
			i18n.T(lang, "filing.incompleteAlbum", len(release.Tracks)-len(album.Tracks)))
	}
	if n := count(methods, MatchByOrder); n > 0 {
		p.Warnings = append(p.Warnings, i18n.T(lang, "filing.matchedByOrder", n))
	}

	discs := discCount(release)
	pattern := patterns.For(discs)
	destinations := map[string]bool{}

	for i, file := range album.Tracks {
		track := release.Tracks[picks[i]]
		tags := tagsFor(file, track, release, discs)

		dest := destinationPath(pattern, tags, formatOf(release), file.Ext)
		if destinations[dest] {
			return Plan{}, i18n.Errorf(lang, "filing.sameDestination", dest)
		}
		destinations[dest] = true

		p.Tracks = append(p.Tracks, PlannedTrack{
			Source:      file.RelPath,
			Destination: dest,
			Index:       picks[i],
			Method:      methods[i],
			Tags:        tags,
		})
	}
	return p, nil
}

func count(methods []MatchMethod, wanted MatchMethod) int {
	n := 0
	for _, m := range methods {
		if m == wanted {
			n++
		}
	}
	return n
}

func formatOf(release musicbrainz.ReleaseDetail) string {
	if release.Format != "" {
		return release.Format
	}
	for _, m := range release.Media {
		if m.Format != "" {
			return m.Format
		}
	}
	return ""
}

func discCount(release musicbrainz.ReleaseDetail) int {
	n := 1
	for _, m := range release.Media {
		if m.Position > n {
			n = m.Position
		}
	}
	return n
}

func tagsFor(file library.Track, track musicbrainz.Track, release musicbrainz.ReleaseDetail, discs int) map[string][]string {
	tags := map[string][]string{}
	for key, values := range file.Raw {
		tags[key] = append([]string(nil), values...)
	}

	set := func(key, value string) {
		if value = strings.TrimSpace(value); value != "" {
			tags[key] = []string{value}
		}
	}
	setAll := func(key string, values []string) {
		if merged := merge(values); len(merged) > 0 {
			tags[key] = merged
		}
	}

	set(taglib.Title, track.Title)
	set(taglib.Artist, track.Artist)
	set(taglib.AlbumArtist, release.Artist)
	set(taglib.Album, release.Title)
	set(taglib.Date, release.Date)
	set(taglib.OriginalDate, release.FirstRelease)
	set(taglib.TrackNumber, strconv.Itoa(track.Position))
	set("TRACKTOTAL", strconv.Itoa(len(release.Tracks)))
	set(taglib.DiscNumber, strconv.Itoa(max(track.Disc, 1)))
	set("DISCTOTAL", strconv.Itoa(discs))
	set(taglib.AlbumArtistSort, release.ArtistSort)
	set(taglib.ArtistSort, track.ArtistSort)
	setAll(taglib.Artists, track.Artists)
	setAll("ALBUMARTISTS", release.Artists)
	set(taglib.DiscSubtitle, track.DiscTitle)
	set("ORIGINALYEAR", year(release.FirstRelease))
	set(taglib.ASIN, release.ASIN)
	set(taglib.Script, release.Script)
	set(taglib.Language, release.Language)
	setAll(taglib.ReleaseType, releaseTypes(release))
	set(taglib.Label, release.Label)
	set(taglib.CatalogNumber, release.Catalog)
	set(taglib.Barcode, release.Barcode)
	set(taglib.Media, release.Format)
	set(taglib.ReleaseCountry, release.Country)
	set(taglib.ReleaseStatus, strings.ToLower(release.Status))
	set(taglib.MusicBrainzAlbumID, release.ID)
	set(taglib.MusicBrainzTrackID, track.RecordingID)
	set(taglib.MusicBrainzReleaseTrackID, track.TrackID)
	set(taglib.MusicBrainzArtistID, track.ArtistID)
	set(taglib.MusicBrainzAlbumArtistID, release.ArtistID)
	set(taglib.MusicBrainzReleaseGroupID, release.ReleaseGroupID)
	setAll(taglib.ISRC, track.ISRCs)
	setAll(taglib.Genre, release.Genres)

	album, own := release.Credits, track.Credits
	setAll(taglib.Producer, merge(album.Producers, own.Producers))
	setAll(taglib.Mixer, merge(album.Mixers, own.Mixers))
	setAll(taglib.Engineer, merge(album.Engineers, own.Engineers))
	setAll(taglib.Arranger, merge(album.Arrangers, own.Arrangers))
	setAll(taglib.Conductor, merge(album.Conductors, own.Conductors))
	setAll(taglib.Performer, merge(album.Performers, own.Performers))
	setAll(taglib.Composer, merge(album.Composers, own.Composers))
	setAll(taglib.Lyricist, merge(album.Lyricists, own.Lyricists))
	setAll("WRITER", merge(album.Writers, own.Writers))
	set(taglib.Work, own.Work)
	set(taglib.MusicBrainzWorkID, own.WorkID)
	set("ISWC", own.ISWC)
	return tags
}

func releaseTypes(release musicbrainz.ReleaseDetail) []string {
	var out []string
	if release.PrimaryType != "" {
		out = append(out, strings.ToLower(release.PrimaryType))
	}
	for _, t := range release.SecondaryTypes {
		out = append(out, strings.ToLower(t))
	}
	return out
}

func merge(lists ...[]string) []string {
	var out []string
	seen := map[string]bool{}
	for _, list := range lists {
		for _, v := range list {
			if v = strings.TrimSpace(v); v != "" && !seen[v] {
				seen[v] = true
				out = append(out, v)
			}
		}
	}
	return out
}

func destinationPath(pattern string, tags map[string][]string, format, ext string) string {
	value := func(key string) string {
		if v := tags[key]; len(v) > 0 {
			return v[0]
		}
		return ""
	}

	texts := map[string]string{
		"artist": firstNonEmpty(value(taglib.AlbumArtist), value(taglib.Artist), "Unknown artist"),
		"album":  firstNonEmpty(value(taglib.Album), "Unknown album"),
		"year":   year(value(taglib.Date)),
		"title":  firstNonEmpty(value(taglib.Title), "Untitled"),
		"format": firstNonEmpty(format, value(taglib.Media)),
	}
	numbers := map[string]int{
		"track": atoi(value(taglib.TrackNumber)),
		"disc":  max(atoi(value(taglib.DiscNumber)), 1),
	}

	var parts []string
	for _, m := range strings.Split(pattern, "/") {
		m = expand(m, texts, numbers)
		if clean := sanitize(m); clean != "" {
			parts = append(parts, clean)
		}
	}
	if len(parts) == 0 {
		parts = []string{"Untitled"}
	}
	return filepath.Join(parts...) + "." + ext
}

func expand(part string, texts map[string]string, numbers map[string]int) string {
	return FieldRE.ReplaceAllStringFunc(part, func(token string) string {
		parts := FieldRE.FindStringSubmatch(token)
		padding := parts[2]

		field, known := Resolve(parts[1])
		if !known {
			return "" // unknown field: validation already refused it
		}
		name := field.Name

		if n, ok := numbers[name]; ok {
			width := DefaultWidth
			if padding != "" {
				width = len(padding)
			}
			return fmt.Sprintf("%0*d", width, n)
		}
		return texts[name]
	})
}

func atoi(v string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(v))
	return n
}

var yearRE = regexp.MustCompile(`(19|20)\d{2}`)

func year(date string) string {
	return yearRE.FindString(date)
}

var forbiddenRE = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)

var emptyGroupsRE = regexp.MustCompile(`\(\s*\)|\[\s*\]|\{\s*\}`)

var separatorsRE = regexp.MustCompile(`\s*([-_·])\s*(?:[-_·]\s*)+`)

func sanitize(s string) string {
	s = forbiddenRE.ReplaceAllString(s, "-")
	s = emptyGroupsRE.ReplaceAllString(s, " ")
	s = separatorsRE.ReplaceAllString(s, " $1 ")
	s = strings.Join(strings.Fields(s), " ")
	s = strings.Trim(s, " .-_·")
	if s == "" {
		return ""
	}
	if len(s) > 150 {
		s = strings.TrimSpace(s[:150])
	}
	return s
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
