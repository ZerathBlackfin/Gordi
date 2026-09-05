package store

import (
	"go.etcd.io/bbolt"
	"path/filepath"
	"testing"
	"time"

	"gordi/internal/library"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSyncToleratesOneMiss(t *testing.T) {
	s := openTestStore(t)
	album := library.Album{
		RelDir: "Pink Floyd - The Dark Side of the Moon",
		Artist: "Air",
		Title:  "The Dark Side of the Moon",
		Tracks: []library.Track{{RelPath: "Pink Floyd - The Dark Side of the Moon/01.mp3", File: "01.mp3"}},
	}

	if _, err := s.Sync([]library.Album{album}); err != nil {
		t.Fatal(err)
	}

	res, err := s.Sync(nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Removed != 0 {
		t.Fatalf("a single miss must remove nothing, %d removed", res.Removed)
	}
	if albums, _ := s.List(""); len(albums) != 1 {
		t.Fatalf("album lost after a single miss")
	}

	res, err = s.Sync(nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Removed != 1 {
		t.Fatalf("after two misses the album must go, %d removed", res.Removed)
	}
}

func TestReturnResetsMissCounter(t *testing.T) {
	s := openTestStore(t)
	album := library.Album{RelDir: "Pink Floyd - The Dark Side of the Moon", Title: "The Dark Side of the Moon"}

	for i := 0; i < 3; i++ {
		if _, err := s.Sync([]library.Album{album}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Sync(nil); err != nil {
			t.Fatal(err)
		}
	}

	albums, err := s.List("")
	if err != nil {
		t.Fatal(err)
	}
	if len(albums) != 1 {
		t.Fatalf("album removed although it shows up on every scan")
	}
}

func TestOpenRebuildsOldState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")

	db, err := bbolt.Open(path, 0o644, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = db.Update(func(tx *bbolt.Tx) error {
		for _, name := range [][]byte{bMeta, bAlbums, bSettings} {
			if _, err := tx.CreateBucketIfNotExists(name); err != nil {
				return err
			}
		}
		if err := tx.Bucket(bMeta).Put([]byte("version"), []byte("1")); err != nil {
			return err
		}
		if err := tx.Bucket(bAlbums).Put([]byte("gone"), []byte(`{"album":{"rel_dir":"gone"}}`)); err != nil {
			return err
		}
		return tx.Bucket(bSettings).Put([]byte("pattern"), []byte("{artist}/{album}"))
	})
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatalf("opening an old database: %v", err)
	}
	defer s.Close()

	if _, err := s.Sync([]library.Album{{RelDir: "Pink Floyd - The Dark Side of the Moon", Title: "The Dark Side of the Moon"}}); err != nil {
		t.Fatalf("the database was not rebuilt: %v", err)
	}

	albums, err := s.List("")
	if err != nil {
		t.Fatal(err)
	}
	if len(albums) != 1 || albums[0].RelDir != "Pink Floyd - The Dark Side of the Moon" {
		t.Fatalf("the stale queue must be dropped, got %d album(s)", len(albums))
	}
	if v, ok := s.Setting("pattern"); !ok || v != "{artist}/{album}" {
		t.Fatal("settings must survive a version change, they are not rebuildable")
	}
}

func TestCacheSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.db")

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CachePut("candidates:1", []byte(`{"ok":true}`), time.Hour); err != nil {
		t.Fatal(err)
	}
	s.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	data, ok := s2.CacheGet("candidates:1")
	if !ok || string(data) != `{"ok":true}` {
		t.Fatalf("entry lost after restart: %q present=%v", data, ok)
	}
}

func TestCacheExpires(t *testing.T) {
	s := openTestStore(t)
	if err := s.CachePut("old", []byte("{}"), -time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.CacheGet("old"); ok {
		t.Fatal("an expired entry was served")
	}
	if n := s.CacheSize(); n != 0 {
		t.Fatalf("an expired entry still counts, %d left", n)
	}
}

func TestCachePurgeFreesExpiredEntries(t *testing.T) {
	s := openTestStore(t)
	if err := s.CachePut("stale", []byte("{}"), -time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := s.CachePut("fresh", []byte("{}"), time.Hour); err != nil {
		t.Fatal(err)
	}

	if err := s.CachePurge(); err != nil {
		t.Fatal(err)
	}

	if _, ok := s.CacheGet("fresh"); !ok {
		t.Fatal("the sweep took a live entry")
	}
	// Reading past it is not enough: it has to leave the disk.
	if n := s.CacheSize(); n != 1 {
		t.Fatalf("1 entry expected after the sweep, %d left", n)
	}
}

func TestSyncForgetsFiledAndVanishedAlbum(t *testing.T) {
	s := openTestStore(t)
	album := library.Album{RelDir: "Pink Floyd - The Dark Side of the Moon", Title: "The Dark Side of the Moon"}

	if _, err := s.Sync([]library.Album{album}); err != nil {
		t.Fatal(err)
	}
	albums, _ := s.List("")
	if err := s.SetStatus(albums[0].ID, StatusDone); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < missesBeforeForget; i++ {
		if _, err := s.Sync(nil); err != nil {
			t.Fatal(err)
		}
	}
	if remaining, _ := s.List(""); len(remaining) != 0 {
		t.Fatalf("the filed and vanished album stayed in the database: %+v", remaining)
	}

	if _, err := s.Sync([]library.Album{album}); err != nil {
		t.Fatal(err)
	}
	returned, _ := s.List(StatusPending)
	if len(returned) != 1 {
		t.Fatal("a folder dropped back in must return to the queue")
	}
}

func TestAlbumsToPrefetchEmptyQueue(t *testing.T) {
	s := openTestStore(t)

	albums, total, remaining, err := s.AlbumsToPrefetch("mb:", ":f", 1)
	if err != nil {
		t.Fatalf("empty queue: %v", err)
	}
	if len(albums) != 0 || total != 0 || remaining != 0 {
		t.Fatalf("expected 0/0/0, got %d/%d/%d", len(albums), total, remaining)
	}
}
