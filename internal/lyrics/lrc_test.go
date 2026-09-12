package lyrics

import (
	"testing"
	"time"
)

const (
	heroes = "[00:17.30] I, I will be king\n[05:50.12] We can be heroes, just for one day"
	teen   = "[00:34.10] Load up on guns, bring your friends\n[04:46.50] A denial"
	brel   = "[00:07.00] Ne me quitte pas\n[03:56.00] Ne me quitte pas"
	lucky  = "[00:31.48] Like the legend of the phoenix\n[05:55.00] We're up all night to get lucky"
)

func TestFitsKeepsABlockThatEndsInsideTheFile(t *testing.T) {
	ok, why := Fits(lucky, 367*time.Second)
	if !ok {
		t.Fatalf("a block ending before the file does should be kept, refused for %q", why)
	}
}

func TestFitsRefusesTimingsFromAnotherEdit(t *testing.T) {
	cases := map[string]struct {
		synced string
		length time.Duration
	}{
		"Heroes, album timings on the single":  {heroes, 303 * time.Second},
		"Smells Like Teen Spirit, 11s too far": {teen, 275 * time.Second},
		"Ne me quitte pas, 11s too far":        {brel, 225 * time.Second},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			ok, why := Fits(c.synced, c.length)
			if ok {
				t.Fatal("a block running past the end of the file must be refused")
			}
			if why != Overrun {
				t.Fatalf("want %q, got %q", Overrun, why)
			}
		})
	}
}

func TestFitsAllowsASecondOfSlack(t *testing.T) {
	if ok, why := Fits("[00:10.00] one\n[03:00.80] two", 180*time.Second); !ok {
		t.Fatalf("a file a hair short of the master should still pass, refused for %q", why)
	}
	if ok, _ := Fits("[00:10.00] one\n[03:02.00] two", 180*time.Second); ok {
		t.Fatal("two seconds past the end is a mismatch, not slack")
	}
}

func TestFitsRefusesBlocksItCannotRead(t *testing.T) {
	if ok, why := Fits("Load up on guns, bring your friends", time.Minute); ok || why != Untimed {
		t.Fatalf("plain text carries no timing: got ok=%v why=%q", ok, why)
	}
	if ok, why := Fits("[00:30.00] later\n[00:10.00] earlier", time.Minute); ok || why != Unordered {
		t.Fatalf("times running backwards must be refused: got ok=%v why=%q", ok, why)
	}
}

func TestStampsReadEveryFractionLength(t *testing.T) {
	got := stamps("[00:01]a\n[00:01.5]b\n[00:01.50]c\n[00:01.500]d\n[01:02:25]e")
	want := []time.Duration{
		time.Second,
		1500 * time.Millisecond,
		1500 * time.Millisecond,
		1500 * time.Millisecond,
		62*time.Second + 250*time.Millisecond,
	}
	if len(got) != len(want) {
		t.Fatalf("want %d timings, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d: want %s, got %s", i+1, want[i], got[i])
		}
	}
}

func TestTimedSkipsLinesWithoutWords(t *testing.T) {
	lines := Timed("[00:00.00]\n[00:10.00] the only words\n[00:20.00]   ")
	if len(lines) != 1 || lines[0].Text != "the only words" {
		t.Fatalf("empty lines should be left out: %+v", lines)
	}
	if lines[0].At != 10 {
		t.Fatalf("want 10s, got %v", lines[0].At)
	}
}

func TestAnchorsSpreadOverTheSong(t *testing.T) {
	lines := Timed("[00:10.00] a\n[00:20.00] b\n[00:30.00] c\n[00:40.00] d\n[00:50.00] e\n[01:00.00] f")
	got := Anchors(lines, 2)
	if len(got) != 2 {
		t.Fatalf("want 2 anchors, got %d", len(got))
	}
	if lines[got[0]].Text != "b" || lines[got[1]].Text != "e" {
		t.Fatalf("anchors should sit away from both ends, got %q and %q",
			lines[got[0]].Text, lines[got[1]].Text)
	}
}

func TestAnchorsGiveBackWhatLittleThereIs(t *testing.T) {
	if got := Anchors(Timed("[00:10.00] alone"), 2); len(got) != 1 {
		t.Fatalf("a single line is still worth listening to, got %d", len(got))
	}
	if got := Anchors(nil, 2); got != nil {
		t.Fatalf("nothing to anchor on: %+v", got)
	}
}
