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

	log, total, err := again.FiledLog(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(log) != 3 || total != 3 {
		t.Fatalf("3 filings expected, got %d of %d", len(log), total)
	}
	if log[0].Album != "The Wall" {
		t.Fatalf("newest first expected, got %q", log[0].Album)
	}
	if log[0].Date.Day() != 3 {
		t.Fatalf("the date must survive, got %v", log[0].Date)
	}

	// The limit trims what comes back, never what is counted.
	two, total, err := again.FiledLog(2)
	if err != nil || len(two) != 2 || total != 3 {
		t.Fatalf("limit ignored: %d entries of %d, %v", len(two), total, err)
	}
}
