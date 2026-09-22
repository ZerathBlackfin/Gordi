package app

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"gordi/internal/apply"
	"gordi/internal/store"
)

func undoApp(t *testing.T) *App {
	t.Helper()
	a := testApp(t)
	a.Cfg.Input, a.Cfg.Output = t.TempDir(), t.TempDir()
	a.Cfg.BinDir = filepath.Join(t.TempDir(), ".bin")
	return a
}

func TestAFilingCanBeUndoneOnce(t *testing.T) {
	a := undoApp(t)
	filed := store.Filed{Date: time.Now().UTC(), Artist: "Otyken", Album: "Kykakacha"}
	if err := a.Store.RecordFiled(filed); err != nil {
		t.Fatal(err)
	}
	id := filed.Date.UnixNano()
	a.keepUndo(id, 42, apply.Undo{Mode: string(apply.ModeMove)})

	j, err := a.FiledLog(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(j.Entries) != 1 || !j.Entries[0].Undo || j.Entries[0].ID != strconv.FormatInt(id, 10) {
		t.Fatalf("the filing should offer an undo: %+v", j.Entries)
	}

	if err := a.Undo(id); err != nil {
		t.Fatal(err)
	}
	j, err = a.FiledLog(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(j.Entries) != 2 || j.Filed != 1 || j.Undone != 1 {
		t.Fatalf("the filing and its undo should both be logged: %+v", j)
	}
	undo, original := j.Entries[0], j.Entries[1]
	if undo.Undoes != id || undo.Album != "Kykakacha" || undo.Undo {
		t.Errorf("undo line %+v", undo)
	}
	if original.UndoneAt == nil || !original.UndoneAt.Equal(undo.Date) || original.Undo {
		t.Errorf("the filing should read as undone, with nothing left to undo: %+v", original)
	}
	if _, err := os.Stat(a.undoPath(id)); err == nil {
		t.Error("the record outlived the undo")
	}
	if err := a.Undo(id); err == nil {
		t.Error("the same filing was undone twice")
	}
}

func TestOldRecordsAreSwept(t *testing.T) {
	a := undoApp(t)
	old := time.Now().Add(-8 * 24 * time.Hour).UnixNano()
	fresh := time.Now().UnixNano()
	a.keepUndo(old, 1, apply.Undo{})
	a.keepUndo(fresh, 2, apply.Undo{})

	a.purgeUndo()

	if _, err := os.Stat(a.undoPath(old)); err == nil {
		t.Error("a record past 7 days survived the sweep")
	}
	if _, err := os.Stat(a.undoPath(fresh)); err != nil {
		t.Error("a fresh record was swept")
	}
}

func TestNothingKeptWhenUndoIsOff(t *testing.T) {
	a := undoApp(t)
	off := 0
	if err := a.Update(SettingsPatch{UndoDays: &off}); err != nil {
		t.Fatal(err)
	}
	id := time.Now().UnixNano()
	a.keepUndo(id, 1, apply.Undo{})
	if _, err := os.Stat(a.undoPath(id)); err == nil {
		t.Error("a record was kept with undo turned off")
	}
}
