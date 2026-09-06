package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"gordi/internal/musicbrainz"
	"gordi/internal/store"
)

const cacheTTL = 7 * 24 * time.Hour

const callTimeout = 90 * time.Second

type Source string

const (
	SourceBarcode Source = "barcode"
	SourceSearch  Source = "search"
)

type Suggestions struct {
	Source   Source                `json:"source"`
	Cache    bool                  `json:"cache"`
	Selected string                `json:"selected"`
	Releases []musicbrainz.Release `json:"releases"`
}

func (a *App) Candidates(ctx context.Context, albumID int64, f musicbrainz.Filters, force bool) (Suggestions, error) {
	album, err := a.Store.Get(albumID)
	if err != nil {
		return Suggestions{}, err
	}
	if album == nil {
		return Suggestions{}, fmt.Errorf("unknown album")
	}

	key := candidatesKey(album, f)
	if !force {
		var p Suggestions
		if a.readCache(key, &p) {
			p.Cache = true
			p.Selected = obviousRelease(p, album.TrackCount)
			return p, nil
		}
	}

	p, err := a.search(ctx, album, f)
	if err != nil {
		return Suggestions{}, err
	}
	a.writeCache(key, p)
	p.Selected = obviousRelease(p, album.TrackCount)
	return p, nil
}

func (a *App) search(ctx context.Context, album *store.Album, f musicbrainz.Filters) (Suggestions, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), callTimeout)
	defer cancel()

	run := func() (Suggestions, error) {
		source := SourceSearch
		var releases []musicbrainz.Release

		if f.IsZero() {
			if code := strings.TrimSpace(album.Barcode); code != "" {
				found, err := a.MB.SearchBarcode(ctx, code)
				if err == nil && len(found) > 1 {
					releases, source = found, SourceBarcode
				}
			}
		}

		if releases == nil {
			title := album.Title
			if title == "" {
				title = album.RelDir
			}
			var err error
			releases, err = a.MB.Search(ctx, album.Artist, title, f, 25)
			if err != nil {
				return Suggestions{}, err
			}
		}

		rankReleases(releases, album.TrackCount)
		if f.IsZero() {
			releases = a.withTaggedRelease(ctx, releases, album.MBAlbumID)
		}
		return Suggestions{Source: source, Releases: releases}, nil
	}

	v, err := a.group.Do(candidatesKey(album, f), func() (any, error) {
		a.beginCall()
		defer a.endCall()
		return run()
	})
	if err != nil {
		return Suggestions{}, err
	}
	return v.(Suggestions), nil
}

func (a *App) Release(ctx context.Context, mbid string) (*musicbrainz.ReleaseDetail, error) {
	return a.release(ctx, mbid)
}

func (a *App) release(ctx context.Context, mbid string) (*musicbrainz.ReleaseDetail, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), callTimeout)
	defer cancel()

	key := "release:" + mbid

	var detail musicbrainz.ReleaseDetail
	if a.readCache(key, &detail) {
		return &detail, nil
	}

	v, err := a.group.Do(key, func() (any, error) {
		a.beginCall()
		defer a.endCall()
		return a.MB.Release(ctx, mbid)
	})
	if err != nil {
		return nil, err
	}
	d := v.(*musicbrainz.ReleaseDetail)
	a.writeCache(key, d)
	return d, nil
}

// AlbumsToPrefetch rebuilds this key. Both ends have to agree.
const candidatesKeyPrefix = "candidates:"

const cooldownKeyPrefix = "prefetch-cooldown:"

// Written once an album can be opened without touching MusicBrainz again.
const readyKeyPrefix = "ready:"

func (a *App) withTaggedRelease(ctx context.Context, releases []musicbrainz.Release, mbid string) []musicbrainz.Release {
	mbid = strings.TrimSpace(mbid)
	if mbid == "" {
		return releases
	}

	for i := range releases {
		if releases[i].ID != mbid {
			continue
		}
		releases[i].FromTags = true
		front := releases[i]
		return append([]musicbrainz.Release{front}, append(releases[:i:i], releases[i+1:]...)...)
	}

	detail, err := a.release(ctx, mbid)
	if err != nil {
		slog.Debug("unusable identifier from tags", "mbid", mbid, "err", err)
		return releases
	}
	front := detail.Release
	front.FromTags = true
	return append([]musicbrainz.Release{front}, releases...)
}

func candidatesKey(album *store.Album, f musicbrainz.Filters) string {
	return candidatesKeyPrefix + fmt.Sprintf("%d%s", album.ID, filtersKey(f))
}

func filtersKey(f musicbrainz.Filters) string {
	return fmt.Sprintf(":%s|%s|%s|%d|%d", f.Country, f.Format, f.Status, f.YearMin, f.YearMax)
}

func (a *App) readCache(key string, dest any) bool {
	data, ok := a.Store.CacheGet(key)
	if !ok {
		return false
	}
	return json.Unmarshal(data, dest) == nil
}

func (a *App) writeCache(key string, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	if err := a.Store.CachePut(key, data, cacheTTL); err != nil {
		slog.Debug("cache not written", "key", key, "err", err)
	}
}

func (a *App) beginCall() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.mbCalls == 0 {
		a.mbSince = time.Now()
	}
	a.mbCalls++
}

func (a *App) endCall() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.mbCalls--
}

type group struct {
	mu       sync.Mutex
	inFlight map[string]*call
}

type call struct {
	done  chan struct{}
	value any
	err   error
}

func (g *group) Do(key string, fn func() (any, error)) (any, error) {
	g.mu.Lock()
	if g.inFlight == nil {
		g.inFlight = map[string]*call{}
	}
	if a, ok := g.inFlight[key]; ok {
		g.mu.Unlock()
		<-a.done
		return a.value, a.err
	}
	a := &call{done: make(chan struct{})}
	g.inFlight[key] = a
	g.mu.Unlock()

	a.value, a.err = fn()

	g.mu.Lock()
	delete(g.inFlight, key)
	g.mu.Unlock()
	close(a.done)

	return a.value, a.err
}

func (a *App) Prefetch(ctx context.Context) {
	if a.Cfg.PrefetchEvery <= 0 {
		slog.Info("MusicBrainz prefetch disabled")
		return
	}
	const idlePause = 2 * time.Minute

	wait := a.PrefetchEvery()
	for {
		if wait <= 0 {
			wait = idlePause // prefetch off: check again later
		}
		select {
		case <-ctx.Done():
			return
		case <-a.wake:
		case <-time.After(wait):
		}

		if a.PrefetchEvery() <= 0 {
			wait = idlePause
			continue
		}

		left, err := a.prefetchOne(ctx)
		if err != nil && ctx.Err() == nil {
			slog.Debug("prefetch", "err", err)
		}
		if left {
			wait = a.PrefetchEvery()
		} else {
			wait = idlePause
		}
	}
}

func (a *App) prefetchOne(ctx context.Context) (bool, error) {
	albums, _, remaining, err := a.toIndex(1)
	if err != nil {
		return true, err
	}

	a.mu.Lock()
	busy := a.mbCalls > 0
	a.mu.Unlock()

	if busy || len(albums) == 0 {
		return remaining > 0, nil
	}
	next := &albums[0]

	a.setIndexing(next.ID)
	defer a.setIndexing(0)

	p, err := a.readyFor(ctx, next)
	if err != nil {
		if ctx.Err() == nil {
			a.Store.CachePut(cooldownKeyPrefix+strconv.FormatInt(next.ID, 10), []byte("1"), 10*time.Minute)
		}
		return true, err
	}

	a.markReady(next)
	slog.Debug("prefetched", "album", next.RelDir, "suggestions", len(p.Releases), "source", p.Source)
	return true, nil
}

// Everything the interface asks for when an album is opened: the search, then
// the release it will land on.
func (a *App) readyFor(ctx context.Context, album *store.Album) (Suggestions, error) {
	p, err := a.Candidates(ctx, album.ID, musicbrainz.Filters{}, false)
	if err != nil {
		return Suggestions{}, err
	}
	if p.Selected != "" {
		if _, err := a.release(ctx, p.Selected); err != nil {
			return Suggestions{}, err
		}
	}
	return p, nil
}

// The album opens without a word to MusicBrainz: the search is cached, and so
// is the release the interface will settle on.
func (a *App) markReady(album *store.Album) {
	var p Suggestions
	if !a.readCache(candidatesKey(album, musicbrainz.Filters{}), &p) {
		return
	}
	mbid := obviousRelease(p, album.TrackCount)
	if mbid != "" && !a.Store.CacheFresh("release:"+mbid) {
		return
	}
	a.Store.CachePut(readyKeyPrefix+strconv.FormatInt(album.ID, 10), []byte("1"), cacheTTL)
}

// The release the interface settles on, decided here so both ends agree.
func obviousRelease(p Suggestions, trackCount int) string {
	for _, r := range p.Releases {
		if r.FromTags {
			return r.ID
		}
	}
	for _, r := range p.Releases {
		if r.TrackCount == trackCount {
			return r.ID
		}
	}
	if len(p.Releases) > 0 {
		return p.Releases[0].ID
	}
	return ""
}

func (a *App) toIndex(limit int) (albums []store.Album, total, remaining int, err error) {
	return a.Store.AlbumsToPrefetch(readyKeyPrefix, "", cooldownKeyPrefix, limit)
}

// Nudge is for when the queue changes under the loop's feet, so it does not
// sit on a stale idle pause.
func (a *App) Nudge() {
	select {
	case a.wake <- struct{}{}:
	default:
	}
}

func (a *App) setIndexing(id int64) {
	a.mu.Lock()
	a.indexing = id
	a.mu.Unlock()
}
