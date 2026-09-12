package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"gordi/internal/apply"
	"gordi/internal/library"
	"gordi/internal/lyrics"
	"gordi/internal/musicbrainz"
)

const anchorCount = 2

type TrackLyrics struct {
	Source string `json:"source"`
	Title  string `json:"title"`

	Found        bool   `json:"found"`
	Instrumental bool   `json:"instrumental"`
	Synced       bool   `json:"synced"`
	WithoutAlbum bool   `json:"without_album"`
	Refused      string `json:"refused"`

	Lines   []lyrics.Line `json:"lines"`
	Anchors []int         `json:"anchors"`
}

func (a *App) Lyrics(ctx context.Context, albumID int64, releaseID string) ([]TrackLyrics, error) {
	album, release, err := a.albumAndRelease(ctx, albumID, releaseID)
	if err != nil {
		return nil, err
	}
	p, err := apply.Prepare(album.Album, *release, a.Patterns(), a.Lang())
	if err != nil {
		return nil, err
	}

	files := byPath(album.Tracks)
	out := make([]TrackLyrics, 0, len(p.Tracks))
	for _, planned := range p.Tracks {
		file, ok := files[planned.Source]
		if !ok {
			continue
		}
		track := release.Tracks[planned.Index]
		found, err := a.words(ctx, release, track, file)
		if err != nil {
			slog.Warn("lyrics not fetched", "track", track.Title, "err", err)
		}
		out = append(out, describe(planned.Source, track, found))
	}
	return out, nil
}

func describe(source string, track musicbrainz.Track, r lyrics.Result) TrackLyrics {
	t := TrackLyrics{
		Source:       source,
		Title:        track.Title,
		Found:        !r.Empty(),
		Instrumental: r.Instrumental,
		WithoutAlbum: r.WithoutAlbum,
		Refused:      r.Refused,
	}
	if r.Synced != "" {
		t.Synced = true
		t.Lines = lyrics.Timed(r.Synced)
		t.Anchors = lyrics.Anchors(t.Lines, anchorCount)
	}
	return t
}

func (a *App) Words(ctx context.Context, release *musicbrainz.ReleaseDetail, p apply.Plan, files map[string]library.Track, noSync map[string]bool) map[string]apply.Words {
	out := map[string]apply.Words{}
	for _, planned := range p.Tracks {
		file, ok := files[planned.Source]
		if !ok {
			continue
		}
		track := release.Tracks[planned.Index]
		found, err := a.words(ctx, release, track, file)
		if err != nil {
			slog.Warn("lyrics not fetched", "track", track.Title, "err", err)
			continue
		}
		if found.Empty() {
			continue
		}
		w := apply.Words{Plain: found.Plain, Synced: found.Synced}
		if noSync[planned.Source] {
			w.Synced = ""
		}
		out[planned.Source] = w
	}
	return out
}

func (a *App) words(ctx context.Context, release *musicbrainz.ReleaseDetail, track musicbrainz.Track, file library.Track) (lyrics.Result, error) {
	length := fileLength(file)
	key := fmt.Sprintf("lyrics:%s:%s:%d", release.ID, track.TrackID, file.Audio.Length)

	var cached lyrics.Result
	if a.readCache(key, &cached) {
		return cached, nil
	}

	artist := track.Artist
	if artist == "" {
		artist = release.Artist
	}
	found, err := a.Lyr.Fetch(ctx, lyrics.Track{
		Artist: artist,
		Title:  track.Title,
		Album:  release.Title,
		Length: length,
	})
	if err != nil {
		return lyrics.Result{}, err
	}
	a.writeCache(key, found)
	return found, nil
}

func fileLength(file library.Track) time.Duration {
	return time.Duration(file.Audio.Length) * time.Millisecond
}

func byPath(tracks []library.Track) map[string]library.Track {
	out := make(map[string]library.Track, len(tracks))
	for _, t := range tracks {
		out[t.RelPath] = t
	}
	return out
}
