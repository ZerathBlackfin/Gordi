package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"gordi/internal/build"
)

const (
	latestURL  = "https://api.github.com/repos/ZerathBlackfin/Gordi/releases/latest"
	releasesUI = "https://github.com/ZerathBlackfin/Gordi/releases/latest"
)

type Release struct {
	Version string `json:"version"`
	URL     string `json:"url"`
}

type Checker struct {
	http    *http.Client
	url     string
	every   time.Duration
	current string

	mu     sync.Mutex
	asked  time.Time
	latest *Release
}

func New() *Checker {
	return &Checker{
		http:    &http.Client{Timeout: 10 * time.Second},
		url:     latestURL,
		every:   6 * time.Hour,
		current: build.Version,
	}
}

func (c *Checker) Latest() *Release {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.latest
}

func (c *Checker) Refresh(ctx context.Context) {
	c.mu.Lock()
	if _, ok := parse(c.current); !ok || (!c.asked.IsZero() && time.Since(c.asked) < c.every) {
		c.mu.Unlock()
		return
	}
	c.asked = time.Now()
	c.mu.Unlock()

	release, err := c.fetch(ctx)
	if err != nil {
		slog.Debug("update check", "err", err)
		return
	}

	c.mu.Lock()
	c.latest = release
	c.mu.Unlock()
	if release != nil {
		slog.Info("update available", "version", release.Version, "running", c.current)
	}
}

func (c *Checker) fetch(ctx context.Context) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Gordi/"+c.current)

	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github answered %s", res.Status)
	}

	var body struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&body); err != nil {
		return nil, err
	}
	if !newer(body.TagName, c.current) {
		return nil, nil
	}

	url := body.HTMLURL
	if url == "" {
		url = releasesUI
	}
	return &Release{Version: strings.TrimPrefix(body.TagName, "v"), URL: url}, nil
}

func newer(latest, current string) bool {
	l, ok := parse(latest)
	if !ok {
		return false
	}
	c, ok := parse(current)
	if !ok {
		return false
	}
	for i := range l {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

func parse(v string) ([3]int, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	v, _, _ = strings.Cut(v, "-")
	v, _, _ = strings.Cut(v, "+")

	parts := strings.Split(v, ".")
	if len(parts) > 3 {
		return [3]int{}, false
	}
	var out [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}
