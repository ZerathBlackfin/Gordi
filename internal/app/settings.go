package app

import (
	"strconv"
	"strings"
	"time"

	"gordi/internal/apply"
	"gordi/internal/config"
	"gordi/internal/i18n"
	"gordi/internal/library"
	"gordi/internal/musicbrainz"
)

const (
	keyPattern       = "pattern"
	keyPatternMulti  = "pattern_multi"
	keyMode          = "mode"
	keyScanEvery     = "scan_every"
	keyPrefetchEvery = "prefetch_every"
	keyMBContact     = "mb_contact"
	keyLang          = "lang"
)

type Values struct {
	Pattern       string `json:"pattern"`
	PatternMulti  string `json:"pattern_multi"`
	Mode          string `json:"mode"`
	ScanEvery     int    `json:"scan_every"`
	PrefetchEvery int    `json:"prefetch_every"`
	MBContact     string `json:"mb_contact"`
	Lang          string `json:"lang"`
}

type Settings struct {
	Values

	Customized []string `json:"customized"`

	Languages []Lang     `json:"languages"`
	Fields    []string   `json:"fields"`
	Hint      string     `json:"hint"`
	Preview   []Example  `json:"preview"`
	Templates []Template `json:"templates"`
	Cache     int        `json:"cache"`
}

func (a *App) Patterns() apply.Patterns {
	return apply.Patterns{
		Simple: a.stringSetting(keyPattern, a.Cfg.Pattern),
		Multi:  a.stringSetting(keyPatternMulti, a.Cfg.PatternMulti),
	}
}

func (a *App) DefaultMode() string { return a.stringSetting(keyMode, string(a.Cfg.Mode)) }

func (a *App) MBContact() string { return a.stringSetting(keyMBContact, a.Cfg.MBContact) }

func (a *App) Lang() i18n.Lang {
	return i18n.ParseLang(a.stringSetting(keyLang, string(a.Cfg.Lang)))
}

func (a *App) ScanEvery() time.Duration {
	return time.Duration(a.intSetting(keyScanEvery, int(a.Cfg.ScanEvery.Seconds()))) * time.Second
}

func (a *App) PrefetchEvery() time.Duration {
	return time.Duration(a.intSetting(keyPrefetchEvery, int(a.Cfg.PrefetchEvery.Seconds()))) * time.Second
}

func (a *App) stringSetting(key, fallback string) string {
	if v, ok := a.Store.Setting(key); ok && strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}

func (a *App) intSetting(key string, fallback int) int {
	v, ok := a.Store.Setting(key)
	if !ok {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return fallback
	}
	return n
}

func (a *App) Settings() Settings {
	patterns := a.Patterns()
	lang := a.Lang()
	r := Settings{
		Values: Values{
			Pattern:       patterns.Simple,
			PatternMulti:  patterns.Multi,
			Mode:          a.DefaultMode(),
			ScanEvery:     int(a.ScanEvery().Seconds()),
			PrefetchEvery: int(a.PrefetchEvery().Seconds()),
			MBContact:     a.MBContact(),
			Lang:          string(lang),
		},
		Languages: languageList(),
		Fields:    apply.DisplayFields(lang),
		Hint:      i18n.T(lang, "hint.padding"),
		Templates: Templates(lang),
		Cache:     a.Store.CacheSize(),
	}
	r.Preview = Preview(patterns, lang)
	for _, key := range []string{keyPattern, keyPatternMulti, keyMode, keyScanEvery, keyPrefetchEvery, keyMBContact, keyLang} {
		if _, ok := a.Store.Setting(key); ok {
			r.Customized = append(r.Customized, key)
		}
	}
	if r.Customized == nil {
		r.Customized = []string{}
	}
	return r
}

type SettingsPatch struct {
	Pattern       *string `json:"pattern"`
	PatternMulti  *string `json:"pattern_multi"`
	Mode          *string `json:"mode"`
	ScanEvery     *int    `json:"scan_every"`
	PrefetchEvery *int    `json:"prefetch_every"`
	MBContact     *string `json:"mb_contact"`
	Lang          *string `json:"lang"`
}

func (a *App) Update(m SettingsPatch) error {
	if m.Pattern != nil {
		if err := a.setString(keyPattern, *m.Pattern, a.Cfg.Pattern, a.ValidatePattern); err != nil {
			return err
		}
	}
	if m.PatternMulti != nil {
		if err := a.setString(keyPatternMulti, *m.PatternMulti, a.Cfg.PatternMulti, a.ValidateMultiPattern); err != nil {
			return err
		}
	}
	if m.Mode != nil {
		if err := a.setString(keyMode, *m.Mode, string(a.Cfg.Mode), a.validateMode); err != nil {
			return err
		}
	}
	if m.MBContact != nil {
		if err := a.setString(keyMBContact, *m.MBContact, a.Cfg.MBContact, a.validateContact); err != nil {
			return err
		}
		a.MB.SetContact(a.MBContact())
	}
	if m.Lang != nil {
		if err := a.setString(keyLang, *m.Lang, string(a.Cfg.Lang), validateLang); err != nil {
			return err
		}
		a.MB.SetLang(a.Lang())
	}
	if m.ScanEvery != nil {
		if err := a.setInt(keyScanEvery, *m.ScanEvery, int(a.Cfg.ScanEvery.Seconds()), 5, 3600); err != nil {
			return err
		}
	}
	if m.PrefetchEvery != nil {
		if err := a.setInt(keyPrefetchEvery, *m.PrefetchEvery, int(a.Cfg.PrefetchEvery.Seconds()), 0, 3600); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) setString(key, value, fallback string, validate func(string) error) error {
	value = strings.TrimSpace(value)
	if value == "" || value == fallback {
		return a.Store.DeleteSetting(key)
	}
	if err := validate(value); err != nil {
		return err
	}
	return a.Store.PutSetting(key, value)
}

func (a *App) setInt(key string, value, fallback, min, max int) error {
	if value < min || value > max {
		return i18n.Errorf(a.Lang(), "setting.outOfRange", key, min, max)
	}
	if value == fallback {
		return a.Store.DeleteSetting(key)
	}
	return a.Store.PutSetting(key, strconv.Itoa(value))
}

func (a *App) ClearCache() (int, error) {
	n, err := a.Store.ClearCache()
	a.Nudge()
	return n, err
}

func (a *App) validateMode(v string) error {
	if v != string(config.ModeCopy) && v != string(config.ModeMove) {
		return i18n.Errorf(a.Lang(), "setting.unknownMode")
	}
	return nil
}

func (a *App) validateContact(v string) error {
	if !strings.Contains(v, "@") && !strings.HasPrefix(v, "http") {
		return i18n.Errorf(a.Lang(), "setting.contact")
	}
	return nil
}

func validateLang(string) error { return nil }

type Lang struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

func languageList() []Lang {
	out := make([]Lang, 0, len(i18n.Langs))
	for _, l := range i18n.Langs {
		out = append(out, Lang{Code: string(l), Name: i18n.Name(l)})
	}
	return out
}

func (a *App) ValidatePattern(pattern string) error {
	lang := a.Lang()
	if strings.HasPrefix(pattern, "/") {
		return i18n.Errorf(lang, "pattern.absolute")
	}
	if strings.Contains(pattern, "..") {
		return i18n.Errorf(lang, "pattern.parentEscape")
	}
	if strings.ContainsAny(pattern, "<>") {
		return i18n.Errorf(lang, "pattern.angleBrackets")
	}
	if !apply.HasField(pattern, "title") && !apply.HasField(pattern, "track") {
		return i18n.Errorf(lang, "pattern.noTitle")
	}
	return apply.ValidateFields(pattern, lang)
}

func (a *App) ValidateMultiPattern(pattern string) error {
	if err := a.ValidatePattern(pattern); err != nil {
		return err
	}
	if !apply.HasField(pattern, "disc") {
		return i18n.Errorf(a.Lang(), "pattern.noDisc")
	}
	return nil
}

func (a *App) Preview(m SettingsPatch) (apply.Patterns, []Example, error) {
	patterns := a.Patterns()
	if m.Pattern != nil {
		patterns.Simple = *m.Pattern
	}
	if m.PatternMulti != nil {
		patterns.Multi = *m.PatternMulti
	}
	if err := a.ValidatePattern(patterns.Simple); err != nil {
		return patterns, nil, err
	}
	if err := a.ValidateMultiPattern(patterns.Multi); err != nil {
		return patterns, nil, err
	}
	return patterns, Preview(patterns, a.Lang()), nil
}

type Example struct {
	Pattern string `json:"pattern"`
	Case    string `json:"case"`
	Path    string `json:"path"`
}

type Template struct {
	Name         string `json:"name"`
	Pattern      string `json:"pattern"`
	PatternMulti string `json:"pattern_multi"`
	Example      string `json:"example"`
}

var templates = []struct{ Key, Pattern, PatternMulti string }{
	{"template.artistAlbum",
		"{artist}/{album} ({year})/{track} - {title}",
		"{artist}/{album} ({year})/CD{disc:0}/{track} - {title}"},
	{"template.yearAlbum",
		"{artist}/{year} - {album}/{track} - {title}",
		"{artist}/{year} - {album}/CD{disc:0}/{track} - {title}"},
	{"template.artistDash",
		"{artist} - {album} ({year})/{track} - {title}",
		"{artist} - {album} ({year})/CD{disc:0}/{track} - {title}"},
	{"template.singleFolder",
		"{artist} - {album} ({year})/{track} - {artist} - {title}",
		"{artist} - {album} ({year})/{disc:0}-{track} - {artist} - {title}"},
}

func Templates(lang i18n.Lang) []Template {
	out := make([]Template, 0, len(templates))
	for _, m := range templates {
		out = append(out, Template{
			Name:         i18n.T(lang, m.Key),
			Pattern:      m.Pattern,
			PatternMulti: m.PatternMulti,
			Example:      render(apply.Patterns{Simple: m.Pattern, Multi: m.PatternMulti}, previewCases[0], lang),
		})
	}
	return out
}

type previewCase struct {
	pattern string
	key     string
	artist  string
	album   string
	date    string
	disc    int
	discs   int
	track   int
	title   string
}

// Short on purpose: each example has to read on one line.
var previewCases = []previewCase{
	{"pattern", "case.ordinary", "Pink Floyd", "Animals", "1977-01-21", 1, 1, 2, "Dogs"},
	{"pattern", "case.noYear", "Pink Floyd", "Animals", "", 1, 1, 2, "Dogs"},
	{"pattern_multi", "case.boxSet", "Pink Floyd", "The Wall", "1979-11-30",
		2, 2, 6, "Comfortably Numb"},
}

func Preview(patterns apply.Patterns, lang i18n.Lang) []Example {
	out := make([]Example, 0, len(previewCases))
	for _, e := range previewCases {
		path := render(patterns, e, lang)
		if path == "" {
			return nil
		}
		out = append(out, Example{Pattern: e.pattern, Case: i18n.T(lang, e.key), Path: path})
	}
	return out
}

func render(patterns apply.Patterns, e previewCase, lang i18n.Lang) string {
	album := library.Album{Tracks: []library.Track{{
		RelPath: "previewCase.flac", File: "previewCase.flac", Ext: "flac",
		TrackNo: e.track, DiscNo: e.disc,
	}}}
	release := musicbrainz.ReleaseDetail{
		Release: musicbrainz.Release{
			Title: e.album, Artist: e.artist, Date: e.date, Format: "CD",
		},
		Tracks: []musicbrainz.Track{{
			Position: e.track, Disc: e.disc, Title: e.title, Artist: e.artist,
		}},
	}
	for i := 1; i <= e.discs; i++ {
		release.Media = append(release.Media, musicbrainz.Medium{Position: i, Format: "CD"})
	}
	plan, err := apply.Prepare(album, release, patterns, lang)
	if err != nil || len(plan.Tracks) == 0 {
		return ""
	}
	return plan.Tracks[0].Destination
}
