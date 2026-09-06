package app

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"gordi/internal/library"
	"gordi/internal/musicbrainz"
	"gordi/internal/store"
)

func TestGroupMakesASingleCall(t *testing.T) {
	var g group
	var calls int
	var mu sync.Mutex

	start := make(chan struct{})
	var wait sync.WaitGroup
	for i := 0; i < 5; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			v, err := g.Do("same-key", func() (any, error) {
				mu.Lock()
				calls++
				mu.Unlock()
				time.Sleep(50 * time.Millisecond)
				return "result", nil
			})
			if err != nil || v.(string) != "result" {
				t.Errorf("unexpected result: %v %v", v, err)
			}
		}()
	}
	close(start)
	wait.Wait()

	if calls != 1 {
		t.Fatalf("expected 1 network call, got %d", calls)
	}
}

func TestGroupSeparatesKeys(t *testing.T) {
	var g group
	var calls int
	var mu sync.Mutex

	for _, key := range []string{"a", "b", "a"} {
		if _, err := g.Do(key, func() (any, error) {
			mu.Lock()
			calls++
			mu.Unlock()
			return nil, nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 3 {
		t.Fatalf("sequential calls must not be grouped, got %d", calls)
	}
}

func TestObviousReleaseFollowsTagsThenTrackCount(t *testing.T) {
	tagged := musicbrainz.Release{ID: "tagged", FromTags: true, TrackCount: 3}
	matching := musicbrainz.Release{ID: "matching", TrackCount: 9}
	first := musicbrainz.Release{ID: "first", TrackCount: 5}

	cases := map[string]struct {
		releases []musicbrainz.Release
		want     string
	}{
		"the tags win":                    {[]musicbrainz.Release{first, matching, tagged}, "tagged"},
		"then the right number of tracks": {[]musicbrainz.Release{first, matching}, "matching"},
		"then the best ranked":            {[]musicbrainz.Release{first}, "first"},
		"nothing found":                   {nil, ""},
	}

	for name, c := range cases {
		if got := obviousRelease(Suggestions{Releases: c.releases}, 9); got != c.want {
			t.Errorf("%s: expected %q, got %q", name, c.want, got)
		}
	}
}

func TestReadyMarkerNeedsTheReleaseToo(t *testing.T) {
	a := testApp(t)
	if _, err := a.Store.Sync([]library.Album{{
		RelDir: "All Them Witches - Dying Surfer",
		Title:  "Dying Surfer",
		Tracks: []library.Track{{RelPath: "Dying Surfer/01.flac", File: "01.flac"}},
	}}); err != nil {
		t.Fatal(err)
	}

	queue, err := a.Store.List(store.StatusPending)
	if err != nil || len(queue) != 1 {
		t.Fatalf("expected one album in the queue: %v", err)
	}
	album := &queue[0]
	marker := readyKeyPrefix + strconv.FormatInt(album.ID, 10)

	a.writeCache(candidatesKey(album, musicbrainz.Filters{}), Suggestions{
		Releases: []musicbrainz.Release{{ID: "abc", TrackCount: 1}},
	})

	a.markReady(album)
	if a.Store.CacheFresh(marker) {
		t.Fatal("the search alone does not open an album without waiting")
	}

	a.writeCache("release:abc", musicbrainz.ReleaseDetail{})
	a.markReady(album)
	if !a.Store.CacheFresh(marker) {
		t.Fatal("search and release both cached, the album is ready")
	}
}
