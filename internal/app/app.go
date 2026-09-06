package app

import (
	"context"
	"log/slog"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"gordi/internal/config"
	"gordi/internal/library"
	"gordi/internal/musicbrainz"
	"gordi/internal/store"
)

type App struct {
	Cfg   config.Config
	Store *store.Store
	MB    *musicbrainz.Client

	mu       sync.Mutex
	scanning bool
	lastScan time.Time
	last     store.SyncResult

	mbCalls int
	mbSince time.Time

	prefetchDone  int
	prefetchTotal int

	group group

	knownMu sync.Mutex
	known   map[string]library.Track
}

func New(cfg config.Config, st *store.Store) *App {
	return &App{
		Cfg:   cfg,
		Store: st,
		MB:    musicbrainz.New(cfg.MBContact),
	}
}

func (a *App) knownTracks() (map[string]library.Track, error) {
	a.knownMu.Lock()
	defer a.knownMu.Unlock()

	if a.known == nil {
		known, err := a.Store.KnownTracks()
		if err != nil {
			return nil, err
		}
		a.known = known
	}
	return a.known, nil
}

func (a *App) rememberTracks(albums []library.Album) {
	known := make(map[string]library.Track, len(a.known))
	for _, album := range albums {
		for _, t := range album.Tracks {
			known[t.RelPath] = t
		}
	}

	a.knownMu.Lock()
	a.known = known
	a.knownMu.Unlock()
}

func (a *App) Cover(albumID int64) ([]byte, string, error) {
	album, err := a.Store.Get(albumID)
	if err != nil {
		return nil, "", err
	}
	if album == nil {
		return nil, "", nil
	}
	for _, t := range album.Tracks {
		if t.Audio.Cover == "" {
			continue
		}
		image, err := library.Cover(filepath.Join(a.Cfg.Input, t.RelPath))
		if err == nil && len(image) > 0 {
			return image, t.Audio.Cover, nil
		}
	}
	return nil, "", nil
}

func rankReleases(releases []musicbrainz.Release, tracks int) {
	sort.SliceStable(releases, func(i, j int) bool {
		gi, gj := gap(releases[i].TrackCount, tracks), gap(releases[j].TrackCount, tracks)
		if gi != gj {
			return gi < gj
		}
		return releases[i].Score > releases[j].Score
	})
}

func gap(a, b int) int {
	if a > b {
		return a - b
	}
	return b - a
}

type Status struct {
	Input    string           `json:"input"`
	Output   string           `json:"output"`
	Mode     string           `json:"mode"`
	Scanning bool             `json:"scanning"`
	LastScan *time.Time       `json:"last_scan"`
	Last     store.SyncResult `json:"last_result"`
	Lang     string           `json:"lang"`
	MB       StatusMB         `json:"mb"`
}

type StatusMB struct {
	SinceSeconds  float64 `json:"since_seconds"`
	PrefetchDone  int     `json:"prefetch_done"`
	PrefetchTotal int     `json:"prefetch_total"`
}

func (a *App) Albums(status string) ([]store.Album, error) {
	albums, err := a.Store.List(status)
	if err != nil {
		return nil, err
	}
	for i := range albums {
		albums[i].Indexed = a.Store.CacheFresh(candidatesKey(&albums[i], musicbrainz.Filters{}))
	}
	return albums, nil
}

func (a *App) Status() (Status, error) {

	a.mu.Lock()
	defer a.mu.Unlock()

	s := Status{
		Input:    a.Cfg.Input,
		Output:   a.Cfg.Output,
		Mode:     string(a.Cfg.Mode),
		Scanning: a.scanning,
		Last:     a.last,
		Lang:     string(a.Lang()),
	}
	if !a.lastScan.IsZero() {
		t := a.lastScan
		s.LastScan = &t
	}
	s.MB = StatusMB{
		PrefetchDone:  a.prefetchDone,
		PrefetchTotal: a.prefetchTotal,
	}
	if a.mbCalls > 0 {
		s.MB.SinceSeconds = time.Since(a.mbSince).Seconds()
	}
	return s, nil
}

func (a *App) Rescan() (store.SyncResult, error) {
	a.mu.Lock()
	if a.scanning {
		last := a.last
		a.mu.Unlock()
		return last, nil
	}
	a.scanning = true
	a.mu.Unlock()

	known, err := a.knownTracks()
	var res store.SyncResult
	if err == nil {
		var albums []library.Album
		if albums, err = library.Scan(a.Cfg.Input, known, a.Lang()); err == nil {
			if res, err = a.Store.Sync(albums); err == nil {
				a.rememberTracks(albums)
			}
		}
	}

	a.mu.Lock()
	a.scanning = false
	a.lastScan = time.Now().UTC()
	a.last = res
	if err != nil {
	}
	a.mu.Unlock()

	return res, err
}

func (a *App) Run(ctx context.Context) {
	for {
		res, err := a.Rescan()
		if err != nil {
			slog.Error("scan", "err", err)
		} else if res.Added > 0 || res.Removed > 0 {
			slog.Info("scan", "added", res.Added, "removed", res.Removed, "seen", res.Updated)
		}

		// Expired answers are skipped on read but stay on disk.
		if err := a.Store.CachePurge(); err != nil {
			slog.Error("cache sweep", "err", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(a.ScanEvery()):
		}
	}
}
