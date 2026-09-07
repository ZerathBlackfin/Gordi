package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewerCompares(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"v1.2.0", "v1.1.9", true},
		{"v1.10.0", "v1.9.0", true},
		{"v2.0.0", "v1.99.99", true},
		{"v1.2.3", "v1.2.3", false},
		{"v1.2.3", "v1.3.0", false},
		{"1.2.3", "v1.2.2", true},
		{"v1.2.3", "dev", false},
		{"main", "v1.2.3", false},
	}
	for _, c := range cases {
		if got := newer(c.latest, c.current); got != c.want {
			t.Errorf("newer(%q, %q) = %v, want %v", c.latest, c.current, got, c.want)
		}
	}
}

func TestRefreshKeepsTheNewerRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"tag_name":"v2.0.0","html_url":"https://example.test/releases/v2.0.0"}`))
	}))
	defer srv.Close()

	c := testChecker(srv.URL, "v1.0.0")
	c.Refresh(context.Background())

	latest := c.Latest()
	if latest == nil {
		t.Fatal("v2.0.0 is newer than v1.0.0, it should have been kept")
	}
	if latest.Version != "2.0.0" || latest.URL != "https://example.test/releases/v2.0.0" {
		t.Fatalf("unexpected release: %+v", latest)
	}
}

func TestRefreshIgnoresTheRunningVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"tag_name":"v1.0.0","html_url":"https://example.test/releases/v1.0.0"}`))
	}))
	defer srv.Close()

	c := testChecker(srv.URL, "v1.0.0")
	c.Refresh(context.Background())

	if latest := c.Latest(); latest != nil {
		t.Fatalf("nothing to upgrade to, got %+v", latest)
	}
}

func TestRefreshAsksOnceThenWaits(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Write([]byte(`{"tag_name":"v2.0.0"}`))
	}))
	defer srv.Close()

	c := testChecker(srv.URL, "v1.0.0")
	c.Refresh(context.Background())
	c.Refresh(context.Background())

	if n := calls.Load(); n != 1 {
		t.Fatalf("want a single call within the interval, got %d", n)
	}
}

func TestRefreshSkipsAWorkingCopy(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
	}))
	defer srv.Close()

	c := testChecker(srv.URL, "dev")
	c.Refresh(context.Background())

	if n := calls.Load(); n != 0 {
		t.Fatalf("a working copy must not ask GitHub, it did %d time(s)", n)
	}
}

func testChecker(url, current string) *Checker {
	c := New()
	c.url = url
	c.current = current
	c.every = time.Hour
	return c
}
