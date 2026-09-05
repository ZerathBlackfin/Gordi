package apply

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gordi/internal/i18n"

	"go.senan.xyz/taglib"
)

type Mode string

const (
	ModeCopy Mode = "copy"
	ModeMove Mode = "move"
)

type Result struct {
	Mode    string   `json:"mode"`
	Filed   int      `json:"filed"`
	Files   []string `json:"files"`
	Deleted int      `json:"deleted"`
	Ignored []string `json:"ignored"`
}

func Execute(p Plan, inbox, libraryDir string, mode Mode, lang i18n.Lang) (Result, error) {
	res := Result{Mode: string(mode), Ignored: p.Ignored}
	if len(p.Tracks) == 0 {
		return res, i18n.Errorf(lang, "filing.emptyPlan")
	}

	destinations := make([]string, 0, len(p.Tracks))
	for _, track := range p.Tracks {
		dest, err := safeDestination(libraryDir, track.Destination, lang)
		if err != nil {
			return res, err
		}
		if _, err := os.Stat(dest); err == nil {
			return res, i18n.Errorf(lang, "filing.alreadyExists", track.Destination)
		}
		destinations = append(destinations, dest)
	}

	written := make([]string, 0, len(p.Tracks))
	rollback := func() {
		for _, f := range written {
			os.Remove(f)
		}
	}

	for i, track := range p.Tracks {
		source := filepath.Join(inbox, filepath.FromSlash(track.Source))
		if err := copyAndTag(source, destinations[i], track.Tags, lang); err != nil {
			rollback()
			return res, fmt.Errorf("%s : %w", track.Source, err)
		}
		written = append(written, destinations[i])
		res.Files = append(res.Files, track.Destination)
	}
	res.Filed = len(written)

	if mode == ModeMove {
		for _, track := range p.Tracks {
			source := filepath.Join(inbox, filepath.FromSlash(track.Source))
			if err := os.Remove(source); err != nil {
				slog.Warn("original not deleted", "file", track.Source, "err", err)
				continue
			}
			res.Deleted++
		}
		pruneEmptyDirs(inbox, p.Tracks)
	}
	return res, nil
}

func copyAndTag(source, dest string, tags map[string][]string, lang i18n.Lang) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(dest), ".gordi-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	inbox, err := os.Open(source)
	if err != nil {
		tmp.Close()
		return err
	}
	_, err = io.Copy(tmp, inbox)
	inbox.Close()
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}

	if err := taglib.WriteTags(tmpName, tags, taglib.Clear); err != nil {
		return fmt.Errorf("%s: %w", i18n.T(lang, "filing.writingTags"), err)
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, dest)
}

func safeDestination(libraryDir, relative string, lang i18n.Lang) (string, error) {
	dest := filepath.Join(libraryDir, filepath.FromSlash(relative))
	rel, err := filepath.Rel(libraryDir, dest)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", i18n.Errorf(lang, "filing.outsideLibrary", relative)
	}
	return dest, nil
}

func pruneEmptyDirs(inbox string, tracks []PlannedTrack) {
	seen := map[string]bool{}
	for _, track := range tracks {
		dir := filepath.Dir(filepath.Join(inbox, filepath.FromSlash(track.Source)))
		for dir != inbox && !seen[dir] && strings.HasPrefix(dir, inbox) {
			seen[dir] = true
			if err := os.Remove(dir); err != nil {
				break // not empty: stop here
			}
			dir = filepath.Dir(dir)
		}
	}
}
