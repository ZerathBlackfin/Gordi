package app

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gordi/internal/apply"
	"gordi/internal/i18n"
	"gordi/internal/store"
)

type undoRecord struct {
	Album int64 `json:"album"`
	apply.Undo
}

type FiledEntry struct {
	store.Filed
	ID       string     `json:"id"`
	Undo     bool       `json:"undo"`
	UndoneAt *time.Time `json:"undone_at,omitempty"`
}

type Journal struct {
	store.FiledCounts
	Entries []FiledEntry `json:"entries"`
}

func (a *App) undoPath(id int64) string {
	return filepath.Join(a.Cfg.BinDir, strconv.FormatInt(id, 10)+".json")
}

func (a *App) undoCutoff() int64 {
	return time.Now().Add(-time.Duration(a.UndoDays()) * 24 * time.Hour).UnixNano()
}

func (a *App) keepUndo(id, albumID int64, u apply.Undo) {
	if a.UndoDays() == 0 {
		return
	}
	data, err := json.Marshal(undoRecord{Album: albumID, Undo: u})
	if err == nil {
		err = os.MkdirAll(a.Cfg.BinDir, 0o755)
	}
	if err == nil {
		err = os.WriteFile(a.undoPath(id), data, 0o644)
	}
	if err != nil {
		slog.Error("album filed but cannot be undone", "album", albumID, "err", err)
	}
}

func (a *App) FiledLog(limit int) (Journal, error) {
	entries, counts, err := a.Store.FiledLog(limit)
	if err != nil {
		return Journal{}, err
	}
	undone := map[int64]time.Time{}
	for _, f := range entries {
		if f.Undoes != 0 {
			undone[f.Undoes] = f.Date
		}
	}
	cutoff := a.undoCutoff()
	j := Journal{FiledCounts: counts, Entries: make([]FiledEntry, 0, len(entries))}
	for _, f := range entries {
		id := f.Date.UnixNano()
		_, err := os.Stat(a.undoPath(id))
		e := FiledEntry{Filed: f, ID: strconv.FormatInt(id, 10), Undo: err == nil && id >= cutoff}
		if at, ok := undone[id]; ok {
			e.UndoneAt = &at
		}
		j.Entries = append(j.Entries, e)
	}
	return j, nil
}

func (a *App) Undo(id int64) error {
	lang := a.Lang()
	path := a.undoPath(id)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) || (err == nil && id < a.undoCutoff()) {
		return i18n.Errorf(lang, "undo.expired")
	}
	if err != nil {
		return err
	}
	var r undoRecord
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	if err := apply.Reverse(r.Undo, a.Cfg.Input, a.Cfg.Output, lang); err != nil {
		return err
	}

	if err := os.Remove(path); err != nil {
		slog.Error("album undone but its record stays", "album", r.Album, "err", err)
	}
	if f, err := a.Store.FiledAt(id); err != nil || f == nil {
		slog.Error("album undone but its filing is not in the log", "album", r.Album, "err", err)
	} else {
		f.Date, f.Undoes = time.Now().UTC(), id
		if err := a.Store.RecordFiled(*f); err != nil {
			slog.Error("album undone but not logged", "album", r.Album, "err", err)
		}
	}
	if err := a.Store.SetStatus(r.Album, store.StatusPending); err != nil {
		slog.Error("album undone but status not saved", "album", r.Album, "err", err)
	}
	slog.Info("album undone", "album", r.Album, "files", len(r.Tracks))

	go func() {
		if _, err := a.Rescan(); err != nil {
			slog.Debug("rescan after undo", "err", err)
		}
	}()
	return nil
}

func (a *App) purgeUndo() {
	entries, err := os.ReadDir(a.Cfg.BinDir)
	if err != nil {
		return
	}
	cutoff := a.undoCutoff()
	for _, e := range entries {
		id, err := strconv.ParseInt(strings.TrimSuffix(e.Name(), ".json"), 10, 64)
		if err != nil || id >= cutoff {
			continue
		}
		if err := os.Remove(filepath.Join(a.Cfg.BinDir, e.Name())); err != nil {
			slog.Error("undo sweep", "err", err)
		}
	}
}
