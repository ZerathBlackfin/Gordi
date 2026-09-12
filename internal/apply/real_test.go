package apply

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gordi/internal/i18n"
	"gordi/internal/library"
	"gordi/internal/musicbrainz"

	"go.senan.xyz/taglib"
)

func realFile() string { return os.Getenv("GORDI_TEST_FLAC") }

func TestFilingARealFLAC(t *testing.T) {
	src := realFile()
	if src == "" {
		t.Skip("set GORDI_TEST_FLAC to a real audio file to run this")
	}
	if _, err := os.Stat(src); err != nil {
		t.Skipf("GORDI_TEST_FLAC unreadable: %v", err)
	}

	inbox, libraryDir := t.TempDir(), t.TempDir()
	folder := filepath.Join(inbox, "junk")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "a.flac"), source, 0o644); err != nil {
		t.Fatal(err)
	}

	album := library.Album{
		RelDir: "junk",
		Tracks: []library.Track{{RelPath: "junk/a.flac", File: "a.flac", Ext: "flac"}},
	}
	release := musicbrainz.ReleaseDetail{
		Release: musicbrainz.Release{
			ID: "43ef324b", Title: "Dying Surfer Meets His Maker",
			Artist: "All Them Witches", Date: "2015-10-30",
			Label: "New West Records", Catalog: "NW6444", Barcode: "607396644421",
		},
		Tracks: []musicbrainz.Track{{
			Position: 1, Disc: 1, Title: "Call Me Star", Artist: "All Them Witches",
			RecordingID: "25b51e7c", ISRCs: []string{"US27Q1564441"},
		}},
		Media: []musicbrainz.Medium{{Position: 1, Format: "CD", TrackCount: 1}},
	}

	plan, err := Prepare(album, release, Patterns{Simple: "{artist}/{album} ({year})/{track} - {title}"}, i18n.EN)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Execute(plan, inbox, libraryDir, ModeCopy, i18n.EN)
	if err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(libraryDir, filepath.FromSlash(res.Files[0]))
	tags, err := taglib.ReadTags(dest)
	if err != nil {
		t.Fatalf("the filed file no longer reads: %v", err)
	}
	for key, want := range map[string]string{
		"TITLE": "Call Me Star", "ALBUM": "Dying Surfer Meets His Maker",
		"CATALOGNUMBER": "NW6444", "ISRC": "US27Q1564441",
		"MUSICBRAINZ_ALBUMID": "43ef324b", "TRACKNUMBER": "1",
	} {
		if len(tags[key]) == 0 || tags[key][0] != want {
			t.Errorf("tag %s = %v, want %q", key, tags[key], want)
		}
	}

	before, err := taglib.ReadProperties(src)
	if err != nil {
		t.Fatal(err)
	}
	after, err := taglib.ReadProperties(dest)
	if err != nil {
		t.Fatal(err)
	}
	if before.Length != after.Length || before.SampleRate != after.SampleRate || before.BitDepth != after.BitDepth {
		t.Errorf("audio changed: before %+v, after %+v", before, after)
	}
	if len(after.Images) != len(before.Images) {
		t.Errorf("cover lost: %d images before, %d after", len(before.Images), len(after.Images))
	}

	if _, err := os.Stat(filepath.Join(folder, "a.flac")); err != nil {
		t.Error("l'original a gone en mode copy")
	}
	t.Logf("filed as %s · length %v · %d bits · %d image(s)",
		res.Files[0], after.Length, after.BitDepth, len(after.Images))
}

func TestFilingWritesTheWordsBesideARealFLAC(t *testing.T) {
	src := realFile()
	if src == "" {
		t.Skip("set GORDI_TEST_FLAC to a real audio file to run this")
	}
	source, err := os.ReadFile(src)
	if err != nil {
		t.Skipf("GORDI_TEST_FLAC unreadable: %v", err)
	}

	inbox, libraryDir := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(inbox, "junk"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inbox, "junk", "a.flac"), source, 0o644); err != nil {
		t.Fatal(err)
	}

	const synced = "[00:12.00] the first line\n[01:40.00] the last one"
	plan, err := Prepare(testAlbum("a.flac"), testRelease("One"), pattern, i18n.EN)
	if err != nil {
		t.Fatal(err)
	}
	plan.Lyrics, plan.LyricsSync = true, true
	plan.Words = map[string]Words{"junk/a.flac": {Plain: "the first line\nthe last one", Synced: synced}}

	res, err := Execute(plan, inbox, libraryDir, ModeCopy, i18n.EN)
	if err != nil {
		t.Fatal(err)
	}
	if res.Lyrics != 1 || res.Synced != 1 {
		t.Fatalf("want one of each, got %d word(s) and %d synced", res.Lyrics, res.Synced)
	}

	dest := filepath.Join(libraryDir, filepath.FromSlash(res.Files[0]))
	tags, err := taglib.ReadTags(dest)
	if err != nil {
		t.Fatalf("the filed file no longer reads: %v", err)
	}
	if got := tags[taglib.Lyrics]; len(got) == 0 || !strings.Contains(got[0], "the first line") {
		t.Errorf("the words did not make it into the tags: %v", got)
	}

	lrc := lrcPath(dest)
	written, err := os.ReadFile(lrc)
	if err != nil {
		t.Fatalf("no .lrc beside the track: %v", err)
	}
	if string(written) != synced {
		t.Errorf("the .lrc holds %q", written)
	}
	if filepath.Ext(lrc) != ".lrc" || strings.TrimSuffix(filepath.Base(lrc), ".lrc") != strings.TrimSuffix(filepath.Base(dest), ".flac") {
		t.Errorf("%q does not sit beside %q", filepath.Base(lrc), filepath.Base(dest))
	}
}

func TestSyncTurnedDownLeavesNoLRC(t *testing.T) {
	src := realFile()
	if src == "" {
		t.Skip("set GORDI_TEST_FLAC to a real audio file to run this")
	}
	source, err := os.ReadFile(src)
	if err != nil {
		t.Skipf("GORDI_TEST_FLAC unreadable: %v", err)
	}

	inbox, libraryDir := t.TempDir(), t.TempDir()
	os.MkdirAll(filepath.Join(inbox, "junk"), 0o755)
	os.WriteFile(filepath.Join(inbox, "junk", "a.flac"), source, 0o644)

	plan, _ := Prepare(testAlbum("a.flac"), testRelease("One"), pattern, i18n.EN)
	plan.Lyrics, plan.LyricsSync = true, true
	plan.Words = map[string]Words{"junk/a.flac": {Plain: "the words"}}

	res, err := Execute(plan, inbox, libraryDir, ModeCopy, i18n.EN)
	if err != nil {
		t.Fatal(err)
	}
	if res.Synced != 0 {
		t.Fatalf("no timings means no .lrc, got %d", res.Synced)
	}
	if _, err := os.Stat(lrcPath(filepath.Join(libraryDir, filepath.FromSlash(res.Files[0])))); err == nil {
		t.Error("an .lrc was written even though the timings were turned down")
	}
}
