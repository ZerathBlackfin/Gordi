package apply

import (
	"os"
	"path/filepath"
	"testing"

	"gordi/internal/i18n"
	"gordi/internal/library"
	"gordi/internal/musicbrainz"
)

var pattern = Patterns{
	Simple: "{artist}/{album} ({year})/{track} - {title}",
	Multi:  "{artist}/{album} ({year})/CD{disc:0}/{track} - {title}",
}

func testAlbum(tracks ...string) library.Album {
	a := library.Album{RelDir: "junk"}
	for i, name := range tracks {
		a.Tracks = append(a.Tracks, library.Track{
			RelPath: "junk/" + name,
			File:    name,
			Ext:     "flac",
			Raw:     map[string][]string{"QBZ:TID": {"123"}},
		})
		_ = i
	}
	return a
}

func testRelease(titles ...string) musicbrainz.ReleaseDetail {
	r := musicbrainz.ReleaseDetail{
		Release: musicbrainz.Release{
			ID:      "abc-123",
			Title:   "The Dark Side of the Moon",
			Artist:  "Pink Floyd",
			Date:    "1973-03-01",
			Label:   "Harvest",
			Catalog: "SHVL 804",
		},
	}
	for i, t := range titles {
		r.Tracks = append(r.Tracks, musicbrainz.Track{
			Position:    i + 1,
			Disc:        1,
			Title:       t,
			Artist:      "Pink Floyd",
			RecordingID: "rec-" + t,
		})
	}
	r.Media = []musicbrainz.Medium{{Position: 1, Format: "CD", TrackCount: len(titles)}}
	return r
}

func TestPrepareBuildsPaths(t *testing.T) {
	p, err := Prepare(testAlbum("a.flac", "b.flac"), testRelease("Speak to Me", "Breathe (In the Air)"), pattern, i18n.EN)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		"Pink Floyd/The Dark Side of the Moon (1973)/01 - Speak to Me.flac",
		"Pink Floyd/The Dark Side of the Moon (1973)/02 - Breathe (In the Air).flac",
	}
	for i, want := range want {
		if p.Tracks[i].Destination != want {
			t.Errorf("track %d : %q, want %q", i, p.Tracks[i].Destination, want)
		}
	}
	if got := p.Tracks[0].Tags["MUSICBRAINZ_ALBUMID"]; len(got) != 1 || got[0] != "abc-123" {
		t.Errorf("MusicBrainz identifier missing from the tags: %v", got)
	}
	if got := p.Tracks[0].Tags["QBZ:TID"]; len(got) != 1 {
		t.Error("tags the file carries that we do not know must be kept")
	}
}

func TestPrepareSanitizesNames(t *testing.T) {
	r := testRelease("Blood and Sand / Milk")
	r.Title = `AC/DC: Live?`
	r.Artist = "AC/DC"

	p, err := Prepare(testAlbum("a.flac"), r, pattern, i18n.EN)
	if err != nil {
		t.Fatal(err)
	}
	dest := p.Tracks[0].Destination
	for _, forbidden := range []string{":", "?", `\`} {
		if filepath.Base(dest) == "" || contains(dest, forbidden) {
			t.Errorf("forbidden character %q in %q", forbidden, dest)
		}
	}
	if dest != "AC-DC/AC-DC- Live- (1973)/01 - Blood and Sand - Milk.flac" {
		t.Errorf("unexpected cleanup: %q", dest)
	}
}

func TestPrepareRejectsMoreFilesThanTracks(t *testing.T) {
	if _, err := Prepare(testAlbum("a.flac", "b.flac"), testRelease("Only"), pattern, i18n.EN); err == nil {
		t.Fatal("filing 2 files onto 1 track must be refused")
	}
}

func TestExecuteLeavesNoDamageOnFailure(t *testing.T) {
	inbox, library := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(inbox, "junk"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inbox, "junk", "a.flac"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := Prepare(testAlbum("a.flac", "b.flac"), testRelease("One", "Two"), pattern, i18n.EN)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Execute(p, inbox, library, ModeMove, i18n.EN); err == nil {
		t.Fatal("execution should have failed")
	}

	left := 0
	filepath.WalkDir(library, func(_ string, d os.DirEntry, _ error) error {
		if d != nil && !d.IsDir() {
			left++
		}
		return nil
	})
	if left != 0 {
		t.Errorf("%d file(s) left in the library after a failure", left)
	}
	if _, err := os.Stat(filepath.Join(inbox, "junk", "a.flac")); err != nil {
		t.Error("the original was deleted even though filing failed")
	}
}

func TestExecuteRefusesToOverwrite(t *testing.T) {
	inbox, library := t.TempDir(), t.TempDir()
	os.MkdirAll(filepath.Join(inbox, "junk"), 0o755)
	os.WriteFile(filepath.Join(inbox, "junk", "a.flac"), []byte("x"), 0o644)

	p, _ := Prepare(testAlbum("a.flac"), testRelease("One"), pattern, i18n.EN)
	dest := filepath.Join(library, filepath.FromSlash(p.Tracks[0].Destination))
	os.MkdirAll(filepath.Dir(dest), 0o755)
	os.WriteFile(dest, []byte("already here"), 0o644)

	if _, err := Execute(p, inbox, library, ModeCopy, i18n.EN); err == nil {
		t.Fatal("overwriting an existing file must be refused")
	}
	if content, _ := os.ReadFile(dest); string(content) != "already here" {
		t.Error("the existing file was changed")
	}
}

func TestDestinationStaysInsideLibrary(t *testing.T) {
	if _, err := safeDestination("/library", "../../etc/passwd", i18n.EN); err == nil {
		t.Fatal("a path leaving the library must be refused")
	}
	if _, err := safeDestination("/library", "Pink Floyd/ok.flac", i18n.EN); err != nil {
		t.Fatalf("ordinary path refused: %v", err)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestMatchingFollowsTrackNumbers(t *testing.T) {
	album := library.Album{Tracks: []library.Track{
		{RelPath: "a/01.flac", File: "01.flac", Ext: "flac", TrackNo: 1, DiscNo: 1},
		{RelPath: "a/03.flac", File: "03.flac", Ext: "flac", TrackNo: 3, DiscNo: 1},
	}}
	release := testRelease("One", "Two", "Three")

	p, err := Prepare(album, release, pattern, i18n.EN)
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Tracks[1].Tags["TITLE"][0]; got != "Three" {
		t.Fatalf("the file numbered 3 must get track 3, got %q", got)
	}
	if p.Tracks[1].Method != MatchByNumber {
		t.Errorf("expected match method %q, got %q", MatchByNumber, p.Tracks[1].Method)
	}
	if p.Tracks[1].Index != 2 {
		t.Errorf("want index 2, got %d", p.Tracks[1].Index)
	}
	if len(p.Warnings) == 0 {
		t.Error("an incomplete album must be flagged")
	}
}

func TestMatchingByTitle(t *testing.T) {
	album := library.Album{Tracks: []library.Track{
		{RelPath: "a/x.flac", File: "x.flac", Ext: "flac", Title: "breathe (in the air)"},
	}}
	p, err := Prepare(album, testRelease("Speak to Me", "Breathe (In the Air)"), pattern, i18n.EN)
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Tracks[0].Tags["TITLE"][0]; got != "Breathe (In the Air)" {
		t.Fatalf("matching by title failed, got %q", got)
	}
	if p.Tracks[0].Method != MatchByTitle {
		t.Errorf("expected match method %q, got %q", MatchByTitle, p.Tracks[0].Method)
	}
}

func TestMatchingDoesNotReuseATrack(t *testing.T) {
	album := library.Album{Tracks: []library.Track{
		{RelPath: "a/1.flac", File: "1.flac", Ext: "flac", TrackNo: 1},
		{RelPath: "a/1bis.flac", File: "1bis.flac", Ext: "flac", TrackNo: 1},
	}}
	p, err := Prepare(album, testRelease("One", "Two"), pattern, i18n.EN)
	if err != nil {
		t.Fatal(err)
	}
	if p.Tracks[0].Destination == p.Tracks[1].Destination {
		t.Fatal("two files aimed at the same track")
	}
}

func TestPatternWithDiscAndFormat(t *testing.T) {
	album := library.Album{Tracks: []library.Track{
		{RelPath: "a/01.flac", File: "01.flac", Ext: "flac", TrackNo: 1, DiscNo: 2},
	}}
	release := testRelease("One")
	release.Tracks[0].Disc = 2
	release.Media = []musicbrainz.Medium{{Position: 1, Format: "CD"}, {Position: 2, Format: "CD"}}

	p, err := Prepare(album, release, Patterns{Multi: "{artist}/{album} [{format}]/CD{disc:0}/{track} - {title}"}, i18n.EN)
	if err != nil {
		t.Fatal(err)
	}
	want := "Pink Floyd/The Dark Side of the Moon [CD]/CD2/01 - One.flac"
	if p.Tracks[0].Destination != want {
		t.Fatalf("got %q, want %q", p.Tracks[0].Destination, want)
	}
}

func TestMultiDiscWithoutDiscField(t *testing.T) {
	album := library.Album{Tracks: []library.Track{
		{RelPath: "a/1.flac", File: "1.flac", Ext: "flac", TrackNo: 1, DiscNo: 1},
		{RelPath: "b/1.flac", File: "1.flac", Ext: "flac", TrackNo: 1, DiscNo: 2},
	}}
	release := testRelease("One", "One")
	release.Tracks[1].Disc, release.Tracks[1].Position = 2, 1
	release.Media = []musicbrainz.Medium{{Position: 1}, {Position: 2}}

	p, err := Prepare(album, release, pattern, i18n.EN)
	if err != nil {
		t.Fatal(err)
	}
	if p.Tracks[0].Destination == p.Tracks[1].Destination {
		t.Fatalf("collision between discs: %s", p.Tracks[0].Destination)
	}
}

func TestPatternChosenByDiscCount(t *testing.T) {
	patterns := Patterns{
		Simple: "{artist}/{album} ({year})/{track} - {title}",
		Multi:  "{artist}/{album} ({year})/CD{disc:0}/{track} - {title}",
	}

	oneDisc := library.Album{Tracks: []library.Track{
		{RelPath: "a/01.flac", File: "01.flac", Ext: "flac", TrackNo: 1, DiscNo: 1},
	}}
	p, err := Prepare(oneDisc, testRelease("Speak to Me"), patterns, i18n.EN)
	if err != nil {
		t.Fatal(err)
	}
	want := "Pink Floyd/The Dark Side of the Moon (1973)/01 - Speak to Me.flac"
	if p.Tracks[0].Destination != want {
		t.Errorf("single disc: got %q, want %q", p.Tracks[0].Destination, want)
	}

	twoDiscs := library.Album{Tracks: []library.Track{
		{RelPath: "a/01.flac", File: "01.flac", Ext: "flac", TrackNo: 1, DiscNo: 2},
	}}
	release := testRelease("Speak to Me")
	release.Tracks[0].Disc = 2
	release.Media = []musicbrainz.Medium{{Position: 1}, {Position: 2}}

	p, err = Prepare(twoDiscs, release, patterns, i18n.EN)
	if err != nil {
		t.Fatal(err)
	}
	want = "Pink Floyd/The Dark Side of the Moon (1973)/CD2/01 - Speak to Me.flac"
	if p.Tracks[0].Destination != want {
		t.Errorf("box set: got %q, want %q", p.Tracks[0].Destination, want)
	}
}

func TestEmptyMultiPatternFallsBackToSimple(t *testing.T) {
	patterns := Patterns{Simple: "{artist}/{album}/{disc}-{track} - {title}"}
	album := library.Album{Tracks: []library.Track{
		{RelPath: "a/01.flac", File: "01.flac", Ext: "flac", TrackNo: 1, DiscNo: 2},
	}}
	release := testRelease("Speak to Me")
	release.Tracks[0].Disc = 2
	release.Media = []musicbrainz.Medium{{Position: 1}, {Position: 2}}

	p, err := Prepare(album, release, patterns, i18n.EN)
	if err != nil {
		t.Fatal(err)
	}
	if p.Tracks[0].Destination != "Pink Floyd/The Dark Side of the Moon/02-01 - Speak to Me.flac" {
		t.Fatalf("got %q", p.Tracks[0].Destination)
	}
}

func TestEmptyFieldCleansLeftovers(t *testing.T) {
	release := testRelease("Speak to Me")
	release.Date = ""
	release.Format = ""
	release.Media = []musicbrainz.Medium{{Position: 1}}

	album := library.Album{Tracks: []library.Track{
		{RelPath: "a/01.flac", File: "01.flac", Ext: "flac", TrackNo: 1},
	}}
	p, err := Prepare(album, release, Patterns{Simple: "{artist}/{album} ({year}) [{format}]/{track} - {title}"}, i18n.EN)
	if err != nil {
		t.Fatal(err)
	}
	want := "Pink Floyd/The Dark Side of the Moon/01 - Speak to Me.flac"
	if p.Tracks[0].Destination != want {
		t.Fatalf("got %q, want %q", p.Tracks[0].Destination, want)
	}
}

func TestEmptyFolderDisappears(t *testing.T) {
	release := testRelease("Speak to Me")
	release.Format = ""
	release.Media = []musicbrainz.Medium{{Position: 1}}

	album := library.Album{Tracks: []library.Track{
		{RelPath: "a/01.flac", File: "01.flac", Ext: "flac", TrackNo: 1},
	}}
	p, err := Prepare(album, release, Patterns{Simple: "{artist}/{format}/{album}/{track} - {title}"}, i18n.EN)
	if err != nil {
		t.Fatal(err)
	}
	want := "Pink Floyd/The Dark Side of the Moon/01 - Speak to Me.flac"
	if p.Tracks[0].Destination != want {
		t.Fatalf("got %q, want %q", p.Tracks[0].Destination, want)
	}
}

func TestTranslatedFieldsGiveSamePath(t *testing.T) {
	album := library.Album{Tracks: []library.Track{
		{RelPath: "a/01.flac", File: "01.flac", Ext: "flac", TrackNo: 1, DiscNo: 1},
	}}
	release := testRelease("Speak to Me")

	field := func(l i18n.Lang, name string) string { return "{" + DisplayName(Field{Name: name}, l) + "}" }
	build := func(l i18n.Lang) string {
		return field(l, "artist") + "/" + field(l, "album") + " (" + field(l, "year") + ")/" +
			field(l, "track") + " - " + field(l, "title")
	}

	for _, l := range i18n.Langs {
		got, err := Prepare(album, release, Patterns{Simple: build(l)}, i18n.EN)
		if err != nil {
			t.Fatalf("%s spelling refused: %v", l, err)
		}
		want, err := Prepare(album, release, Patterns{Simple: build(i18n.EN)}, i18n.EN)
		if err != nil {
			t.Fatal(err)
		}
		if got.Tracks[0].Destination != want.Tracks[0].Destination {
			t.Fatalf("%s spelling diverges: %q vs %q", l,
				got.Tracks[0].Destination, want.Tracks[0].Destination)
		}
	}
}

func TestPaddingWorksInBothSpellings(t *testing.T) {
	album := library.Album{Tracks: []library.Track{
		{RelPath: "a/01.flac", File: "01.flac", Ext: "flac", TrackNo: 1, DiscNo: 1},
	}}
	for _, l := range i18n.Langs {
		pattern := "{" + DisplayName(Field{Name: "artist"}, l) + "}/{" +
			DisplayName(Field{Name: "track"}, l) + ":000} - {" +
			DisplayName(Field{Name: "title"}, l) + "}"
		p, err := Prepare(album, testRelease("Speak to Me"), Patterns{Simple: pattern}, i18n.EN)
		if err != nil {
			t.Fatalf("%s : %v", pattern, err)
		}
		if p.Tracks[0].Destination != "Pink Floyd/001 - Speak to Me.flac" {
			t.Errorf("%s gives %q", pattern, p.Tracks[0].Destination)
		}
	}
}

func TestEveryFieldIsTranslatedInEveryLanguage(t *testing.T) {
	for _, f := range Fields {
		for _, l := range i18n.Langs {
			spelling := DisplayName(f, l)
			if spelling == "" || spelling == "field."+f.Name {
				t.Errorf("field %q has no %s spelling", f.Name, l)
				continue
			}
			got, ok := Resolve(spelling)
			if !ok || got.Name != f.Name {
				t.Errorf("{%s} no longer resolves to %q", spelling, f.Name)
			}
		}
	}
}
