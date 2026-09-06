package store

import (
	"fmt"
	"gordi/internal/library"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// The app scans while handlers read. Only has to prove there is no race.
func TestConcurrentScanAndRead(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "gordi.bolt"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	albums := fakeAlbums(60, 8)
	var wg sync.WaitGroup

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				s.Sync(albums)
			}
		}()
	}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				s.List("")
				s.KnownTracks()
				s.AlbumsToPrefetch("candidates:", ":", "cool:", 5)
				s.CachePut(fmt.Sprintf("k%d", n), []byte("x"), time.Minute)
				s.CacheGet(fmt.Sprintf("k%d", n))
			}
		}(i)
	}
	wg.Wait()

	if got, _ := s.List(""); len(got) != 60 {
		t.Fatalf("60 albums expected after concurrent scans, got %d", len(got))
	}
}

func fakeAlbums(n, tracksEach int) []library.Album {
	albums := make([]library.Album, n)
	for i := range albums {
		dir := fmt.Sprintf("Artist %04d/Album %04d", i, i)
		tracks := make([]library.Track, tracksEach)
		for j := range tracks {
			tracks[j] = library.Track{RelPath: fmt.Sprintf("%s/%02d.flac", dir, j+1), Title: "T"}
		}
		albums[i] = library.Album{RelDir: dir, Artist: "A", Title: "B", Tracks: tracks}
	}
	return albums
}
