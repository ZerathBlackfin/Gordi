package config

import (
	"gordi/internal/i18n"
	"os"
	"strconv"
	"time"
)

type Mode string

const (
	ModeMove Mode = "move"
	ModeCopy Mode = "copy"
)

type Config struct {
	Addr      string
	Input     string
	Output    string
	DBPath    string
	Mode      Mode
	ScanEvery time.Duration

	MBContact string

	PrefetchEvery time.Duration

	Lang string

	Pattern      string
	PatternMulti string
}

func Load() Config {
	c := Config{
		Addr:          env("GORDI_ADDR", ":7373"),
		Input:         env("GORDI_INPUT", "/input"),
		Output:        env("GORDI_OUTPUT", "/output"),
		DBPath:        env("GORDI_DB", "/config/gordi.bolt"),
		Mode:          Mode(env("GORDI_MODE", string(ModeMove))),
		ScanEvery:     time.Duration(envInt("GORDI_SCAN_EVERY", 60)) * time.Second,
		MBContact:     env("GORDI_CONTACT", "https://github.com/ZerathBlackfin/Gordi"),
		Lang:          string(i18n.ParseLang(env("GORDI_LANG", "en"))),
		PrefetchEvery: time.Duration(envInt("GORDI_PREFETCH_EVERY", 3)) * time.Second,
		Pattern:       env("GORDI_PATTERN", "{artist}/{album} ({year})/{track} - {title}"),
		PatternMulti:  env("GORDI_PATTERN_MULTI", "{artist}/{album} ({year})/CD{disc:0}/{track} - {title}"),
	}
	if c.Mode != ModeMove && c.Mode != ModeCopy {
		c.Mode = ModeMove
	}
	return c
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 0 {
		return def
	}
	return v
}
