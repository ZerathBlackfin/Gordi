package lyrics

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	Untimed   = "no timestamp"
	Unordered = "timestamps out of order"
	Overrun   = "runs past the end of the file"
)

var stampRE = regexp.MustCompile(`^\[(\d{1,3}):(\d{1,2})(?:[.:](\d{1,3}))?\]`)

const grace = time.Second

func stamps(synced string) []time.Duration {
	var out []time.Duration
	for _, line := range strings.Split(synced, "\n") {
		m := stampRE.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		minutes, _ := strconv.Atoi(m[1])
		seconds, _ := strconv.Atoi(m[2])
		at := time.Duration(minutes)*time.Minute + time.Duration(seconds)*time.Second
		out = append(out, at+fraction(m[3]))
	}
	return out
}

func fraction(digits string) time.Duration {
	n, _ := strconv.Atoi(digits)
	switch len(digits) {
	case 1:
		return time.Duration(n) * 100 * time.Millisecond
	case 2:
		return time.Duration(n) * 10 * time.Millisecond
	case 3:
		return time.Duration(n) * time.Millisecond
	}
	return 0
}

func Fits(synced string, length time.Duration) (bool, string) {
	times := stamps(synced)
	if len(times) == 0 {
		return false, Untimed
	}
	for i, at := range times[1:] {
		if at < times[i] {
			return false, Unordered
		}
	}
	if length > 0 && times[len(times)-1] > length+grace {
		return false, Overrun
	}
	return true, ""
}

func Anchors(lines []Line, want int) []int {
	if want <= 0 || len(lines) == 0 {
		return nil
	}
	out := make([]int, 0, want)
	if len(lines) <= want {
		for i := range lines {
			out = append(out, i)
		}
		return out
	}
	for i := 0; i < want; i++ {
		out = append(out, (2*i+1)*len(lines)/(2*want))
	}
	return out
}

type Line struct {
	At   float64 `json:"at"`
	Text string  `json:"text"`
}

func Timed(synced string) []Line {
	var out []Line
	for _, raw := range strings.Split(synced, "\n") {
		line := strings.TrimSpace(raw)
		m := stampRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		text := strings.TrimSpace(line[len(m[0]):])
		if text == "" {
			continue
		}
		minutes, _ := strconv.Atoi(m[1])
		seconds, _ := strconv.Atoi(m[2])
		at := time.Duration(minutes)*time.Minute + time.Duration(seconds)*time.Second + fraction(m[3])
		out = append(out, Line{At: at.Seconds(), Text: text})
	}
	return out
}
