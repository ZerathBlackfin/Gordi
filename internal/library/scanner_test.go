package library

import (
	"gordi/internal/i18n"
	"os"
	"path/filepath"
	"testing"
)

func TestScanUnreadableRoot(t *testing.T) {
	if _, err := Scan(filepath.Join(t.TempDir(), "absent"), nil, i18n.EN); err == nil {
		t.Fatal("a missing inbox must raise an error, not zero albums")
	}
}

func TestScanGroupsByFolder(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "Pink Floyd - The Dark Side of the Moon (1973)")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"01.mp3", "02.mp3", "cover.jpg"} {
		if err := os.WriteFile(filepath.Join(folder, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	albums, err := Scan(root, nil, i18n.EN)
	if err != nil {
		t.Fatal(err)
	}
	if len(albums) != 1 {
		t.Fatalf("want 1 album, got %d", len(albums))
	}
	a := albums[0]
	if a.Artist != "Pink Floyd" || a.Title != "The Dark Side of the Moon" || a.Year != 1973 {
		t.Errorf("folder name misread: %+v", a)
	}
	if len(a.Tracks) != 2 {
		t.Errorf("want 2 tracks (the jpg does not count), got %d", len(a.Tracks))
	}
}

func TestScanReusesUnchangedFiles(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "Pink Floyd - The Dark Side of the Moon")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "01.mp3"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	first, err := Scan(root, nil, i18n.EN)
	if err != nil {
		t.Fatal(err)
	}
	track := first[0].Tracks[0]

	track.Title = "taken from cache"
	known := map[string]Track{track.RelPath: track}

	second, err := Scan(root, known, i18n.EN)
	if err != nil {
		t.Fatal(err)
	}
	if second[0].Tracks[0].Title != "taken from cache" {
		t.Fatalf("the unchanged file was read again: %q", second[0].Tracks[0].Title)
	}

	if err := os.WriteFile(filepath.Join(folder, "01.mp3"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	third, err := Scan(root, known, i18n.EN)
	if err != nil {
		t.Fatal(err)
	}
	if third[0].Tracks[0].Title == "taken from cache" {
		t.Fatal("a changed file must be read again")
	}
}

func TestScanMergesDiscFolders(t *testing.T) {
	root := t.TempDir()
	for disc, tracks := range map[string][]string{"CD1": {"01.flac", "02.flac"}, "CD 2": {"01.flac"}} {
		folder := filepath.Join(root, "Daft Punk - Alive", disc)
		if err := os.MkdirAll(folder, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, name := range tracks {
			if err := os.WriteFile(filepath.Join(folder, name), nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}

	albums, err := Scan(root, nil, i18n.EN)
	if err != nil {
		t.Fatal(err)
	}
	if len(albums) != 1 {
		t.Fatalf("want 1 album, got %d : %v", len(albums), albums)
	}
	if len(albums[0].Tracks) != 3 {
		t.Fatalf("want 3 tracks, got %d", len(albums[0].Tracks))
	}
	if albums[0].Discs != 2 {
		t.Errorf("want 2 discs, got %d", albums[0].Discs)
	}
	discs := map[int]int{}
	for _, tr := range albums[0].Tracks {
		discs[tr.DiscNo]++
	}
	if discs[1] != 2 || discs[2] != 1 {
		t.Errorf("disc numbers badly spread: %v", discs)
	}
}

func TestScanDoesNotMergeAmbiguousFolder(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "Album")
	if err := os.MkdirAll(filepath.Join(parent, "CD2"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(parent, "01.flac"), nil, 0o644)
	os.WriteFile(filepath.Join(parent, "CD2", "01.flac"), nil, 0o644)

	albums, err := Scan(root, nil, i18n.EN)
	if err != nil {
		t.Fatal(err)
	}
	if len(albums) != 2 {
		t.Fatalf("want 2 separate albums, got %d", len(albums))
	}
}

func TestScanMergesSiblingDiscFolders(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"Daft Punk - Alive CD1", "Daft Punk - Alive CD2"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, d, "01.flac"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	albums, err := Scan(root, nil, i18n.EN)
	if err != nil {
		t.Fatal(err)
	}
	if len(albums) != 1 {
		t.Fatalf("want 1 album, got %d", len(albums))
	}
	if albums[0].RelDir != "Daft Punk - Alive" {
		t.Errorf("album name should drop the disc suffix, got %q", albums[0].RelDir)
	}
	if albums[0].Discs != 2 {
		t.Errorf("want 2 discs, got %d", albums[0].Discs)
	}
}

func TestScanLeavesLoneDiscFolder(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "Alive CD1"), 0o755)
	os.WriteFile(filepath.Join(root, "Alive CD1", "01.flac"), nil, 0o644)

	albums, err := Scan(root, nil, i18n.EN)
	if err != nil {
		t.Fatal(err)
	}
	if len(albums) != 1 || albums[0].RelDir != "Alive CD1" {
		t.Fatalf("lone folder changed: %+v", albums)
	}
}
