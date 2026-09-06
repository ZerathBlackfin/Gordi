package store

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"go.etcd.io/bbolt"

	"gordi/internal/library"
)

const (
	StatusPending = "pending"
	StatusDone    = "done" // filed into the library
)

const missesBeforeForget = 2

const stateVersion = 2

// Keys are sorted, so folder and instant give order free; payloads sit apart.
var (
	bMeta     = []byte("meta")
	bAlbums   = []byte("albums")
	bTracks   = []byte("tracks")
	bIDs      = []byte("ids")
	bCache    = []byte("cache")
	bCacheExp = []byte("cache_expiry")
	bSettings = []byte("settings")
	bFiled    = []byte("filed")
)

var buckets = [][]byte{bMeta, bAlbums, bTracks, bIDs, bCache, bCacheExp, bSettings, bFiled}

type Album struct {
	library.Album

	ID         int64     `json:"id"`
	Status     string    `json:"status"`
	TrackCount int       `json:"track_count"`
	LastSeen   time.Time `json:"last_seen"`

	Indexed bool `json:"indexed"`
}

type SyncResult struct {
	Added   int `json:"added"`
	Updated int `json:"updated"`
	Removed int `json:"removed"`
}

// The queue entry. Its tracks live beside it, so listing skips them.
type entry struct {
	Album  Album `json:"album"`
	Misses int   `json:"misses"`
}

type Store struct {
	db *bbolt.DB
}

func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("database folder: %w", err)
		}
	}
	db, err := bbolt.Open(path, 0o644, &bbolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, err
	}

	err = db.Update(func(tx *bbolt.Tx) error {
		for _, name := range buckets {
			if _, err := tx.CreateBucketIfNotExists(name); err != nil {
				return err
			}
		}

		version := tx.Bucket(bMeta).Get([]byte("version"))
		if version != nil && string(version) == strconv.Itoa(stateVersion) {
			return nil
		}
		// The next scan rebuilds the queue from disk. Settings and log cannot be.
		for _, name := range [][]byte{bAlbums, bTracks, bIDs} {
			if err := tx.DeleteBucket(name); err != nil && err != bbolt.ErrBucketNotFound {
				return err
			}
			if _, err := tx.CreateBucket(name); err != nil {
				return err
			}
		}
		return tx.Bucket(bMeta).Put([]byte("version"), []byte(strconv.Itoa(stateVersion)))
	})
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func itob(v int64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(v))
	return b
}

func (s *Store) Sync(albums []library.Album) (SyncResult, error) {
	var res SyncResult
	now := time.Now().UTC()

	err := s.db.Update(func(tx *bbolt.Tx) error {
		all, tracks, ids := tx.Bucket(bAlbums), tx.Bucket(bTracks), tx.Bucket(bIDs)
		meta := tx.Bucket(bMeta)

		nextID := int64(1)
		if v := meta.Get([]byte("next_id")); v != nil {
			nextID = int64(binary.BigEndian.Uint64(v))
		}

		seen := map[string]bool{}
		for _, a := range albums {
			seen[a.RelDir] = true
			key := []byte(a.RelDir)
			list := a.Tracks
			a.Tracks = nil

			e := entry{}
			if raw := all.Get(key); raw != nil {
				if err := json.Unmarshal(raw, &e); err != nil {
					return err
				}
				e.Album.Album = a
				e.Album.LastSeen = now
				e.Misses = 0
				res.Updated++
			} else {
				e = entry{Album: Album{
					Album: a, ID: nextID, Status: StatusPending,
					LastSeen: now,
				}}
				if err := ids.Put(itob(nextID), key); err != nil {
					return err
				}
				nextID++
				res.Added++
			}

			if err := putJSON(all, key, e); err != nil {
				return err
			}
			if err := putJSON(tracks, key, list); err != nil {
				return err
			}
		}

		var gone [][]byte
		cursor := all.Cursor()
		for k, raw := cursor.First(); k != nil; k, raw = cursor.Next() {
			if seen[string(k)] {
				continue
			}
			var e entry
			if err := json.Unmarshal(raw, &e); err != nil {
				return err
			}
			e.Misses++
			if e.Misses < missesBeforeForget {
				if err := putJSON(all, k, e); err != nil {
					return err
				}
				continue
			}
			gone = append(gone, append([]byte(nil), k...))
			if err := ids.Delete(itob(e.Album.ID)); err != nil {
				return err
			}
		}
		for _, k := range gone {
			if err := all.Delete(k); err != nil {
				return err
			}
			if err := tracks.Delete(k); err != nil {
				return err
			}
			res.Removed++
		}

		return meta.Put([]byte("next_id"), itob(nextID))
	})
	return res, err
}

func putJSON(b *bbolt.Bucket, key []byte, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return b.Put(key, data)
}

func (s *Store) List(status string) ([]Album, error) {
	albums := []Album{}
	err := s.db.View(func(tx *bbolt.Tx) error {
		tracks := tx.Bucket(bTracks)
		return tx.Bucket(bAlbums).ForEach(func(k, raw []byte) error {
			var e entry
			if err := json.Unmarshal(raw, &e); err != nil {
				return err
			}
			if status != "" && e.Album.Status != status {
				return nil
			}
			e.Album.TrackCount = countTracks(tracks.Get(k))
			albums = append(albums, e.Album)
			return nil
		})
	})
	return albums, err
}

// Length of the JSON array, without decoding the tracks.
func countTracks(raw []byte) int {
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) != nil {
		return 0
	}
	return len(list)
}

func (s *Store) Get(id int64) (*Album, error) {
	var out *Album
	err := s.db.View(func(tx *bbolt.Tx) error {
		key := tx.Bucket(bIDs).Get(itob(id))
		if key == nil {
			return nil
		}
		var e entry
		if err := json.Unmarshal(tx.Bucket(bAlbums).Get(key), &e); err != nil {
			return err
		}
		var list []library.Track
		if raw := tx.Bucket(bTracks).Get(key); raw != nil {
			if err := json.Unmarshal(raw, &list); err != nil {
				return err
			}
		}
		e.Album.Tracks = list
		e.Album.TrackCount = len(list)
		out = &e.Album
		return nil
	})
	return out, err
}

func (s *Store) SetStatus(id int64, status string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		key := tx.Bucket(bIDs).Get(itob(id))
		if key == nil {
			return nil
		}
		all := tx.Bucket(bAlbums)
		var e entry
		if err := json.Unmarshal(all.Get(key), &e); err != nil {
			return err
		}
		e.Album.Status = status
		return putJSON(all, key, e)
	})
}

func (s *Store) KnownTracks() (map[string]library.Track, error) {
	known := map[string]library.Track{}
	err := s.db.View(func(tx *bbolt.Tx) error {
		return tx.Bucket(bTracks).ForEach(func(_, raw []byte) error {
			var list []library.Track
			if err := json.Unmarshal(raw, &list); err != nil {
				return err
			}
			for _, t := range list {
				known[t.RelPath] = t
			}
			return nil
		})
	})
	return known, err
}

func (s *Store) AlbumsToPrefetch(keyStart, keyEnd, coolStart string, limit int) (albums []Album, total, remaining int, err error) {
	err = s.db.View(func(tx *bbolt.Tx) error {
		expiries, tracks := tx.Bucket(bCacheExp), tx.Bucket(bTracks)
		now := time.Now()

		fresh := func(key string) bool {
			v := expiries.Get([]byte(key))
			return v != nil && now.Before(unixNano(v))
		}

		return tx.Bucket(bAlbums).ForEach(func(k, raw []byte) error {
			var e entry
			if err := json.Unmarshal(raw, &e); err != nil {
				return err
			}
			if e.Album.Status != StatusPending {
				return nil
			}
			total++

			id := strconv.FormatInt(e.Album.ID, 10)
			if fresh(keyStart + id + keyEnd) {
				return nil
			}
			remaining++
			if fresh(coolStart + id) {
				return nil
			}
			if len(albums) < limit {
				e.Album.TrackCount = countTracks(tracks.Get(k))
				albums = append(albums, e.Album)
			}
			return nil
		})
	})
	return albums, total, remaining, err
}

func unixNano(v []byte) time.Time {
	return time.Unix(0, int64(binary.BigEndian.Uint64(v)))
}

func (s *Store) CacheFresh(key string) bool {
	fresh := false
	s.db.View(func(tx *bbolt.Tx) error {
		expiry := tx.Bucket(bCacheExp).Get([]byte(key))
		fresh = expiry != nil && time.Now().Before(unixNano(expiry))
		return nil
	})
	return fresh
}

func (s *Store) CacheGet(key string) ([]byte, bool) {
	var data []byte
	s.db.View(func(tx *bbolt.Tx) error {
		expiry := tx.Bucket(bCacheExp).Get([]byte(key))
		if expiry == nil || !time.Now().Before(unixNano(expiry)) {
			return nil
		}
		if v := tx.Bucket(bCache).Get([]byte(key)); v != nil {
			data = append([]byte(nil), v...)
		}
		return nil
	})
	return data, data != nil
}

func (s *Store) CachePut(key string, data []byte, ttl time.Duration) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		if err := tx.Bucket(bCache).Put([]byte(key), data); err != nil {
			return err
		}
		return tx.Bucket(bCacheExp).Put([]byte(key), itob(time.Now().Add(ttl).UnixNano()))
	})
}

func (s *Store) CachePurge() error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		now := time.Now()
		var stale [][]byte
		tx.Bucket(bCacheExp).ForEach(func(k, v []byte) error {
			if !now.Before(unixNano(v)) {
				stale = append(stale, append([]byte(nil), k...))
			}
			return nil
		})
		for _, k := range stale {
			if err := tx.Bucket(bCache).Delete(k); err != nil {
				return err
			}
			if err := tx.Bucket(bCacheExp).Delete(k); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) ClearCache() (int, error) {
	n := 0
	err := s.db.Update(func(tx *bbolt.Tx) error {
		n = tx.Bucket(bCacheExp).Stats().KeyN
		for _, name := range [][]byte{bCache, bCacheExp} {
			if err := tx.DeleteBucket(name); err != nil {
				return err
			}
			if _, err := tx.CreateBucket(name); err != nil {
				return err
			}
		}
		return nil
	})
	return n, err
}

func (s *Store) CacheSize() int {
	n := 0
	s.db.View(func(tx *bbolt.Tx) error {
		now := time.Now()
		return tx.Bucket(bCacheExp).ForEach(func(_, v []byte) error {
			if now.Before(unixNano(v)) {
				n++
			}
			return nil
		})
	})
	return n
}

func (s *Store) Setting(key string) (string, bool) {
	var value string
	found := false
	s.db.View(func(tx *bbolt.Tx) error {
		if v := tx.Bucket(bSettings).Get([]byte(key)); v != nil {
			value, found = string(v), true
		}
		return nil
	})
	return value, found
}

func (s *Store) PutSetting(key, value string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(bSettings).Put([]byte(key), []byte(value))
	})
}

func (s *Store) DeleteSetting(key string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(bSettings).Delete([]byte(key))
	})
}

// Kept once an album leaves the queue. Keyed by instant, so already in order.
type Filed struct {
	Date        time.Time `json:"date"`
	Artist      string    `json:"artist"`
	Album       string    `json:"album"`
	Year        int       `json:"year"`
	Tracks      int       `json:"tracks"`
	Destination string    `json:"destination"`
}

func (s *Store) RecordFiled(f Filed) error {
	if f.Date.IsZero() {
		f.Date = time.Now().UTC()
	}
	return s.db.Update(func(tx *bbolt.Tx) error {
		return putJSON(tx.Bucket(bFiled), itob(f.Date.UnixNano()), f)
	})
}

// Newest first, with how many there are in all.
func (s *Store) FiledLog(limit int) ([]Filed, int, error) {
	out := []Filed{}
	total := 0
	err := s.db.View(func(tx *bbolt.Tx) error {
		total = tx.Bucket(bFiled).Stats().KeyN
		cursor := tx.Bucket(bFiled).Cursor()
		for k, raw := cursor.Last(); k != nil; k, raw = cursor.Prev() {
			if limit > 0 && len(out) == limit {
				return nil
			}
			var f Filed
			if err := json.Unmarshal(raw, &f); err != nil {
				return err
			}
			out = append(out, f)
		}
		return nil
	})
	return out, total, err
}
