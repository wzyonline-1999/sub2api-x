package admin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

type snapshotCacheEntry struct {
	ETag      string
	Payload   any
	ExpiresAt time.Time
}

type snapshotCache struct {
	mu          sync.RWMutex
	ttl         time.Duration
	loadTimeout time.Duration
	maxEntries  int
	items       map[string]snapshotCacheEntry
	flights     map[string]*snapshotCacheFlight
}

// snapshotCacheFlight coalesces one cache miss while at least one caller is
// still waiting. Its loader context is deliberately independent from the
// first HTTP request: one disconnected client must not cancel the shared SQL
// query for the remaining callers. The final departing waiter cancels it.
type snapshotCacheFlight struct {
	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}
	waiters  int
	complete bool
	entry    snapshotCacheEntry
	err      error
}

type snapshotCacheLoaderResult struct {
	payload any
	err     error
}

const (
	defaultSnapshotCacheMaxEntries  = 256
	defaultSnapshotCacheLoadTimeout = 30 * time.Second
)

func newSnapshotCache(ttl time.Duration) *snapshotCache {
	return newSnapshotCacheWithOptions(ttl, defaultSnapshotCacheLoadTimeout, defaultSnapshotCacheMaxEntries)
}

func newSnapshotCacheWithCapacity(ttl time.Duration, maxEntries int) *snapshotCache {
	return newSnapshotCacheWithOptions(ttl, defaultSnapshotCacheLoadTimeout, maxEntries)
}

func newSnapshotCacheWithLoadTimeout(ttl, loadTimeout time.Duration) *snapshotCache {
	return newSnapshotCacheWithOptions(ttl, loadTimeout, defaultSnapshotCacheMaxEntries)
}

func newSnapshotCacheWithOptions(ttl, loadTimeout time.Duration, maxEntries int) *snapshotCache {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	if loadTimeout <= 0 {
		loadTimeout = defaultSnapshotCacheLoadTimeout
	}
	if maxEntries <= 0 {
		maxEntries = defaultSnapshotCacheMaxEntries
	}
	return &snapshotCache{
		ttl:         ttl,
		loadTimeout: loadTimeout,
		maxEntries:  maxEntries,
		items:       make(map[string]snapshotCacheEntry),
		flights:     make(map[string]*snapshotCacheFlight),
	}
}

func (c *snapshotCache) Get(key string) (snapshotCacheEntry, bool) {
	if c == nil || key == "" {
		return snapshotCacheEntry{}, false
	}
	now := time.Now()

	c.mu.RLock()
	entry, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		return snapshotCacheEntry{}, false
	}
	if !now.Before(entry.ExpiresAt) {
		c.mu.Lock()
		// Set may have replaced the entry after the read lock was released.
		// Return that replacement when it is fresh; otherwise remove the stale
		// value that is still present.
		if current, exists := c.items[key]; exists {
			if now.Before(current.ExpiresAt) {
				c.mu.Unlock()
				return current, true
			}
			delete(c.items, key)
		}
		c.mu.Unlock()
		return snapshotCacheEntry{}, false
	}
	return entry, true
}

func (c *snapshotCache) Set(key string, payload any) snapshotCacheEntry {
	if c == nil {
		return snapshotCacheEntry{}
	}
	entry := c.newEntry(payload)
	if key == "" {
		return entry
	}
	c.mu.Lock()
	c.pruneLocked(time.Now(), key)
	c.items[key] = entry
	c.mu.Unlock()
	return entry
}

func (c *snapshotCache) newEntry(payload any) snapshotCacheEntry {
	return snapshotCacheEntry{
		ETag:      buildETagFromAny(payload),
		Payload:   payload,
		ExpiresAt: time.Now().Add(c.ttl),
	}
}

// pruneLocked removes expired entries and, when needed, the entry that will
// expire first. Keeping the cache bounded matters for query-shaped keys: even
// short-lived one-off dashboard filters must not grow this process-local map
// without limit.
func (c *snapshotCache) pruneLocked(now time.Time, incomingKey string) {
	for key, entry := range c.items {
		if !now.Before(entry.ExpiresAt) {
			delete(c.items, key)
		}
	}

	if _, replacing := c.items[incomingKey]; replacing || len(c.items) < c.maxEntries {
		return
	}

	var oldestKey string
	var oldestExpiry time.Time
	for key, entry := range c.items {
		if oldestKey == "" || entry.ExpiresAt.Before(oldestExpiry) {
			oldestKey = key
			oldestExpiry = entry.ExpiresAt
		}
	}
	if oldestKey != "" {
		delete(c.items, oldestKey)
	}
}

func (c *snapshotCache) GetOrLoad(
	ctx context.Context,
	key string,
	load func(context.Context) (any, error),
) (snapshotCacheEntry, bool, error) {
	if load == nil {
		return snapshotCacheEntry{}, false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if entry, ok := c.Get(key); ok {
		return entry, true, nil
	}
	if c == nil || key == "" {
		loadTimeout := defaultSnapshotCacheLoadTimeout
		if c != nil {
			loadTimeout = c.loadTimeout
		}
		loadCtx, cancel := context.WithTimeout(ctx, loadTimeout)
		defer cancel()
		payload, err := runSnapshotCacheLoader(loadCtx, load)
		if err != nil {
			return snapshotCacheEntry{}, false, err
		}
		return c.Set(key, payload), false, nil
	}

	c.mu.Lock()
	// Close the race between the optimistic Get above and joining/creating a
	// flight. A completed loader may have populated the cache in between.
	if entry, ok := c.items[key]; ok && time.Now().Before(entry.ExpiresAt) {
		c.mu.Unlock()
		return entry, true, nil
	}

	flight := c.flights[key]
	if flight == nil {
		// Preserve request-scoped values (for example the selected timezone),
		// while detaching cancellation and deadlines from the first waiter.
		flightBaseCtx := context.WithoutCancel(ctx)
		flightCtx, cancel := context.WithTimeout(flightBaseCtx, c.loadTimeout)
		flight = &snapshotCacheFlight{
			ctx:     flightCtx,
			cancel:  cancel,
			done:    make(chan struct{}),
			waiters: 1,
		}
		c.flights[key] = flight
		go c.runLoad(key, flight, load)
	} else {
		flight.waiters++
	}
	c.mu.Unlock()

	select {
	case <-flight.done:
		c.leaveFlight(key, flight)
		return flight.entry, false, flight.err
	case <-ctx.Done():
		c.leaveFlight(key, flight)
		return snapshotCacheEntry{}, false, ctx.Err()
	}
}

func (c *snapshotCache) runLoad(
	key string,
	flight *snapshotCacheFlight,
	load func(context.Context) (any, error),
) {
	payload, err := runSnapshotCacheLoader(flight.ctx, load)
	entry := snapshotCacheEntry{}
	if err == nil {
		entry = c.newEntry(payload)
	}

	c.mu.Lock()
	active := c.flights[key] == flight
	// An abandoned flight must not repopulate the cache after its final waiter
	// left and a replacement refresh may already be running.
	if err == nil && active && flight.waiters > 0 {
		c.pruneLocked(time.Now(), key)
		c.items[key] = entry
	}
	flight.entry = entry
	flight.err = err
	flight.complete = true
	if active {
		delete(c.flights, key)
	}
	close(flight.done)
	c.mu.Unlock()
	flight.cancel()
}

func (c *snapshotCache) leaveFlight(key string, flight *snapshotCacheFlight) {
	c.mu.Lock()
	if flight.waiters > 0 {
		flight.waiters--
	}
	shouldCancel := flight.waiters == 0 && !flight.complete
	if shouldCancel && c.flights[key] == flight {
		delete(c.flights, key)
	}
	c.mu.Unlock()
	if shouldCancel {
		flight.cancel()
	}
}

// runSnapshotCacheLoader makes the timeout observable even if a buggy loader
// fails to return after its context is canceled. The result channel is
// buffered so such a loader can finish later without retaining the flight.
func runSnapshotCacheLoader(
	ctx context.Context,
	load func(context.Context) (any, error),
) (any, error) {
	resultCh := make(chan snapshotCacheLoaderResult, 1)
	go func() {
		result := snapshotCacheLoaderResult{}
		defer func() {
			if recovered := recover(); recovered != nil {
				result.err = fmt.Errorf("snapshot cache loader panic: %v", recovered)
			}
			resultCh <- result
		}()
		result.payload, result.err = load(ctx)
	}()

	select {
	case result := <-resultCh:
		return result.payload, result.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func buildETagFromAny(payload any) string {
	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return "\"" + hex.EncodeToString(sum[:]) + "\""
}

func parseBoolQueryWithDefault(raw string, def bool) bool {
	value := strings.TrimSpace(strings.ToLower(raw))
	if value == "" {
		return def
	}
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}
