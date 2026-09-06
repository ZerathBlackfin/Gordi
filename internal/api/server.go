package api

import (
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gordi/internal/app"
	"gordi/internal/musicbrainz"
)

func Handler(a *app.App, web fs.FS) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) {
		s, err := a.Status()
		if err != nil {
			fail(w, http.StatusInternalServerError, err)
			return
		}
		ok(w, s)
	})

	mux.HandleFunc("GET /api/albums", func(w http.ResponseWriter, r *http.Request) {
		albums, err := a.Albums(r.URL.Query().Get("status"))
		if err != nil {
			fail(w, http.StatusInternalServerError, err)
			return
		}
		ok(w, albums)
	})

	mux.HandleFunc("GET /api/filed", func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		entries, total, err := a.Store.FiledLog(limit)
		if err != nil {
			fail(w, http.StatusInternalServerError, err)
			return
		}
		ok(w, map[string]any{"total": total, "entries": entries})
	})

	mux.HandleFunc("GET /api/albums/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			fail(w, http.StatusBadRequest, err)
			return
		}
		album, err := a.Store.Get(id)
		if err != nil {
			fail(w, http.StatusInternalServerError, err)
			return
		}
		if album == nil {
			http.Error(w, "unknown album", http.StatusNotFound)
			return
		}
		ok(w, album)
	})

	mux.HandleFunc("GET /api/albums/{id}/cover", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			fail(w, http.StatusBadRequest, err)
			return
		}
		image, mime, err := a.Cover(id)
		if err != nil {
			fail(w, http.StatusInternalServerError, err)
			return
		}
		if mime == "" {
			http.Error(w, "no cover art", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", mime)
		w.Header().Set("Cache-Control", "private, max-age=300")
		w.Write(image)
	})

	mux.HandleFunc("GET /api/albums/{id}/candidates", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			fail(w, http.StatusBadRequest, err)
			return
		}
		q := r.URL.Query()
		filters := musicbrainz.Filters{
			Country: q.Get("country"),
			Format:  q.Get("format"),
			Status:  q.Get("status"),
			YearMin: intParam(q.Get("year_min")),
			YearMax: intParam(q.Get("year_max")),
		}
		suggestions, err := a.Candidates(r.Context(), id, filters, q.Get("force") != "")
		if err != nil {
			fail(w, http.StatusBadGateway, err)
			return
		}
		ok(w, suggestions)
	})

	mux.HandleFunc("GET /api/releases/{mbid}", func(w http.ResponseWriter, r *http.Request) {
		release, err := a.Release(r.Context(), r.PathValue("mbid"))
		if err != nil {
			fail(w, http.StatusBadGateway, err)
			return
		}
		ok(w, release)
	})

	mux.HandleFunc("GET /api/albums/{id}/plan", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			fail(w, http.StatusBadRequest, err)
			return
		}
		q := r.URL.Query()
		plan, err := a.Plan(r.Context(), id, q.Get("release_id"), a.RequestedMode(q.Get("mode")))
		if err != nil {
			fail(w, http.StatusBadRequest, err)
			return
		}
		ok(w, plan)
	})

	mux.HandleFunc("POST /api/albums/{id}/apply", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			fail(w, http.StatusBadRequest, err)
			return
		}
		var body struct {
			ReleaseID string `json:"release_id"`
			Mode      string `json:"mode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			fail(w, http.StatusBadRequest, err)
			return
		}
		res, err := a.Apply(r.Context(), id, body.ReleaseID, a.RequestedMode(body.Mode))
		if err != nil {
			fail(w, http.StatusBadRequest, err)
			return
		}
		ok(w, res)
	})

	mux.HandleFunc("GET /api/settings", func(w http.ResponseWriter, r *http.Request) {
		ok(w, a.Settings())
	})

	mux.HandleFunc("POST /api/settings", func(w http.ResponseWriter, r *http.Request) {
		var body app.SettingsPatch
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			fail(w, http.StatusBadRequest, err)
			return
		}
		if err := a.Update(body); err != nil {
			fail(w, http.StatusBadRequest, err)
			return
		}
		ok(w, a.Settings())
	})

	mux.HandleFunc("POST /api/settings/preview", func(w http.ResponseWriter, r *http.Request) {
		var body app.SettingsPatch
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			fail(w, http.StatusBadRequest, err)
			return
		}
		patterns, preview, err := a.Preview(body)
		if err != nil {
			fail(w, http.StatusBadRequest, err)
			return
		}
		ok(w, map[string]any{
			"pattern":       patterns.Simple,
			"pattern_multi": patterns.Multi,
			"preview":       preview,
		})
	})

	mux.HandleFunc("POST /api/settings/cache/clear", func(w http.ResponseWriter, r *http.Request) {
		n, err := a.ClearCache()
		if err != nil {
			fail(w, http.StatusInternalServerError, err)
			return
		}
		ok(w, map[string]int{"cleared": n})
	})

	mux.HandleFunc("POST /api/scan", func(w http.ResponseWriter, r *http.Request) {
		res, err := a.Rescan()
		if err != nil {
			fail(w, http.StatusInternalServerError, err)
			return
		}
		ok(w, res)
	})

	mux.Handle("/", spa(web))
	return logging(mux)
}

func spa(web fs.FS) http.Handler {
	files := http.FileServer(http.FS(web))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(web, path); err != nil {
			if _, err := fs.Stat(web, "index.html"); err != nil {
				http.Error(w, "web interface not built (run npm run build in web/)", http.StatusNotFound)
				return
			}
			r = r.Clone(r.Context())
			r.URL.Path = "/"
		}
		files.ServeHTTP(w, r)
	})
}

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			slog.Debug("http", "method", r.Method, "path", r.URL.Path, "elapsed", time.Since(start))
		}
	})
}

func intParam(v string) int {
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func ok(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("json encoding", "err", err)
	}
}

func fail(w http.ResponseWriter, code int, err error) {
	slog.Error("api", "code", code, "err", err)
	http.Error(w, err.Error(), code)
}
