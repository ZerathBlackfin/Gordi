package apply

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"slices"

	"gordi/internal/i18n"

	"go.senan.xyz/taglib"
)

type Undo struct {
	Mode   string      `json:"mode"`
	Tracks []UndoTrack `json:"tracks"`
	Extras []string    `json:"extras,omitempty"`
}

type UndoTrack struct {
	Source      string              `json:"source"`
	Destination string              `json:"destination"`
	Size        int64               `json:"size"`
	ModTime     int64               `json:"mtime"`
	Tags        map[string][]string `json:"tags,omitempty"`
}

func changedTags(before, after map[string][]string) map[string][]string {
	out := map[string][]string{}
	for key, old := range before {
		if !slices.Equal(old, after[key]) {
			out[key] = old
		}
	}
	for key := range after {
		if _, ok := before[key]; !ok {
			out[key] = nil
		}
	}
	return out
}

func Reverse(u Undo, inbox, libraryDir string, lang i18n.Lang) error {
	type restore struct {
		from, to string
		tags     map[string][]string
	}
	var restores []restore
	var filed, relative []string

	for _, track := range u.Tracks {
		dest, err := safeDestination(libraryDir, track.Destination, lang)
		if err != nil {
			return err
		}
		source, err := safeDestination(inbox, track.Source, lang)
		if err != nil {
			return err
		}

		info, err := os.Stat(dest)
		if errors.Is(err, fs.ErrNotExist) {
			return i18n.Errorf(lang, "undo.missing", track.Destination)
		}
		if err != nil {
			return err
		}
		if info.Size() != track.Size || info.ModTime().UnixNano() != track.ModTime {
			return i18n.Errorf(lang, "undo.changed", track.Destination)
		}

		switch _, err := os.Stat(source); {
		case err == nil && u.Mode == string(ModeMove):
			return i18n.Errorf(lang, "undo.occupied", track.Source)
		case errors.Is(err, fs.ErrNotExist):
			restores = append(restores, restore{dest, source, track.Tags})
		case err != nil:
			return err
		}
		filed = append(filed, dest)
		relative = append(relative, track.Destination)
	}
	for _, extra := range u.Extras {
		path, err := safeDestination(libraryDir, extra, lang)
		if err != nil {
			return err
		}
		filed = append(filed, path)
		relative = append(relative, extra)
	}

	restored := make([]string, 0, len(restores))
	for _, r := range restores {
		err := copyThen(r.from, r.to, func(tmp string) error {
			if len(r.tags) == 0 {
				return nil
			}
			if err := taglib.WriteTags(tmp, r.tags, 0); err != nil {
				return fmt.Errorf("%s: %w", i18n.T(lang, "filing.writingTags"), err)
			}
			return nil
		})
		if err != nil {
			for _, f := range restored {
				os.Remove(f)
			}
			return fmt.Errorf("%s : %w", r.to, err)
		}
		restored = append(restored, r.to)
	}

	for _, f := range filed {
		if err := os.Remove(f); err != nil && !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("filed file not removed", "file", f, "err", err)
		}
	}
	pruneEmptyDirs(libraryDir, relative)
	return nil
}
