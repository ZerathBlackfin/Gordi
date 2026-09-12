package app

import (
	"testing"
	"time"

	"gordi/internal/library"
	"gordi/internal/lyrics"
	"gordi/internal/musicbrainz"
)

func TestFileLengthReadsMilliseconds(t *testing.T) {
	file := library.Track{Audio: library.Audio{Length: 234800}}
	if got := fileLength(file); got != 234800*time.Millisecond {
		t.Fatalf("want 3m54.8s, got %s", got)
	}
	if seconds := int(fileLength(file).Round(time.Second).Seconds()); seconds != 235 {
		t.Fatalf("LRCLIB should be asked for 235, not %d", seconds)
	}
}

func TestDescribeCountsTheLinesItWillOffer(t *testing.T) {
	got := describe("junk/a.flac", musicbrainz.Track{Title: "Call Me Star"},
		lyrics.Result{
			Plain:  "one\n\ntwo\nthree",
			Synced: "[00:10.00] one\n[01:00.00] two\n[02:00.00] three\n[03:00.00] four",
		})

	if !got.Found || !got.Synced {
		t.Fatalf("a block with words and timings counts as both: %+v", got)
	}
	if len(got.Lines) != 4 {
		t.Errorf("want the 4 timed lines, got %d", len(got.Lines))
	}
	if len(got.Anchors) != anchorCount {
		t.Errorf("want %d places to start from, got %d", anchorCount, len(got.Anchors))
	}
}

func TestDescribeKeepsTheWordsWhenTheTimingsAreTurnedDown(t *testing.T) {
	got := describe("junk/a.flac", musicbrainz.Track{Title: "Heroes"},
		lyrics.Result{Plain: "I, I will be king", Refused: lyrics.Overrun})

	if got.Synced || len(got.Lines) != 0 {
		t.Fatal("nothing to listen to when the timings are gone")
	}
	if !got.Found || got.Refused != lyrics.Overrun {
		t.Fatalf("the words survive and say why: %+v", got)
	}
}
