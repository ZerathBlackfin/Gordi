package lyrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gordi/internal/build"
)

func testClient(base string) *Client {
	c := New("https://example.test")
	c.base = base
	c.limiter = &limiter{}
	return c
}

var getLucky = Track{
	Artist: "Daft Punk",
	Title:  "Get Lucky",
	Album:  "Random Access Memories",
	Length: 367 * time.Second,
}

func TestFetchAsksWithTheLength(t *testing.T) {
	asked := make(chan url.Values, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked <- r.URL.Query()
		w.Write([]byte(`{"plainLyrics":"Like the legend of the phoenix","duration":367}`))
	}))
	defer srv.Close()

	res, err := testClient(srv.URL).Fetch(context.Background(), getLucky)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	q := <-asked
	if q.Get("duration") != "367" {
		t.Fatalf("the length must always be part of the question, got %q", q.Get("duration"))
	}
	if q.Get("album_name") != "Random Access Memories" {
		t.Fatalf("the album name should be tried first, got %q", q.Get("album_name"))
	}
	if res.Plain == "" || res.WithoutAlbum {
		t.Fatalf("a first-try answer did not need a second one: %+v", res)
	}
}

func TestFetchDropsTheAlbumNameOnASecondTry(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			http.Error(w, `{"name":"TrackNotFound"}`, http.StatusNotFound)
			return
		}
		if r.URL.Query().Has("album_name") {
			t.Error("the second try should leave the album name out")
		}
		w.Write([]byte(`{"plainLyrics":"the words","duration":367}`))
	}))
	defer srv.Close()

	res, err := testClient(srv.URL).Fetch(context.Background(), getLucky)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Plain != "the words" || !res.WithoutAlbum {
		t.Fatalf("want a second-try match carrying the words, got %+v", res)
	}
	if n := calls.Load(); n != 2 {
		t.Fatalf("want 2 calls, got %d", n)
	}
}

func TestFetchComesBackEmptyRatherThanGuessing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"name":"TrackNotFound"}`, http.StatusNotFound)
	}))
	defer srv.Close()

	res, err := testClient(srv.URL).Fetch(context.Background(), getLucky)
	if err != nil {
		t.Fatalf("nothing found is not a failure: %v", err)
	}
	if !res.Empty() {
		t.Fatalf("want nothing at all, got %+v", res)
	}
}

func TestFetchRetriesWhileLRCLIBIsBusy(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			http.Error(w, `{"name":"ServerOverloaded"}`, http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(`{"plainLyrics":"the words","duration":367}`))
	}))
	defer srv.Close()

	c := testClient(srv.URL)
	c.limiter = &limiter{every: time.Millisecond}
	res, err := c.Fetch(context.Background(), getLucky)
	if err != nil {
		t.Fatalf("a busy server is worth another try: %v", err)
	}
	if res.Plain != "the words" {
		t.Fatalf("want the words, got %+v", res)
	}
}

func TestFetchKeepsTheWordsAndDropsCrookedTimings(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"plainLyrics":"I, I will be king",` +
			`"syncedLyrics":"[00:17.30] I, I will be king\n[05:50.12] just for one day",` +
			`"duration":303}`))
	}))
	defer srv.Close()

	res, err := testClient(srv.URL).Fetch(context.Background(), Track{
		Artist: "David Bowie", Title: "Heroes", Album: "\"Heroes\"", Length: 303 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Synced != "" {
		t.Fatal("timings running past the end of the file must not survive")
	}
	if res.Refused != Overrun {
		t.Fatalf("want %q, got %q", Overrun, res.Refused)
	}
	if res.Plain == "" {
		t.Fatal("the words themselves are still good")
	}
}

func TestFetchStaysHomeWithoutALength(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("no length means no question worth asking")
	}))
	defer srv.Close()

	res, err := testClient(srv.URL).Fetch(context.Background(), Track{Artist: "a", Title: "b"})
	if err != nil || !res.Empty() {
		t.Fatalf("want a quiet empty answer, got %+v (%v)", res, err)
	}
}

func TestUserAgentCarriesContact(t *testing.T) {
	received := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header.Get("User-Agent")
		w.Write([]byte(`{"instrumental":true,"duration":367}`))
	}))
	defer srv.Close()

	if _, err := testClient(srv.URL).Fetch(context.Background(), getLucky); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	agent := <-received
	if !strings.Contains(agent, build.Version) || !strings.Contains(agent, "https://example.test") {
		t.Fatalf("LRCLIB should know who is calling, got %q", agent)
	}
}
