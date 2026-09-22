package apply

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gordi/internal/i18n"

	"go.senan.xyz/taglib"
)

func TestChangedTagsKeepsOnlyWhatMoved(t *testing.T) {
	before := map[string][]string{
		"TITLE":   {"call me star"},
		"ARTIST":  {"All Them Witches"},
		"COMMENT": {"ripped by me"},
	}
	after := map[string][]string{
		"TITLE":               {"Call Me Star"},
		"ARTIST":              {"All Them Witches"},
		"MUSICBRAINZ_TRACKID": {"25b51e7c"},
	}
	want := map[string][]string{
		"TITLE":               {"call me star"},
		"COMMENT":             {"ripped by me"},
		"MUSICBRAINZ_TRACKID": nil,
	}
	if got := changedTags(before, after); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func fileRealFLAC(t *testing.T, mode Mode) (inbox, libraryDir string, before map[string][]string, res Result) {
	t.Helper()
	src := realFile()
	if src == "" {
		t.Skip("set GORDI_TEST_FLAC to a real audio file to run this")
	}
	source, err := os.ReadFile(src)
	if err != nil {
		t.Skipf("GORDI_TEST_FLAC unreadable: %v", err)
	}

	inbox, libraryDir = t.TempDir(), t.TempDir()
	original := filepath.Join(inbox, "junk", "a.flac")
	if err := os.MkdirAll(filepath.Dir(original), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(original, source, 0o644); err != nil {
		t.Fatal(err)
	}
	if before, err = taglib.ReadTags(original); err != nil {
		t.Fatal(err)
	}

	plan, err := Prepare(testAlbum("a.flac"), testRelease("One"), pattern, i18n.EN)
	if err != nil {
		t.Fatal(err)
	}
	plan.LyricsSync = true
	plan.Words = map[string]Words{"junk/a.flac": {Synced: "[00:12.00] a line"}}

	if res, err = Execute(plan, inbox, libraryDir, mode, i18n.EN); err != nil {
		t.Fatal(err)
	}
	if len(res.Undo.Extras) != 1 {
		t.Fatalf("the .lrc should be down to be removed, got %v", res.Undo.Extras)
	}
	return inbox, libraryDir, before, res
}

func TestUndoingAMovePutsTheFileBack(t *testing.T) {
	inbox, libraryDir, before, res := fileRealFLAC(t, ModeMove)
	original := filepath.Join(inbox, "junk", "a.flac")
	if _, err := os.Stat(original); err == nil {
		t.Fatal("move mode left the original behind")
	}

	if err := Reverse(res.Undo, inbox, libraryDir, i18n.EN); err != nil {
		t.Fatal(err)
	}

	after, err := taglib.ReadTags(original)
	if err != nil {
		t.Fatalf("the original did not come back: %v", err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Errorf("tags not restored:\nbefore %v\nafter  %v", before, after)
	}
	left, err := os.ReadDir(libraryDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Errorf("the library still holds %v", left)
	}
	record, _ := json.Marshal(res.Undo)
	t.Logf("undo record: %d bytes, %d tag(s) changed", len(record), len(res.Undo.Tracks[0].Tags))
}

func TestUndoingACopyRestoresAnOriginalDeletedSince(t *testing.T) {
	inbox, libraryDir, before, res := fileRealFLAC(t, ModeCopy)
	original := filepath.Join(inbox, "junk", "a.flac")
	if err := os.Remove(original); err != nil {
		t.Fatal(err)
	}

	if err := Reverse(res.Undo, inbox, libraryDir, i18n.EN); err != nil {
		t.Fatal(err)
	}
	after, err := taglib.ReadTags(original)
	if err != nil {
		t.Fatalf("the original did not come back: %v", err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Errorf("tags not restored:\nbefore %v\nafter  %v", before, after)
	}
}

func TestUndoLeavesAFileChangedSinceAlone(t *testing.T) {
	inbox, libraryDir, _, res := fileRealFLAC(t, ModeMove)
	filed := filepath.Join(libraryDir, filepath.FromSlash(res.Files[0]))
	if err := taglib.WriteTags(filed, map[string][]string{"COMMENT": {"edited in another app"}}, 0); err != nil {
		t.Fatal(err)
	}

	if err := Reverse(res.Undo, inbox, libraryDir, i18n.EN); err == nil {
		t.Fatal("undo went through over a file changed after filing")
	}
	if _, err := os.Stat(filed); err != nil {
		t.Error("the changed file was removed anyway")
	}
	if _, err := os.Stat(filepath.Join(inbox, "junk", "a.flac")); err == nil {
		t.Error("the original came back anyway")
	}
}

func TestUndoingAMoveStopsOnAnOccupiedInbox(t *testing.T) {
	inbox, libraryDir, _, res := fileRealFLAC(t, ModeMove)
	if err := os.MkdirAll(filepath.Join(inbox, "junk"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inbox, "junk", "a.flac"), []byte("someone else"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Reverse(res.Undo, inbox, libraryDir, i18n.EN); err == nil {
		t.Fatal("undo wrote over a file already in the inbox")
	}
	if _, err := os.Stat(filepath.Join(libraryDir, filepath.FromSlash(res.Files[0]))); err != nil {
		t.Error("the filed copy was removed anyway")
	}
}
