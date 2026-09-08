package musicbrainz

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gordi/internal/build"
)

func TestGetRetriesAfter503(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			http.Error(w, `{"error": "busy"}`, http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(`{"releases":[{"id":"abc","title":"The Dark Side of the Moon"}]}`))
	}))
	defer srv.Close()

	c := testClient(srv.URL)
	releases, err := c.Search(context.Background(), "Pink Floyd", "The Dark Side of the Moon", Filters{}, 3)
	if err != nil {
		t.Fatalf("the client should have got through after two 503s: %v", err)
	}
	if len(releases) != 1 || releases[0].Title != "The Dark Side of the Moon" {
		t.Fatalf("answer badly parsed: %+v", releases)
	}
	if n := calls.Load(); n != 3 {
		t.Fatalf("want 3 calls, got %d", n)
	}
}

func TestGetDoesNotRetry404(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "Not Found", http.StatusNotFound)
	}))
	defer srv.Close()

	if _, err := testClient(srv.URL).Release(context.Background(), "abc"); err == nil {
		t.Fatal("a 404 must surface as an error")
	}
	if n := calls.Load(); n != 1 {
		t.Fatalf("want exactly 1 call, got %d", n)
	}
}

func TestUserAgentCarriesContact(t *testing.T) {
	received := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header.Get("User-Agent")
		w.Write([]byte(`{"releases":[]}`))
	}))
	defer srv.Close()

	if _, err := testClient(srv.URL).Search(context.Background(), "", "The Dark Side of the Moon", Filters{}, 1); err != nil {
		t.Fatal(err)
	}
	// The version comes from the build, so the test pins the shape MusicBrainz
	// asks for, not the number.
	if ua := <-received; ua != "Gordi/"+build.Version+" ( contact@example.fr )" {
		t.Fatalf("unexpected user-agent: %q", ua)
	}
}

func testClient(base string) *Client {
	c := New("contact@example.fr")
	c.base = base
	c.limiter = &limiter{every: time.Millisecond}
	return c
}

func TestFiltersInQuery(t *testing.T) {
	received := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.URL.Query().Get("query")
		w.Write([]byte(`{"releases":[]}`))
	}))
	defer srv.Close()

	f := Filters{Country: "fr", Format: "vinyl", Status: "official", YearMin: 2000, YearMax: 2005}
	if _, err := testClient(srv.URL).Search(context.Background(), "Pink Floyd", "The Dark Side of the Moon", f, 5); err != nil {
		t.Fatal(err)
	}

	query := <-received
	for _, want := range []string{
		`artist:"Pink Floyd"`,
		`release:"The Dark Side of the Moon"`,
		"country:FR",
		"format:*Vinyl*",
		"status:Official",
		"date:[2000 TO 2005]",
	} {
		if !strings.Contains(query, want) {
			t.Errorf("the query does not contain %q\nquery: %s", want, query)
		}
	}
}

func TestUnknownFilterIgnored(t *testing.T) {
	received := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.URL.Query().Get("query")
		w.Write([]byte(`{"releases":[]}`))
	}))
	defer srv.Close()

	f := Filters{Country: "anything at all", Format: "AND artist:*", Status: "bogus"}
	if _, err := testClient(srv.URL).Search(context.Background(), "", "The Dark Side of the Moon", f, 5); err != nil {
		t.Fatal(err)
	}

	if query := <-received; query != `release:"The Dark Side of the Moon"` {
		t.Fatalf("dubious filters copied into the query: %s", query)
	}
}

func TestGenresFollowTheVotes(t *testing.T) {
	got := topGenres([]genreJSON{
		{Name: "cool jazz", Count: 7},
		{Name: "bebop", Count: 3},
		{Name: "jazz", Count: 28},
		{Name: "modal jazz", Count: 4},
		{Name: "post-bop", Count: 2},
	})
	want := []string{"jazz", "cool jazz", "modal jazz", "bebop"}
	if len(got) != len(want) {
		t.Fatalf("got %v, wanted %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, wanted %v", got, want)
		}
	}
}

func TestCreditsReadRelations(t *testing.T) {
	var rels []relationJSON
	if err := json.Unmarshal([]byte(`[
		{"type":"producer","target-type":"artist","artist":{"name":"Teo Macero"}},
		{"type":"mix","target-type":"artist","artist":{"name":"Larry Keyes"}},
		{"type":"instrument","target-type":"artist","attributes":["piano"],"artist":{"name":"Bill Evans"}},
		{"type":"performance","target-type":"work","work":{"id":"w-1","title":"So What","iswcs":["T-073.092.797-5"],
			"relations":[{"type":"composer","target-type":"artist","artist":{"name":"Miles Davis"}}]}}
	]`), &rels); err != nil {
		t.Fatal(err)
	}

	c := creditsFrom(rels)
	for _, tc := range []struct{ got, want string }{
		{strings.Join(c.Producers, ","), "Teo Macero"},
		{strings.Join(c.Mixers, ","), "Larry Keyes"},
		{strings.Join(c.Performers, ","), "Bill Evans (piano)"},
		{strings.Join(c.Composers, ","), "Miles Davis"},
		{c.Work, "So What"},
		{c.WorkID, "w-1"},
		{c.ISWC, "T-073.092.797-5"},
	} {
		if tc.got != tc.want {
			t.Errorf("got %q, wanted %q", tc.got, tc.want)
		}
	}
}
