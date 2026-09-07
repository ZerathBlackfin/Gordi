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
	Cover   string   `json:"cover"`
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

	var embedded []byte
	if p.Art.Embed {
		embedded = p.Art.Image
	}

	for i, track := range p.Tracks {
		source := filepath.Join(inbox, filepath.FromSlash(track.Source))
		if err := copyAndTag(source, destinations[i], track.Tags, embedded, lang); err != nil {
			rollback()
			return res, fmt.Errorf("%s : %w", track.Source, err)
		}
		written = append(written, destinations[i])
		res.Files = append(res.Files, track.Destination)
	}
	res.Filed = len(written)

	if p.Cover != "" && len(p.Art.Image) > 0 {
		saved, err := writeCover(libraryDir, p.Cover, p.Art.Image, lang)
		if err != nil {
			slog.Warn("cover not written", "file", p.Cover, "err", err)
		} else if saved {
			res.Cover = p.Cover
		}
	}

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

func copyAndTag(source, dest string, tags map[string][]string, image []byte, lang i18n.Lang) error {
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
	if len(image) > 0 {
		if err := taglib.WriteImageOptions(tmpName, image, 0, "Front Cover", "", "image/jpeg"); err != nil {
			slog.Warn("cover not embedded", "file", dest, "err", err)
		}
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, dest)
}

func writeCover(libraryDir, relative string, image []byte, lang i18n.Lang) (bool, error) {
	dest, err := safeDestination(libraryDir, relative, lang)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(dest); err == nil {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(dest, image, 0o644); err != nil {
		return false, err
	}
	return true, nil
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
