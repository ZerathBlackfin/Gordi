package musicbrainz

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"

	"gordi/internal/build"
	"gordi/internal/i18n"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const baseURL = "https://musicbrainz.org/ws/2"

type Release struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Disambiguation string `json:"disambiguation"`
	Artist         string `json:"artist"`
	Date           string `json:"date"`
	Country        string `json:"country"`
	Status         string `json:"status"`
	Packaging      string `json:"packaging"`
	Format         string `json:"format"`
	Label          string `json:"label"`
	Catalog        string `json:"catalog"`
	Barcode        string `json:"barcode"`
	ReleaseGroupID string `json:"release_group_id"`
	FromTags       bool   `json:"from_tags"`
	TrackCount     int    `json:"track_count"`
	Score          int    `json:"score"`
}

type Track struct {
	Position    int      `json:"position"`
	Disc        int      `json:"disc"`
	Number      string   `json:"number"`
	Title       string   `json:"title"`
	Artist      string   `json:"artist"`
	Length      int      `json:"length"`
	RecordingID string   `json:"recording_id"`
	ISRCs       []string `json:"isrcs"`
}

type Event struct {
	Date    string `json:"date"`
	Country string `json:"country"`
	Area    string `json:"area"`
}

type Medium struct {
	Position   int    `json:"position"`
	Format     string `json:"format"`
	Title      string `json:"title"`
	TrackCount int    `json:"track_count"`
}

type ReleaseDetail struct {
	Release

	SecondaryTypes []string `json:"secondary_types"`
	FirstRelease   string   `json:"first_release"`
	ASIN           string   `json:"asin"`
	Language       string   `json:"language"`
	Script         string   `json:"script"`
	Quality        string   `json:"quality"`
	ArtistID       string   `json:"artist_id"`
	ArtistSort     string   `json:"artist_sort"`
	CoverURL       string   `json:"cover_url"`
	Genres         []string `json:"genres"`
	ArtistGenres   []string `json:"artist_genres"`

	Events []Event  `json:"events"`
	Media  []Medium `json:"media"`
	Tracks []Track  `json:"tracks"`
}

type Client struct {
	http    *http.Client
	base    string
	limiter *limiter

	agentMu sync.RWMutex
	agent   string
	lang    i18n.Lang
}

// MusicBrainz asks that a client name itself and leave a way to be reached.
func userAgent(contact string) string {
	return fmt.Sprintf("Gordi/%s ( %s )", build.Version, contact)
}

func New(contact string) *Client {
	return &Client{
		http:    &http.Client{Timeout: 15 * time.Second},
		agent:   userAgent(contact),
		base:    baseURL,
		lang:    i18n.EN,
		limiter: &limiter{every: time.Second},
	}
}

type Filters struct {
	Country string `json:"country"`
	Format  string `json:"format"`
	YearMin int    `json:"year_min"`
	YearMax int    `json:"year_max"`
	Status  string `json:"status"`
}

func (f Filters) IsZero() bool { return f == Filters{} }

var mbFormats = map[string]string{
	"cd":       `format:CD`,
	"digital":  `format:"Digital Media"`,
	"vinyl":    `format:*Vinyl*`,
	"cassette": `format:Cassette`,
}

var mbStatuses = map[string]string{
	"official":       `status:Official`,
	"promotion":      `status:Promotion`,
	"bootleg":        `status:Bootleg`,
	"pseudo-release": `status:"Pseudo-Release"`,
}

var countryRE = regexp.MustCompile(`^[A-Za-z]{2}$`)

func (f Filters) clauses() []string {
	var out []string
	if countryRE.MatchString(f.Country) {
		out = append(out, "country:"+strings.ToUpper(f.Country))
	}
	if c, ok := mbFormats[strings.ToLower(f.Format)]; ok {
		out = append(out, c)
	}
	if c, ok := mbStatuses[strings.ToLower(f.Status)]; ok {
		out = append(out, c)
	}
	if f.YearMin > 0 || f.YearMax > 0 {
		min, max := "*", "*"
		if f.YearMin > 0 {
			min = strconv.Itoa(f.YearMin)
		}
		if f.YearMax > 0 {
			max = strconv.Itoa(f.YearMax)
		}
		out = append(out, fmt.Sprintf("date:[%s TO %s]", min, max))
	}
	return out
}

func (c *Client) SetContact(contact string) {
	c.agentMu.Lock()
	defer c.agentMu.Unlock()
	c.agent = userAgent(contact)
}

func (c *Client) SetLang(l i18n.Lang) {
	c.agentMu.Lock()
	defer c.agentMu.Unlock()
	c.lang = l
}

func (c *Client) currentLang() i18n.Lang {
	c.agentMu.RLock()
	defer c.agentMu.RUnlock()
	return c.lang
}

func (c *Client) userAgent() string {
	c.agentMu.RLock()
	defer c.agentMu.RUnlock()
	return c.agent
}

func (c *Client) Search(ctx context.Context, artist, album string, f Filters, limit int) ([]Release, error) {
	if strings.TrimSpace(album) == "" {
		return nil, fmt.Errorf("empty album title")
	}
	if limit <= 0 {
		limit = 25
	}

	parts := []string{fmt.Sprintf("release:%s", quote(album))}
	if a := strings.TrimSpace(artist); a != "" {
		parts = append([]string{fmt.Sprintf("artist:%s", quote(a))}, parts...)
	}
	parts = append(parts, f.clauses()...)

	return c.search(ctx, strings.Join(parts, " AND "), limit)
}

func (c *Client) search(ctx context.Context, query string, limit int) ([]Release, error) {
	params := url.Values{}
	params.Set("query", query)
	params.Set("limit", fmt.Sprint(limit))
	params.Set("fmt", "json")

	var payload struct {
		Releases []struct {
			ID             string `json:"id"`
			Title          string `json:"title"`
			Disambiguation string `json:"disambiguation"`
			Score          int    `json:"score"`
			Date           string `json:"date"`
			Country        string `json:"country"`
			Status         string `json:"status"`
			Packaging      string `json:"packaging"`
			Barcode        string `json:"barcode"`
			ReleaseGroup   struct {
				ID string `json:"id"`
			} `json:"release-group"`
			ArtistCredit []struct {
				Name string `json:"name"`
			} `json:"artist-credit"`
			Media []struct {
				Format     string `json:"format"`
				TrackCount int    `json:"track-count"`
			} `json:"media"`
			LabelInfo []struct {
				CatalogNumber string `json:"catalog-number"`
				Label         struct {
					Name string `json:"name"`
				} `json:"label"`
			} `json:"label-info"`
		} `json:"releases"`
	}
	if err := c.get(ctx, "/release?"+params.Encode(), &payload); err != nil {
		return nil, err
	}

	releases := make([]Release, 0, len(payload.Releases))
	for _, r := range payload.Releases {
		rel := Release{
			ID:             r.ID,
			Title:          r.Title,
			Disambiguation: r.Disambiguation,
			Score:          r.Score,
			Date:           r.Date,
			Country:        r.Country,
			Status:         r.Status,
			Packaging:      r.Packaging,
			Barcode:        r.Barcode,
			ReleaseGroupID: r.ReleaseGroup.ID,
		}
		if len(r.ArtistCredit) > 0 {
			rel.Artist = r.ArtistCredit[0].Name
		}
		for _, m := range r.Media {
			rel.TrackCount += m.TrackCount
			if rel.Format == "" {
				rel.Format = m.Format
			}
		}
		if len(r.LabelInfo) > 0 {
			rel.Label = r.LabelInfo[0].Label.Name
			rel.Catalog = r.LabelInfo[0].CatalogNumber
		}
		releases = append(releases, rel)
	}
	return releases, nil
}

const incRelease = "recordings+artist-credits+labels+release-groups+isrcs+genres"

type genreJSON struct {
	Name string `json:"name"`
}

func (c *Client) SearchBarcode(ctx context.Context, barcode string) ([]Release, error) {
	barcode = strings.TrimSpace(barcode)
	if barcode == "" {
		return nil, fmt.Errorf("empty barcode")
	}
	return c.search(ctx, "barcode:"+quote(barcode), 25)
}

func (c *Client) Release(ctx context.Context, mbid string) (*ReleaseDetail, error) {
	if strings.TrimSpace(mbid) == "" {
		return nil, fmt.Errorf("empty identifier")
	}

	var payload struct {
		ID                 string      `json:"id"`
		Title              string      `json:"title"`
		Disambiguation     string      `json:"disambiguation"`
		Date               string      `json:"date"`
		Country            string      `json:"country"`
		Status             string      `json:"status"`
		Packaging          string      `json:"packaging"`
		Barcode            string      `json:"barcode"`
		ASIN               string      `json:"asin"`
		Quality            string      `json:"quality"`
		Genres             []genreJSON `json:"genres"`
		TextRepresentation struct {
			Language string `json:"language"`
			Script   string `json:"script"`
		} `json:"text-representation"`
		ArtistCredit []struct {
			Name   string `json:"name"`
			Artist struct {
				ID       string      `json:"id"`
				SortName string      `json:"sort-name"`
				Genres   []genreJSON `json:"genres"`
			} `json:"artist"`
		} `json:"artist-credit"`
		ReleaseGroup struct {
			ID               string      `json:"id"`
			PrimaryType      string      `json:"primary-type"`
			SecondaryTypes   []string    `json:"secondary-types"`
			FirstReleaseDate string      `json:"first-release-date"`
			Genres           []genreJSON `json:"genres"`
		} `json:"release-group"`
		CoverArt struct {
			Front bool `json:"front"`
			Back  bool `json:"back"`
			Count int  `json:"count"`
		} `json:"cover-art-archive"`
		ReleaseEvents []struct {
			Date string `json:"date"`
			Area struct {
				Name  string   `json:"name"`
				Codes []string `json:"iso-3166-1-codes"`
			} `json:"area"`
		} `json:"release-events"`
		LabelInfo []struct {
			CatalogNumber string `json:"catalog-number"`
			Label         struct {
				Name string `json:"name"`
			} `json:"label"`
		} `json:"label-info"`
		Media []struct {
			Position   int    `json:"position"`
			Format     string `json:"format"`
			Title      string `json:"title"`
			TrackCount int    `json:"track-count"`
			Tracks     []struct {
				Position  int    `json:"position"`
				Number    string `json:"number"`
				Title     string `json:"title"`
				Length    int    `json:"length"`
				Recording struct {
					ID           string   `json:"id"`
					ISRCs        []string `json:"isrcs"`
					ArtistCredit []struct {
						Name string `json:"name"`
					} `json:"artist-credit"`
				} `json:"recording"`
			} `json:"tracks"`
		} `json:"media"`
	}
	path := "/release/" + url.PathEscape(mbid) + "?inc=" + incRelease + "&fmt=json"
	if err := c.get(ctx, path, &payload); err != nil {
		return nil, err
	}

	d := &ReleaseDetail{
		Release: Release{
			ID:             payload.ID,
			Title:          payload.Title,
			Disambiguation: payload.Disambiguation,
			Date:           payload.Date,
			Country:        payload.Country,
			Status:         payload.Status,
			Packaging:      payload.Packaging,
			ReleaseGroupID: payload.ReleaseGroup.ID,
			Barcode:        payload.Barcode,
		},
		SecondaryTypes: payload.ReleaseGroup.SecondaryTypes,
		FirstRelease:   payload.ReleaseGroup.FirstReleaseDate,
		ASIN:           payload.ASIN,
		Language:       payload.TextRepresentation.Language,
		Script:         payload.TextRepresentation.Script,
		Quality:        payload.Quality,
		Genres:         genreNames(payload.Genres),
		Events:         []Event{},
		Media:          []Medium{},
		Tracks:         []Track{},
	}
	if len(payload.ArtistCredit) > 0 {
		a := payload.ArtistCredit[0]
		d.Artist = a.Name
		d.ArtistID = a.Artist.ID
		d.ArtistSort = a.Artist.SortName
		d.ArtistGenres = genreNames(a.Artist.Genres)
	}
	if len(payload.LabelInfo) > 0 {
		d.Label = payload.LabelInfo[0].Label.Name
		d.Catalog = payload.LabelInfo[0].CatalogNumber
	}
	if payload.CoverArt.Front {
		d.CoverURL = "https://coverartarchive.org/release/" + url.PathEscape(payload.ID) + "/front-500"
	}
	for _, e := range payload.ReleaseEvents {
		event := Event{Date: e.Date, Area: e.Area.Name}
		if len(e.Area.Codes) > 0 {
			event.Country = e.Area.Codes[0]
		}
		d.Events = append(d.Events, event)
	}

	for _, m := range payload.Media {
		d.Media = append(d.Media, Medium{
			Position:   m.Position,
			Format:     m.Format,
			Title:      m.Title,
			TrackCount: m.TrackCount,
		})
		if d.Format == "" {
			d.Format = m.Format
		}
		for _, t := range m.Tracks {
			track := Track{
				Position:    t.Position,
				Disc:        m.Position,
				Number:      t.Number,
				Title:       t.Title,
				Length:      t.Length,
				Artist:      d.Artist,
				RecordingID: t.Recording.ID,
				ISRCs:       t.Recording.ISRCs,
			}
			if len(t.Recording.ArtistCredit) > 0 {
				track.Artist = t.Recording.ArtistCredit[0].Name
			}
			d.Tracks = append(d.Tracks, track)
		}
	}
	d.TrackCount = len(d.Tracks)
	return d, nil
}

func genreNames(genres []genreJSON) []string {
	if len(genres) == 0 {
		return nil
	}
	out := make([]string, 0, len(genres))
	for _, g := range genres {
		out = append(out, g.Name)
	}
	sort.Strings(out)
	return out
}

const maxAttempts = 4

func (c *Client) get(ctx context.Context, path string, out any) error {
	for attempt := 1; ; attempt++ {
		retry, err := c.attempt(ctx, path, out)
		if err == nil {
			return nil
		}
		if !retry || attempt >= maxAttempts {
			return err
		}
		if err := sleepCtx(ctx, time.Duration(attempt)*time.Second); err != nil {
			return err
		}
		slog.Debug("musicbrainz busy, retrying", "attempt", attempt+1, "path", path)
	}
}

func (c *Client) attempt(ctx context.Context, path string, out any) (bool, error) {
	if err := c.limiter.wait(ctx); err != nil {
		return false, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("User-Agent", c.userAgent())

	res, err := c.http.Do(req)
	if err != nil {
		return true, i18n.Errorf(c.currentLang(), "mb.unreachable", err)
	}
	defer res.Body.Close()

	switch {
	case res.StatusCode == http.StatusOK:
		return false, json.NewDecoder(res.Body).Decode(out)
	case res.StatusCode == http.StatusServiceUnavailable:
		return true, i18n.Errorf(c.currentLang(), "mb.busy")
	case res.StatusCode >= 500:
		return true, i18n.Errorf(c.currentLang(), "mb.response", res.Status, "")
	default:
		body, _ := io.ReadAll(io.LimitReader(res.Body, 300))
		return false, i18n.Errorf(c.currentLang(), "mb.response", res.Status, strings.TrimSpace(string(body)))
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

func quote(v string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + r.Replace(v) + `"`
}

type limiter struct {
	mu    sync.Mutex
	last  time.Time
	every time.Duration
}

func (l *limiter) wait(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if wait := time.Until(l.last.Add(l.every)); wait > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
	l.last = time.Now()
	return nil
}
