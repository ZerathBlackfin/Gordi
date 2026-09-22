package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestFiledLogSurvivesRestartAndReadsNewestFirst(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gordi.json")

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for i, title := range []string{"Meddle", "Animals", "The Wall"} {
		err := s.RecordFiled(Filed{
			Date:   time.Date(2026, 9, i+1, 0, 0, 0, 0, time.UTC),
			Artist: "Pink Floyd", Album: title, Tracks: 5,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	s.Close()

	again, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()

	log, counts, err := again.FiledLog(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(log) != 3 || counts.Total != 3 {
		t.Fatalf("3 filings expected, got %d of %d", len(log), counts.Total)
	}
	if log[0].Album != "The Wall" {
		t.Fatalf("newest first expected, got %q", log[0].Album)
	}
	if log[0].Date.Day() != 3 {
		t.Fatalf("the date must survive, got %v", log[0].Date)
	}

	// The limit trims what comes back, never what is counted.
	two, counts, err := again.FiledLog(2)
	if err != nil || len(two) != 2 || counts.Total != 3 {
		t.Fatalf("limit ignored: %d entries of %d, %v", len(two), counts.Total, err)
	}
}

func TestAnUndoIsLoggedBesideTheFiling(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "gordi.bolt"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	filed := Filed{Date: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), Album: "Animals", Tracks: 5}
	undo := filed
	undo.Date, undo.Undoes = filed.Date.Add(time.Hour), filed.Date.UnixNano()
	for _, f := range []Filed{filed, undo} {
		if err := s.RecordFiled(f); err != nil {
			t.Fatal(err)
		}
	}

	log, counts, err := s.FiledLog(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(log) != 2 || log[0].Undoes != filed.Date.UnixNano() {
		t.Fatalf("the undo should sit on top of the filing: %+v", log)
	}
	if counts.Filed != 1 || counts.Undone != 1 || !counts.Last.Equal(filed.Date) {
		t.Errorf("counts %+v", counts)
	}
	if got, err := s.FiledAt(filed.Date.UnixNano()); err != nil || got == nil || got.Album != "Animals" {
		t.Errorf("filing not found by its id: %+v, %v", got, err)
	}
}
