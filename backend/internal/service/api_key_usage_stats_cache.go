package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

const (
	apiKeyUsageStatsCacheTTL     = 30 * time.Second
	apiKeyUsageStatsCacheTimeout = 500 * time.Millisecond
	apiKeyUsageStatsFetchTimeout = 25 * time.Second
	apiKeyUsageStatsCacheVersion = 1

	apiKeyUsageStatsCacheFailureBackoff = 5 * time.Second
	apiKeyUsageStatsL1Capacity          = 256
	apiKeyUsageStatsMaxConcurrentFetch  = 2
)

// ErrAPIKeyUsageStatsCacheMiss marks a normal shared-cache miss.
var ErrAPIKeyUsageStatsCacheMiss = errors.New("api key usage stats cache miss")

type apiKeyUsageStatsCacheReadStatus uint8

const (
	apiKeyUsageStatsCacheReadMiss apiKeyUsageStatsCacheReadStatus = iota
	apiKeyUsageStatsCacheReadHit
	apiKeyUsageStatsCacheReadUnavailable
)

// APIKeyUsageStatsCache is an optional Redis-backed cache implemented by the
// production usage cache. Keeping it separate from UsageRankingCache means
// alternate ranking-cache implementations remain source compatible.
type APIKeyUsageStatsCache interface {
	GetAPIKeyUsageStats(ctx context.Context, key string) (string, error)
	SetAPIKeyUsageStats(ctx context.Context, key string, data string, ttl time.Duration) error
	DeleteAPIKeyUsageStats(ctx context.Context, key string) error
}

type apiKeyUsageStatsCacheEntry struct {
	Version  int                                         `json:"version"`
	UserID   int64                                       `json:"user_id"`
	Timezone string                                      `json:"timezone"`
	Day      string                                      `json:"day"`
	Stats    map[int64]*usagestats.BatchAPIKeyUsageStats `json:"stats"`
}

type apiKeyUsageStatsL1Entry struct {
	stats      map[int64]*usagestats.BatchAPIKeyUsageStats
	expiresAt  time.Time
	lastAccess uint64
}

type apiKeyUsageStatsCacheState struct {
	mu sync.Mutex

	l1         map[string]apiKeyUsageStatsL1Entry
	l1Sequence uint64
	fetchGate  chan struct{}
	flights    map[string]*apiKeyUsageStatsFlight

	cacheReadUnavailableUntil  time.Time
	cacheWriteUnavailableUntil time.Time
}

// apiKeyUsageStatsFlight coalesces one user-scoped key set while still making
// the shared PostgreSQL query cancellable. A plain singleflight call would
// either inherit the first caller's cancellation (breaking the other waiters)
// or detach the query and leave it running after every HTTP request is gone.
type apiKeyUsageStatsFlight struct {
	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}
	waiters  int
	complete bool
	stats    map[int64]*usagestats.BatchAPIKeyUsageStats
	err      error
}

func (s *apiKeyUsageStatsCacheState) doFetch(
	ctx context.Context,
	key string,
	fn func(context.Context) (map[int64]*usagestats.BatchAPIKeyUsageStats, error),
) (map[int64]*usagestats.BatchAPIKeyUsageStats, error) {
	s.mu.Lock()
	if s.flights == nil {
		s.flights = make(map[string]*apiKeyUsageStatsFlight)
	}
	flight := s.flights[key]
	if flight == nil {
		flightCtx, cancel := context.WithCancel(context.Background())
		flight = &apiKeyUsageStatsFlight{
			ctx:     flightCtx,
			cancel:  cancel,
			done:    make(chan struct{}),
			waiters: 1,
		}
		s.flights[key] = flight
		go s.runFetch(key, flight, fn)
	} else {
		flight.waiters++
	}
	s.mu.Unlock()

	select {
	case <-ctx.Done():
		s.leaveFetch(key, flight)
		return nil, ctx.Err()
	case <-flight.done:
		s.leaveFetch(key, flight)
		return cloneAPIKeyUsageStats(flight.stats), flight.err
	}
}

func (s *apiKeyUsageStatsCacheState) runFetch(
	key string,
	flight *apiKeyUsageStatsFlight,
	fn func(context.Context) (map[int64]*usagestats.BatchAPIKeyUsageStats, error),
) {
	stats, err := fn(flight.ctx)

	s.mu.Lock()
	flight.stats = stats
	flight.err = err
	flight.complete = true
	if s.flights[key] == flight {
		delete(s.flights, key)
	}
	close(flight.done)
	s.mu.Unlock()
	flight.cancel()
}

func (s *apiKeyUsageStatsCacheState) leaveFetch(key string, flight *apiKeyUsageStatsFlight) {
	s.mu.Lock()
	flight.waiters--
	shouldCancel := flight.waiters == 0 && !flight.complete
	if shouldCancel && s.flights[key] == flight {
		delete(s.flights, key)
	}
	s.mu.Unlock()
	if shouldCancel {
		flight.cancel()
	}
}

func cloneAPIKeyUsageStats(
	stats map[int64]*usagestats.BatchAPIKeyUsageStats,
) map[int64]*usagestats.BatchAPIKeyUsageStats {
	if stats == nil {
		return nil
	}
	cloned := make(map[int64]*usagestats.BatchAPIKeyUsageStats, len(stats))
	for id, stat := range stats {
		if stat == nil {
			cloned[id] = nil
			continue
		}
		value := *stat
		cloned[id] = &value
	}
	return cloned
}

func (s *apiKeyUsageStatsCacheState) loadL1(
	key string,
	now time.Time,
) (map[int64]*usagestats.BatchAPIKeyUsageStats, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneExpiredL1Locked(now)

	entry, ok := s.l1[key]
	if !ok {
		return nil, false
	}
	s.l1Sequence++
	entry.lastAccess = s.l1Sequence
	s.l1[key] = entry
	return cloneAPIKeyUsageStats(entry.stats), true
}

func (s *apiKeyUsageStatsCacheState) storeL1(
	key string,
	stats map[int64]*usagestats.BatchAPIKeyUsageStats,
	now time.Time,
) {
	if key == "" || stats == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.l1 == nil {
		s.l1 = make(map[string]apiKeyUsageStatsL1Entry, apiKeyUsageStatsL1Capacity)
	}
	s.pruneExpiredL1Locked(now)
	if _, exists := s.l1[key]; !exists && len(s.l1) >= apiKeyUsageStatsL1Capacity {
		var oldestKey string
		var oldestAccess uint64
		for candidateKey, entry := range s.l1 {
			if oldestKey == "" || entry.lastAccess < oldestAccess {
				oldestKey = candidateKey
				oldestAccess = entry.lastAccess
			}
		}
		delete(s.l1, oldestKey)
	}
	s.l1Sequence++
	s.l1[key] = apiKeyUsageStatsL1Entry{
		stats:      cloneAPIKeyUsageStats(stats),
		expiresAt:  now.Add(apiKeyUsageStatsCacheTTL),
		lastAccess: s.l1Sequence,
	}
}

func (s *apiKeyUsageStatsCacheState) pruneExpiredL1Locked(now time.Time) {
	for key, entry := range s.l1 {
		if !now.Before(entry.expiresAt) {
			delete(s.l1, key)
		}
	}
}

func (s *apiKeyUsageStatsCacheState) acquireFetchSlot(ctx context.Context) bool {
	s.mu.Lock()
	if s.fetchGate == nil {
		s.fetchGate = make(chan struct{}, apiKeyUsageStatsMaxConcurrentFetch)
	}
	gate := s.fetchGate
	s.mu.Unlock()

	select {
	case gate <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

func (s *apiKeyUsageStatsCacheState) releaseFetchSlot() {
	s.mu.Lock()
	gate := s.fetchGate
	s.mu.Unlock()
	if gate != nil {
		<-gate
	}
}

func (s *apiKeyUsageStatsCacheState) shouldBackOffCacheRead(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return now.Before(s.cacheReadUnavailableUntil)
}

func (s *apiKeyUsageStatsCacheState) shouldBackOffCacheWrite(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return now.Before(s.cacheWriteUnavailableUntil)
}

func (s *apiKeyUsageStatsCacheState) markCacheReadUnavailable(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	wasUnavailable := now.Before(s.cacheReadUnavailableUntil)
	s.cacheReadUnavailableUntil = now.Add(apiKeyUsageStatsCacheFailureBackoff)
	return !wasUnavailable
}

func (s *apiKeyUsageStatsCacheState) markCacheWriteUnavailable(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	wasUnavailable := now.Before(s.cacheWriteUnavailableUntil)
	s.cacheWriteUnavailableUntil = now.Add(apiKeyUsageStatsCacheFailureBackoff)
	return !wasUnavailable
}

func (s *apiKeyUsageStatsCacheState) markCacheReadAvailable() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cacheReadUnavailableUntil = time.Time{}
}

func (s *apiKeyUsageStatsCacheState) markCacheWriteAvailable() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cacheWriteUnavailableUntil = time.Time{}
}

func normalizeAPIKeyUsageStatsIDs(ids []int64) []int64 {
	if len(ids) == 0 {
		return nil
	}
	normalized := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		normalized = append(normalized, id)
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i] < normalized[j] })
	return normalized
}

func apiKeyUsageStatsCacheKey(userID int64, timezoneName string, day string, ids []int64) string {
	var identity strings.Builder
	identity.Grow(len(timezoneName) + len(day) + len(ids)*12 + 24)
	identity.WriteString(strconv.FormatInt(userID, 10))
	identity.WriteByte('|')
	identity.WriteString(timezoneName)
	identity.WriteByte('|')
	identity.WriteString(day)
	for _, id := range ids {
		identity.WriteByte('|')
		identity.WriteString(strconv.FormatInt(id, 10))
	}
	sum := sha256.Sum256([]byte(identity.String()))
	return hex.EncodeToString(sum[:])
}

func (s *UsageService) getBatchAPIKeyUsageStats(
	ctx context.Context,
	cacheScopeUserID int64,
	apiKeyIDs []int64,
	startTime time.Time,
	endTime time.Time,
) (map[int64]*usagestats.BatchAPIKeyUsageStats, error) {
	normalizedIDs := normalizeAPIKeyUsageStatsIDs(apiKeyIDs)
	if len(normalizedIDs) == 0 {
		return map[int64]*usagestats.BatchAPIKeyUsageStats{}, nil
	}
	if s == nil || s.usageRepo == nil {
		return nil, errors.New("get batch api key usage stats: repository is unavailable")
	}

	// Explicit ranges are caller-defined analytical queries. Only the fixed
	// dashboard window is safe to share under the short-lived cache key.
	if !startTime.IsZero() || !endTime.IsZero() {
		stats, err := s.fetchBatchAPIKeyUsageStats(ctx, normalizedIDs, startTime, endTime)
		return stats, normalizeAPIKeyUsageStatsCallerError(err)
	}

	timezoneName := timezone.Location().String()
	dayStartUTC := timezone.Today().UTC().Format(time.RFC3339)
	cacheKey := apiKeyUsageStatsCacheKey(cacheScopeUserID, timezoneName, dayStartUTC, normalizedIDs)
	cache, cacheEnabled := s.rankingCache.(APIKeyUsageStatsCache)
	if cacheEnabled {
		if stats, ok := s.apiKeyUsageStatsState.loadL1(cacheKey, time.Now()); ok &&
			validAPIKeyUsageStats(stats, normalizedIDs) {
			return stats, nil
		}
		if !s.apiKeyUsageStatsState.shouldBackOffCacheRead(time.Now()) {
			stats, status := s.loadAPIKeyUsageStatsCache(
				ctx,
				cache,
				cacheKey,
				cacheScopeUserID,
				timezoneName,
				dayStartUTC,
				normalizedIDs,
			)
			if status == apiKeyUsageStatsCacheReadHit {
				s.apiKeyUsageStatsState.storeL1(cacheKey, stats, time.Now())
				return stats, nil
			}
		}
	}

	// Coalesce the exact user/key-set before entering the bounded global gate.
	// Two unrelated users can make progress concurrently, while equivalent
	// cache misses still execute one PostgreSQL aggregation. The flight owns a
	// shared cancellable context and stops the query once its last waiter leaves.
	stats, err := s.apiKeyUsageStatsState.doFetch(ctx, cacheKey, func(flightCtx context.Context) (
		map[int64]*usagestats.BatchAPIKeyUsageStats,
		error,
	) {
		return s.refreshAPIKeyUsageStats(
			flightCtx,
			cacheScopeUserID,
			normalizedIDs,
			cache,
			cacheEnabled,
			cacheKey,
			timezoneName,
			dayStartUTC,
		)
	})
	return stats, normalizeAPIKeyUsageStatsCallerError(err)
}

func normalizeAPIKeyUsageStatsCallerError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) && !infraerrors.IsGatewayTimeout(err) {
		return infraerrors.GatewayTimeout(
			"API_KEY_USAGE_STATS_REQUEST_TIMEOUT",
			"API key usage statistics request timed out",
		).WithCause(err)
	}
	return err
}

func (s *UsageService) refreshAPIKeyUsageStats(
	ctx context.Context,
	cacheScopeUserID int64,
	normalizedIDs []int64,
	cache APIKeyUsageStatsCache,
	cacheEnabled bool,
	cacheKey string,
	timezoneName string,
	dayStartUTC string,
) (map[int64]*usagestats.BatchAPIKeyUsageStats, error) {
	if !s.apiKeyUsageStatsState.acquireFetchSlot(ctx) {
		return nil, ctx.Err()
	}
	fetchSlotHeld := true
	defer func() {
		if fetchSlotHeld {
			s.apiKeyUsageStatsState.releaseFetchSlot()
		}
	}()

	// A previous waiter may have populated L1 or Redis while this request was
	// queued behind the aggregate gate.
	if cacheEnabled {
		if stats, ok := s.apiKeyUsageStatsState.loadL1(cacheKey, time.Now()); ok &&
			validAPIKeyUsageStats(stats, normalizedIDs) {
			return stats, nil
		}
		if !s.apiKeyUsageStatsState.shouldBackOffCacheRead(time.Now()) {
			stats, status := s.loadAPIKeyUsageStatsCache(
				ctx,
				cache,
				cacheKey,
				cacheScopeUserID,
				timezoneName,
				dayStartUTC,
				normalizedIDs,
			)
			if status == apiKeyUsageStatsCacheReadHit {
				s.apiKeyUsageStatsState.storeL1(cacheKey, stats, time.Now())
				return stats, nil
			}
		}
	}

	stats, err := s.fetchBatchAPIKeyUsageStats(ctx, normalizedIDs, time.Time{}, time.Time{})
	if err != nil {
		return nil, err
	}
	if !validAPIKeyUsageStats(stats, normalizedIDs) {
		return nil, errors.New("get batch api key usage stats: repository returned invalid result")
	}
	if cacheEnabled {
		s.apiKeyUsageStatsState.storeL1(cacheKey, stats, time.Now())
	}

	// The gate protects PostgreSQL, not Redis. Release it as soon as the valid
	// result is available in L1 so unrelated key sets do not wait behind a
	// best-effort cache write.
	s.apiKeyUsageStatsState.releaseFetchSlot()
	fetchSlotHeld = false
	if cacheEnabled {
		s.storeAPIKeyUsageStatsCache(
			ctx,
			cache,
			cacheKey,
			cacheScopeUserID,
			timezoneName,
			dayStartUTC,
			stats,
		)
	}
	return stats, nil
}

func (s *UsageService) fetchBatchAPIKeyUsageStats(
	ctx context.Context,
	apiKeyIDs []int64,
	startTime time.Time,
	endTime time.Time,
) (map[int64]*usagestats.BatchAPIKeyUsageStats, error) {
	queryCtx, cancel := context.WithTimeout(ctx, apiKeyUsageStatsFetchTimeout)
	defer cancel()
	stats, err := s.usageRepo.GetBatchAPIKeyUsageStats(queryCtx, apiKeyIDs, startTime, endTime)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, infraerrors.GatewayTimeout(
				"API_KEY_USAGE_STATS_TIMEOUT",
				"API key usage statistics query timed out",
			).WithCause(err)
		}
		return nil, fmt.Errorf("get batch api key usage stats: %w", err)
	}
	return stats, nil
}

func (s *UsageService) loadAPIKeyUsageStatsCache(
	ctx context.Context,
	cache APIKeyUsageStatsCache,
	key string,
	userID int64,
	timezoneName string,
	day string,
	apiKeyIDs []int64,
) (map[int64]*usagestats.BatchAPIKeyUsageStats, apiKeyUsageStatsCacheReadStatus) {
	cacheCtx, cancel := context.WithTimeout(ctx, apiKeyUsageStatsCacheTimeout)
	defer cancel()
	data, err := cache.GetAPIKeyUsageStats(cacheCtx, key)
	if err != nil {
		if !errors.Is(err, ErrAPIKeyUsageStatsCacheMiss) {
			if ctx.Err() == nil && s.apiKeyUsageStatsState.markCacheReadUnavailable(time.Now()) {
				slog.Warn("failed to read api key usage stats cache", "error", err)
			}
			return nil, apiKeyUsageStatsCacheReadUnavailable
		}
		s.apiKeyUsageStatsState.markCacheReadAvailable()
		return nil, apiKeyUsageStatsCacheReadMiss
	}
	s.apiKeyUsageStatsState.markCacheReadAvailable()

	var entry apiKeyUsageStatsCacheEntry
	if err := json.Unmarshal([]byte(data), &entry); err != nil ||
		entry.Version != apiKeyUsageStatsCacheVersion ||
		entry.UserID != userID ||
		entry.Timezone != timezoneName ||
		entry.Day != day ||
		!validAPIKeyUsageStats(entry.Stats, apiKeyIDs) {
		s.deleteAPIKeyUsageStatsCache(ctx, cache, key)
		return nil, apiKeyUsageStatsCacheReadMiss
	}
	return entry.Stats, apiKeyUsageStatsCacheReadHit
}

func (s *UsageService) storeAPIKeyUsageStatsCache(
	ctx context.Context,
	cache APIKeyUsageStatsCache,
	key string,
	userID int64,
	timezoneName string,
	day string,
	stats map[int64]*usagestats.BatchAPIKeyUsageStats,
) {
	entry := apiKeyUsageStatsCacheEntry{
		Version:  apiKeyUsageStatsCacheVersion,
		UserID:   userID,
		Timezone: timezoneName,
		Day:      day,
		Stats:    stats,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		slog.Warn("failed to encode api key usage stats cache", "error", err)
		return
	}
	if s.apiKeyUsageStatsState.shouldBackOffCacheWrite(time.Now()) {
		return
	}

	cacheCtx, cancel := context.WithTimeout(ctx, apiKeyUsageStatsCacheTimeout)
	defer cancel()
	if err := cache.SetAPIKeyUsageStats(cacheCtx, key, string(data), apiKeyUsageStatsCacheTTL); err != nil {
		if ctx.Err() == nil && s.apiKeyUsageStatsState.markCacheWriteUnavailable(time.Now()) {
			slog.Warn("failed to store api key usage stats cache", "error", err)
		}
		return
	}
	s.apiKeyUsageStatsState.markCacheWriteAvailable()
}

func (s *UsageService) deleteAPIKeyUsageStatsCache(
	ctx context.Context,
	cache APIKeyUsageStatsCache,
	key string,
) {
	if s.apiKeyUsageStatsState.shouldBackOffCacheWrite(time.Now()) {
		return
	}
	cacheCtx, cancel := context.WithTimeout(ctx, apiKeyUsageStatsCacheTimeout)
	defer cancel()
	if err := cache.DeleteAPIKeyUsageStats(cacheCtx, key); err != nil {
		if ctx.Err() == nil && s.apiKeyUsageStatsState.markCacheWriteUnavailable(time.Now()) {
			slog.Warn("failed to delete api key usage stats cache", "error", err)
		}
		return
	}
	s.apiKeyUsageStatsState.markCacheWriteAvailable()
}

func validAPIKeyUsageStats(
	stats map[int64]*usagestats.BatchAPIKeyUsageStats,
	apiKeyIDs []int64,
) bool {
	if len(stats) != len(apiKeyIDs) {
		return false
	}
	for _, id := range apiKeyIDs {
		stat, ok := stats[id]
		if !ok || stat == nil || stat.APIKeyID != id {
			return false
		}
	}
	return true
}
