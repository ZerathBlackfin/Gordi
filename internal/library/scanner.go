package library

import (
	"fmt"
	"gordi/internal/i18n"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.senan.xyz/taglib"
)

var audioExt = map[string]bool{
	".mp3":  true,
	".flac": true,
	".m4a":  true,
	".m4b":  true,
	".aac":  true,
	".ogg":  true,
	".oga":  true,
	".opus": true,
	".wav":  true,
	".wma":  true,
	".aiff": true,
	".aif":  true,
}

type Image struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

type Audio struct {
	Length     int     `json:"length"`
	Bitrate    int     `json:"bitrate"`
	SampleRate int     `json:"sample_rate"`
	BitDepth   int     `json:"bit_depth"`
	Channels   int     `json:"channels"`
	Cover      string  `json:"cover"`
	Images     []Image `json:"images"`
}

type Track struct {
	RelPath  string    `json:"rel_path"`
	File     string    `json:"file"`
	Ext      string    `json:"ext"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
	Tagged   bool      `json:"tagged"`

	Title       string `json:"title"`
	Artist      string `json:"artist"`
	AlbumArtist string `json:"album_artist"`
	Album       string `json:"album"`
	Composer    string `json:"composer"`
	Genre       string `json:"genre"`
	Date        string `json:"date"`
	TrackNo     int    `json:"track_no"`
	TrackTotal  int    `json:"track_total"`
	DiscNo      int    `json:"disc_no"`
	DiscTotal   int    `json:"disc_total"`
	ISRC        string `json:"isrc"`
	Copyright   string `json:"copyright"`
	Comment     string `json:"comment"`
	Label       string `json:"label"`
	Catalog     string `json:"catalog"`
	Barcode     string `json:"barcode"`
	MBTrackID   string `json:"mb_track_id"`
	MBAlbumID   string `json:"mb_album_id"`

	Audio Audio               `json:"audio"`
	Raw   map[string][]string `json:"raw"`
}

type Album struct {
	RelDir string `json:"rel_dir"`
	Artist string `json:"artist"`
	Title  string `json:"title"`
	Year   int    `json:"year"`

	Date      string   `json:"date"`
	Genre     string   `json:"genre"`
	Discs     int      `json:"discs"`
	Length    int      `json:"length"`
	Formats   []string `json:"formats"`
	Quality   string   `json:"quality"`
	Covers    int      `json:"covers"`
	Untagged  int      `json:"untagged"`
	Label     string   `json:"label"`
	Catalog   string   `json:"catalog"`
	Barcode   string   `json:"barcode"`
	MBAlbumID string   `json:"mb_album_id"`
	Extras    []string `json:"extras"`
	Size      int64    `json:"size"`

	Tracks []Track `json:"tracks,omitempty"`
}

func Scan(root string, known map[string]Track, lang i18n.Lang) ([]Album, error) {
	if _, err := os.Stat(root); err != nil {
		return nil, i18n.Errorf(lang, "scan.unreadableFolder", err)
	}

	tracks := map[string][]Track{}
	extras := map[string][]string{}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == root {
				return err
			}
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && path != root {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		dir := filepath.Dir(rel)

		ext := strings.ToLower(filepath.Ext(d.Name()))
		if !audioExt[ext] {
			extras[dir] = append(extras[dir], d.Name())
			return nil
		}
		tracks[dir] = append(tracks[dir], readTrack(path, rel, ext, d, known))
		return nil
	})
	if err != nil {
		return nil, err
	}

	mergeDiscFolders(tracks, extras)

	albums := make([]Album, 0, len(tracks))
	for dir, tracks := range tracks {
		sort.Slice(tracks, func(i, j int) bool {
			if tracks[i].DiscNo != tracks[j].DiscNo {
				return tracks[i].DiscNo < tracks[j].DiscNo
			}
			if tracks[i].TrackNo != tracks[j].TrackNo {
				return tracks[i].TrackNo < tracks[j].TrackNo
			}
			return tracks[i].File < tracks[j].File
		})
		albums = append(albums, buildAlbum(dir, tracks, extras[dir]))
	}
	sort.Slice(albums, func(i, j int) bool { return albums[i].RelDir < albums[j].RelDir })
	return albums, nil
}

func readTrack(path, rel, ext string, d fs.DirEntry, known map[string]Track) Track {
	t := Track{
		RelPath: rel,
		File:    d.Name(),
		Ext:     strings.TrimPrefix(ext, "."),
		Title:   strings.TrimSuffix(d.Name(), filepath.Ext(d.Name())),
	}
	if info, err := d.Info(); err == nil {
		t.Size = info.Size()
		t.Modified = info.ModTime()

		if known, ok := known[rel]; ok && known.Size == t.Size && known.Modified.Equal(t.Modified) {
			return known
		}
	}

	if props, err := taglib.ReadProperties(path); err == nil {
		t.Audio = Audio{
			Length:     int(props.Length.Milliseconds()),
			Bitrate:    int(props.BitRate),
			SampleRate: int(props.SampleRate),
			BitDepth:   int(props.BitDepth),
			Channels:   int(props.Channels),
		}
		for _, img := range props.Images {
			t.Audio.Images = append(t.Audio.Images, Image{
				Type:        img.Type,
				Description: img.Description,
			})
		}
		if len(props.Images) > 0 {
			t.Audio.Cover = firstNonEmpty(props.Images[0].MIMEType, "image")
		}
	}

	tags, err := taglib.ReadTags(path)
	if err != nil || len(tags) == 0 {
		return t
	}
	t.Tagged = true
	t.Raw = tags

	if v := tagValue(tags, taglib.Title); v != "" {
		t.Title = v
	}
	t.Artist = tagValue(tags, taglib.Artist)
	t.AlbumArtist = tagValue(tags, taglib.AlbumArtist)
	t.Album = tagValue(tags, taglib.Album)
	t.Composer = tagValue(tags, taglib.Composer)
	t.Genre = tagValue(tags, taglib.Genre)
	t.Date = tagValue(tags, taglib.Date)
	t.ISRC = tagValue(tags, taglib.ISRC)
	t.Copyright = tagValue(tags, taglib.Copyright)
	t.Comment = tagValue(tags, taglib.Comment)
	t.Label = tagValue(tags, taglib.Label)
	t.Catalog = tagValue(tags, taglib.CatalogNumber)
	t.Barcode = tagValue(tags, taglib.Barcode)
	t.MBTrackID = tagValue(tags, taglib.MusicBrainzTrackID)
	t.MBAlbumID = tagValue(tags, taglib.MusicBrainzAlbumID)

	t.TrackNo, t.TrackTotal = numberAndTotal(tags, taglib.TrackNumber, "TRACKTOTAL", "TOTALTRACKS")
	t.DiscNo, t.DiscTotal = numberAndTotal(tags, taglib.DiscNumber, "DISCTOTAL", "TOTALDISCS")
	return t
}

func buildAlbum(dir string, tracks []Track, extras []string) Album {
	a := Album{RelDir: dir, Tracks: tracks, Extras: extras, Discs: 1}

	a.Artist = majority(tracks, func(t Track) string { return t.AlbumArtist })
	if a.Artist == "" {
		a.Artist = majority(tracks, func(t Track) string { return t.Artist })
	}
	a.Title = majority(tracks, func(t Track) string { return t.Album })
	a.Date = majority(tracks, func(t Track) string { return t.Date })
	a.Genre = majority(tracks, func(t Track) string { return t.Genre })
	a.Label = majority(tracks, func(t Track) string { return t.Label })
	a.Catalog = majority(tracks, func(t Track) string { return t.Catalog })
	a.Barcode = majority(tracks, func(t Track) string { return t.Barcode })
	a.MBAlbumID = majority(tracks, func(t Track) string { return t.MBAlbumID })

	formats := map[string]bool{}
	for _, t := range tracks {
		a.Length += t.Audio.Length
		a.Size += t.Size
		if t.Audio.Cover != "" {
			a.Covers++
		}
		if !t.Tagged {
			a.Untagged++
		}
		if t.DiscNo > a.Discs {
			a.Discs = t.DiscNo
		}
		if t.DiscTotal > a.Discs {
			a.Discs = t.DiscTotal
		}
		formats[strings.ToUpper(t.Ext)] = true
	}
	for f := range formats {
		a.Formats = append(a.Formats, f)
	}
	sort.Strings(a.Formats)
	a.Quality = quality(tracks)

	base := filepath.Base(dir)
	if base == "." || base == string(filepath.Separator) {
		base = ""
	}
	folderArtist, folderTitle := splitFolderName(base)
	if a.Artist == "" {
		a.Artist = folderArtist
	}
	if a.Title == "" {
		a.Title = folderTitle
	}
	a.Year = year(a.Date)
	if a.Year == 0 {
		a.Year = yearInText(base)
	}
	return a
}

func quality(tracks []Track) string {
	if len(tracks) == 0 {
		return ""
	}
	a := tracks[0].Audio
	parts := []string{strings.ToUpper(tracks[0].Ext)}
	if a.BitDepth > 0 {
		parts = append(parts, fmt.Sprintf("%d bits", a.BitDepth))
	}
	if a.SampleRate > 0 {
		khz := strconv.FormatFloat(float64(a.SampleRate)/1000, 'f', -1, 64)
		parts = append(parts, khz+" kHz")
	}
	if a.BitDepth == 0 && a.Bitrate > 0 {
		parts = append(parts, fmt.Sprintf("%d kbit/s", a.Bitrate))
	}
	return strings.Join(parts, " · ")
}

func tagValue(tags map[string][]string, key string) string {
	for _, v := range tags[key] {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

func numberAndTotal(tags map[string][]string, key string, totalKeys ...string) (n, total int) {
	raw := tagValue(tags, key)
	if head, tail, cut := strings.Cut(raw, "/"); cut {
		raw = head
		total, _ = strconv.Atoi(strings.TrimSpace(tail))
	}
	n, _ = strconv.Atoi(strings.TrimSpace(raw))
	if total == 0 {
		for _, c := range totalKeys {
			if total, _ = strconv.Atoi(tagValue(tags, c)); total > 0 {
				break
			}
		}
	}
	return n, total
}

var yearRE = regexp.MustCompile(`(19|20)\d{2}`)

func year(date string) int {
	m := yearRE.FindString(date)
	n, _ := strconv.Atoi(m)
	return n
}

var yearInNameRE = regexp.MustCompile(`[\(\[\s]((19|20)\d{2})[\)\]\s]?`)

func yearInText(base string) int {
	m := yearInNameRE.FindStringSubmatch(base)
	if len(m) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

func splitFolderName(base string) (artist, title string) {
	clean := strings.TrimSpace(yearInNameRE.ReplaceAllString(base, " "))
	clean = strings.Trim(clean, " -_")
	if parts := strings.SplitN(clean, " - ", 2); len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return "", clean
}

func majority(tracks []Track, get func(Track) string) string {
	counts := map[string]int{}
	for _, t := range tracks {
		if v := strings.TrimSpace(get(t)); v != "" {
			counts[v]++
		}
	}
	best, bestN := "", 0
	for v, n := range counts {
		if n > bestN || (n == bestN && v < best) {
			best, bestN = v, n
		}
	}
	return best
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func Cover(path string) ([]byte, error) {
	return taglib.ReadImage(path)
}
