package lyrics

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"gordi/internal/build"
)

const baseURL = "https://lrclib.net/api"

type Track struct {
	Artist string
	Title  string
	Album  string
	Length time.Duration
}

type Result struct {
	Plain        string `json:"plain"`
	Synced       string `json:"synced"`
	Instrumental bool   `json:"instrumental"`
	Refused      string `json:"refused"`
	WithoutAlbum bool   `json:"without_album"`
}

func (r Result) Empty() bool {
	return r.Plain == "" && r.Synced == "" && !r.Instrumental
}

type Client struct {
	http    *http.Client
	base    string
	agent   string
	limiter *limiter
}

func New(contact string) *Client {
	return &Client{
		http:    &http.Client{Timeout: 15 * time.Second},
		base:    baseURL,
		agent:   fmt.Sprintf("Gordi/%s ( %s )", build.Version, contact),
		limiter: &limiter{every: 300 * time.Millisecond},
	}
}

func (c *Client) Fetch(ctx context.Context, t Track) (Result, error) {
	seconds := int(t.Length.Round(time.Second).Seconds())
	if t.Artist == "" || t.Title == "" || seconds <= 0 {
		return Result{}, nil
	}

	res, found, err := c.get(ctx, t, seconds, true)
	if err != nil {
		return Result{}, err
	}
	if !found && t.Album != "" {
		if res, found, err = c.get(ctx, t, seconds, false); err != nil {
			return Result{}, err
		}
		res.WithoutAlbum = found
	}
	if !found {
		return Result{}, nil
	}

	if res.Synced != "" {
		if ok, why := Fits(res.Synced, t.Length); !ok {
			slog.Debug("synced lyrics dropped", "track", t.Title, "reason", why)
			res.Synced, res.Refused = "", why
		}
	}
	return res, nil
}

const maxAttempts = 4

func (c *Client) get(ctx context.Context, t Track, seconds int, withAlbum bool) (Result, bool, error) {
	q := url.Values{
		"artist_name": {t.Artist},
		"track_name":  {t.Title},
		"duration":    {strconv.Itoa(seconds)},
	}
	if withAlbum {
		q.Set("album_name", t.Album)
	}
	address := c.base + "/get?" + q.Encode()

	for attempt := 1; ; attempt++ {
		res, found, retry, err := c.attempt(ctx, address)
		if err == nil || !retry || attempt >= maxAttempts {
			return res, found, err
		}
		if err := sleep(ctx, time.Duration(attempt)*time.Second); err != nil {
			return Result{}, false, err
		}
		slog.Debug("lrclib busy, retrying", "attempt", attempt+1)
	}
}

func (c *Client) attempt(ctx context.Context, address string) (Result, bool, bool, error) {
	if err := c.limiter.wait(ctx); err != nil {
		return Result{}, false, false, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return Result{}, false, false, err
	}
	req.Header.Set("User-Agent", c.agent)

	res, err := c.http.Do(req)
	if err != nil {
		return Result{}, false, true, fmt.Errorf("lrclib unreachable: %w", err)
	}
	defer res.Body.Close()

	switch {
	case res.StatusCode == http.StatusOK:
		var body struct {
			Instrumental bool   `json:"instrumental"`
			Plain        string `json:"plainLyrics"`
			Synced       string `json:"syncedLyrics"`
		}
		if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
			return Result{}, false, false, err
		}
		return Result{
			Plain:        body.Plain,
			Synced:       body.Synced,
			Instrumental: body.Instrumental,
		}, true, false, nil
	case res.StatusCode == http.StatusNotFound:
		return Result{}, false, false, nil
	case res.StatusCode >= 500:
		return Result{}, false, true, fmt.Errorf("lrclib answered %s", res.Status)
	default:
		body, _ := io.ReadAll(io.LimitReader(res.Body, 300))
		return Result{}, false, false, fmt.Errorf("lrclib answered %s: %s", res.Status, body)
	}
}

func sleep(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

type limiter struct {
	mu    sync.Mutex
	last  time.Time
	every time.Duration
}

func (l *limiter) wait(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if wait := time.Until(l.last.Add(l.every)); wait > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
	l.last = time.Now()
	return nil
}
