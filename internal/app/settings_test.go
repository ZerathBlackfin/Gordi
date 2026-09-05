package app

import (
	"path/filepath"
	"testing"

	"gordi/internal/apply"
	"gordi/internal/config"
	"gordi/internal/i18n"
	"gordi/internal/store"
)

func TestValidatePattern(t *testing.T) {
	a := testApp(t)
	good := []string{
		"{artist}/{album}/{track} - {title}",
		"{title}",
		"{artist} - {album} ({year})/{track}",
	}
	for _, m := range good {
		if err := a.ValidatePattern(m); err != nil {
			t.Errorf("pattern wrongly refused %q: %v", m, err)
		}
	}

	bad := map[string]string{
		"/{artist}/{title}":      "path absolute",
		"../{title}":             "climbs out of the library",
		"{artist}/{album}":       "no {title} and no {track}, collision guaranteed",
		"{artist}/{nosuchfield}": "unknown field",
	}
	for m, why := range bad {
		if err := a.ValidatePattern(m); err == nil {
			t.Errorf("pattern wrongly accepted (%s): %q", why, m)
		}
	}
}

func TestPreviewShowsThreeCases(t *testing.T) {
	got := previewEN(apply.Patterns{
		Simple: "{artist}/{album} ({year})/{track} - {title}",
		Multi:  "{artist}/{album} ({year})/CD{disc:0}/{track} - {title}",
	})
	want := []string{
		"Pink Floyd/Animals (1977)/02 - Dogs.flac",
		"Pink Floyd/Animals/02 - Dogs.flac",
		"Pink Floyd/The Wall (1979)/CD2/06 - Comfortably Numb.flac",
	}
	if len(got) != len(want) {
		t.Fatalf("want %d previewCases, got %d", len(want), len(got))
	}
	for i, want := range want {
		if got[i].Path != want {
			t.Errorf("%s : got %q, want %q", got[i].Case, got[i].Path, want)
		}
	}
}

func TestValidatePatternRejectsAngleBrackets(t *testing.T) {
	a := testApp(t)
	if err := a.ValidatePattern("{artist}/{album}< ({year})>/{title}"); err == nil {
		t.Fatal("a pattern with angle brackets must be refused")
	}
}

func TestUpdateDoesNotStoreDefaults(t *testing.T) {
	a := testApp(t)

	fallback := a.Cfg.Pattern
	if err := a.Update(SettingsPatch{Pattern: &fallback}); err != nil {
		t.Fatal(err)
	}
	if r := a.Settings(); len(r.Customized) != 0 {
		t.Fatalf("setting stored although it equals the default: %v", r.Customized)
	}

	other := "{album}/{track} - {title}"
	if err := a.Update(SettingsPatch{Pattern: &other}); err != nil {
		t.Fatal(err)
	}
	r := a.Settings()
	if len(r.Customized) != 1 || r.Customized[0] != keyPattern {
		t.Fatalf("custom setting not kept: %v", r.Customized)
	}
	if r.Pattern != other {
		t.Fatalf("pattern effective = %q", r.Pattern)
	}

	empty := ""
	if err := a.Update(SettingsPatch{Pattern: &empty}); err != nil {
		t.Fatal(err)
	}
	if r := a.Settings(); r.Pattern != fallback || len(r.Customized) != 0 {
		t.Fatalf("falling back to the default failed: %+v", r.Customized)
	}
}

func TestUpdateRejectsOutOfRangeValues(t *testing.T) {
	a := testApp(t)

	tooBig := 99999
	if err := a.Update(SettingsPatch{ScanEvery: &tooBig}); err == nil {
		t.Error("an absurd scan interval must be refused")
	}
	badMode := "erase"
	if err := a.Update(SettingsPatch{Mode: &badMode}); err == nil {
		t.Error("an unknown mode must be refused")
	}
	badContact := "me"
	if err := a.Update(SettingsPatch{MBContact: &badContact}); err == nil {
		t.Error("a contact with neither email nor URL must be refused")
	}
}

func testApp(t *testing.T) *App {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return New(config.Load(), st)
}

func TestZeroPadding(t *testing.T) {
	cases := map[string]string{
		"{artist}/{album}/{track} - {title}":     "Pink Floyd/Animals/02 - Dogs.flac",
		"{artist}/{album}/{track:000} - {title}": "Pink Floyd/Animals/002 - Dogs.flac",
		"{artist}/{album}/{track:0} - {title}":   "Pink Floyd/Animals/2 - Dogs.flac",
	}
	for pattern, want := range cases {
		got := previewEN(apply.Patterns{Simple: pattern, Multi: pattern + "/{disc}"})
		if len(got) == 0 {
			t.Fatalf("pattern refused: %s", pattern)
		}
		if got[0].Path != want {
			t.Errorf("%s\n  got %q\n  want %q", pattern, got[0].Path, want)
		}
	}
}

func TestPaddingRejectedOnText(t *testing.T) {
	a := testApp(t)
	if err := a.ValidatePattern("{artist}/{album:000}/{title}"); err == nil {
		t.Fatal("padding on {album} must be refused")
	}
	if err := a.ValidatePattern("{artist}/{album}/{track:000} - {title}"); err != nil {
		t.Fatalf("valid padding refused: %v", err)
	}
}

func TestPaddedFieldIsRecognized(t *testing.T) {
	a := testApp(t)
	if err := a.ValidateMultiPattern("{artist}/{album}/CD{disc:0}/{track} - {title}"); err != nil {
		t.Fatalf("multi pattern with padding refused: %v", err)
	}
	if err := a.ValidatePattern("{artist}/{album}/{track:000}"); err != nil {
		t.Fatalf("pattern with {track:000} refused: %v", err)
	}
	if err := a.ValidateMultiPattern("{artist}/{album}/{track} - {title}"); err == nil {
		t.Fatal("a multi pattern without disc must stay refused")
	}
}

func previewEN(patterns apply.Patterns) []Example { return Preview(patterns, i18n.EN) }

func TestLanguage(t *testing.T) {
	a := testApp(t)

	if a.Lang() != i18n.EN {
		t.Fatalf("default lang = %q, expected English", a.Lang())
	}
	if err := a.ValidatePattern("{artist}/{album}"); err == nil {
		t.Fatal("a pattern with neither title nor track must be refused")
	} else if !contains(err.Error(), "Add {title} or {track}") {
		t.Errorf("expected an English message, got : %v", err)
	}

	fr := "fr"
	if err := a.Update(SettingsPatch{Lang: &fr}); err != nil {
		t.Fatal(err)
	}
	if a.Lang() != i18n.FR {
		t.Fatalf("lang = %q after switching to French", a.Lang())
	}
	if err := a.ValidatePattern("{artist}/{album}"); err == nil {
		t.Fatal("the pattern must stay refused")
	} else if err.Error() != i18n.T(i18n.FR, "pattern.noTitle") {
		t.Errorf("expected a French message, got: %v", err)
	}

	unknown := "kl"
	if err := a.Update(SettingsPatch{Lang: &unknown}); err != nil {
		t.Fatal(err)
	}
	if a.Lang() != i18n.EN {
		t.Errorf("an unknown language should fall back to English, got %q", a.Lang())
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
