package server

import (
	"errors"
	"io/fs"
	"log"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/quad341/hold-court/internal/feed"
)

// feedCache re-scans a feed directory at most once per interval, or
// immediately after fsnotify observes a filesystem change — so handleIndex
// never re-parses and re-renders every hold's markdown on every request,
// while new or changed holds still show up promptly (DESIGN.md: "re-scans
// on fsnotify + interval").
type feedCache struct {
	dir      string
	interval time.Duration

	mu       sync.Mutex
	holds    []*feed.Hold
	err      error
	lastScan time.Time
	dirty    bool
}

func newFeedCache(dir string, interval time.Duration) *feedCache {
	c := &feedCache{dir: dir, interval: interval, dirty: true}
	c.rescan()
	return c
}

// rescan treats a feed dir that does not exist yet as empty rather than an
// error: the adapter that owns the directory may not have run yet, and the
// interval rescan (or a later fsnotify event, once the directory exists) is
// what's expected to pick it up once it does.
func (c *feedCache) rescan() {
	holds, err := feed.ScanDir(c.dir)
	if err != nil && errors.Is(err, fs.ErrNotExist) {
		holds, err = nil, nil
	}

	c.mu.Lock()
	c.holds, c.err, c.lastScan, c.dirty = holds, err, time.Now(), false
	c.mu.Unlock()
}

func (c *feedCache) markDirty() {
	c.mu.Lock()
	c.dirty = true
	c.mu.Unlock()
}

// snapshot returns the current holds, re-scanning first if the cache was
// marked dirty (an fsnotify event fired) or the interval has elapsed.
func (c *feedCache) snapshot() ([]*feed.Hold, error) {
	c.mu.Lock()
	stale := c.dirty || time.Since(c.lastScan) >= c.interval
	c.mu.Unlock()

	if stale {
		c.rescan()
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	return c.holds, c.err
}

// watch attaches an fsnotify watch on the cache's directory and marks the
// cache dirty on any event, for as long as the server process runs. If the
// directory can't be watched (most commonly: it doesn't exist yet), watch
// logs and returns — the interval rescan in snapshot still converges once
// the directory appears, fsnotify just stops being able to shortcut the
// wait.
func (c *feedCache) watch() {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("hold-court: fsnotify unavailable, falling back to interval-only rescan: %v", err)
		return
	}
	defer w.Close()

	if err := w.Add(c.dir); err != nil {
		log.Printf("hold-court: watch %s: %v (falling back to interval-only rescan)", c.dir, err)
		return
	}

	for {
		select {
		case _, ok := <-w.Events:
			if !ok {
				return
			}
			c.markDirty()
		case _, ok := <-w.Errors:
			if !ok {
				return
			}
		}
	}
}
