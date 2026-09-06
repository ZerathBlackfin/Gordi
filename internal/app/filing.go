package app

import (
	"context"
	"log/slog"
	"path/filepath"

	"gordi/internal/apply"
	"gordi/internal/config"
	"gordi/internal/i18n"
	"gordi/internal/musicbrainz"
	"gordi/internal/store"
)

func (a *App) Plan(ctx context.Context, albumID int64, releaseID string, mode apply.Mode) (apply.Plan, error) {
	album, release, err := a.albumAndRelease(ctx, albumID, releaseID)
	if err != nil {
		return apply.Plan{}, err
	}
	p, err := apply.Prepare(album.Album, *release, a.Patterns(), a.Lang())
	if err != nil {
		return apply.Plan{}, err
	}
	p.Mode = string(mode)
	a.markReady(album)
	return p, nil
}

func (a *App) Apply(ctx context.Context, albumID int64, releaseID string, mode apply.Mode) (apply.Result, error) {
	album, release, err := a.albumAndRelease(ctx, albumID, releaseID)
	if err != nil {
		return apply.Result{}, err
	}
	p, err := apply.Prepare(album.Album, *release, a.Patterns(), a.Lang())
	if err != nil {
		return apply.Result{}, err
	}
	p.Mode = string(mode)

	res, err := apply.Execute(p, a.Cfg.Input, a.Cfg.Output, mode, a.Lang())
	if err != nil {
		return apply.Result{}, err
	}

	if err := a.Store.SetStatus(albumID, store.StatusDone); err != nil {
		slog.Error("album filed but status not saved", "album", albumID, "err", err)
	}
	slog.Info("album filed", "album", albumID, "files", res.Filed, "mode", res.Mode)

	if err := a.Store.RecordFiled(store.Filed{
		Artist:      album.Artist,
		Album:       album.Title,
		Year:        album.Year,
		Tracks:      res.Filed,
		Destination: filepath.Dir(p.Tracks[0].Destination),
	}); err != nil {
		slog.Error("album filed but not logged", "album", albumID, "err", err)
	}

	go func() {
		if _, err := a.Rescan(); err != nil {
			slog.Debug("rescan after filing", "err", err)
		}
	}()
	return res, nil
}

func (a *App) albumAndRelease(ctx context.Context, albumID int64, releaseID string) (*store.Album, *musicbrainz.ReleaseDetail, error) {
	album, err := a.Store.Get(albumID)
	if err != nil {
		return nil, nil, err
	}
	if album == nil {
		return nil, nil, i18n.Errorf(a.Lang(), "mb.unknownAlbum")
	}
	release, err := a.Release(ctx, releaseID)
	if err != nil {
		return nil, nil, err
	}
	return album, release, nil
}

func (a *App) RequestedMode(asked string) apply.Mode {
	switch apply.Mode(asked) {
	case apply.ModeCopy:
		return apply.ModeCopy
	case apply.ModeMove:
		return apply.ModeMove
	}
	if a.Cfg.Mode == config.ModeCopy {
		return apply.ModeCopy
	}
	return apply.ModeMove
}
