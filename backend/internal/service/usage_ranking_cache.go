package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

const (
	// Keep the leaderboard responsive without letting a frequently refreshed
	// page repeatedly run the two-period full-table aggregation.
	// The key advances at the next minute boundary. Keep the previous bucket
	// longer than one minute so a request near :00 cannot expire and recompute
	// the same bucket again before that boundary.
	usageRankingCacheTTL = 90 * time.Second

	// A refresh is shared while at least one request is waiting. The repository
	// query has its own bound and is canceled when the final waiter leaves.
	usageRankingFetchTimeout = 60 * time.Second
	usageRankingCacheTimeout = 2 * time.Second

	// The lease only protects the expensive refresh. It is deliberately
	// bounded and carries a safety margin over the database fetch deadline so a
	// timed-out owner cannot leave the leaderboard locked indefinitely.
	usageRankingRefreshLeaseTTL = usageRankingFetchTimeout +
		2*usageRankingCacheTimeout +
		5*time.Second
	usageRankingRefreshPollInterval = 150 * time.Millisecond
	// The lease wait has an independent timer, so a process can recover after one
	// abandoned lease and still give the eventual query its complete fetch
	// budget.
	usageRankingRefreshWaitTimeout = 150 * time.Second
	// A single cross-key lease serializes every expensive leaderboard refresh
	// across blue/green instances. Per-key leases still allow metric/timezone
	// variations to run concurrently and therefore do not bound DB pressure.
	usageRankingGlobalRefreshLeaseKey = "global"

	// L1 serves the exact current minute without repeated Redis decoding and is
	// also the last-known-good fallback during a cache outage. A bounded lifetime
	// and entry count prevent old snapshots from accumulating in long-running
	// instances.
	usageRankingL1StaleTTL = 10 * time.Minute
	usageRankingL1Capacity = 64

	// Avoid paying the Redis timeout on every request during a short outage.
	// The next probe is deliberately soon so recovery is discovered quickly.
	usageRankingCacheFailureBackoff = 5 * time.Second
)

var (
	// ErrUsageRankingCacheMiss marks a normal shared-cache miss.
	ErrUsageRankingCacheMiss          = errors.New("usage ranking cache miss")
	errUsageRankingRefreshWaitTimeout = errors.New("usage ranking refresh lease wait timed out")
)

// UsageRankingCache is the cross-instance cache for shared leaderboard
// snapshots. User-specific fields are derived after a cache hit.
type UsageRankingCache interface {
	GetUsageRanking(ctx context.Context, key string) (string, error)
	SetUsageRanking(ctx context.Context, key string, data string, ttl time.Duration) error
	DeleteUsageRanking(ctx context.Context, key string) error
	TryAcquireUsageRankingRefresh(ctx context.Context, key string, token string, ttl time.Duration) (bool, error)
	ReleaseUsageRankingRefresh(ctx context.Context, key string, token string) error
}

type usageRankingSnapshotRepository interface {
	GetUsageRankingSnapshot(context.Context, usagestats.UsageRankingQuery) (*usagestats.UsageRankingSnapshot, error)
}

type usageRankingCacheState struct {
	mu                         sync.Mutex
	l1                         map[string]usageRankingL1Entry
	l1Sequence                 uint64
	cacheReadUnavailableUntil  time.Time
	cacheWriteUnavailableUntil time.Time
	localRefreshGate           chan struct{}
	flights                    map[string]*usageRankingFlight
}

// usageRankingFlight keeps one refresh shared by equivalent callers without
// orphaning it. The refresh continues while at least one HTTP request waits;
// when the last waiter leaves, its context is canceled and both lease polling
// and the repository query stop promptly.
type usageRankingFlight struct {
	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}
	waiters  int
	complete bool
	snapshot *usagestats.UsageRankingSnapshot
	err      error
}

type usageRankingL1Entry struct {
	snapshot   *usagestats.UsageRankingSnapshot
	exactKey   string
	expiresAt  time.Time
	lastAccess uint64
}

type usageRankingCacheReadStatus uint8

const (
	usageRankingCacheReadMiss usageRankingCacheReadStatus = iota
	usageRankingCacheReadHit
	usageRankingCacheReadUnavailable
)

func (s *usageRankingCacheState) doRefresh(
	ctx context.Context,
	key string,
	fn func(context.Context) (*usagestats.UsageRankingSnapshot, error),
) (*usagestats.UsageRankingSnapshot, error) {
	s.mu.Lock()
	if s.flights == nil {
		s.flights = make(map[string]*usageRankingFlight)
	}
	flight := s.flights[key]
	if flight == nil {
		flightCtx, cancel := context.WithCancel(context.Background())
		flight = &usageRankingFlight{
			ctx:     flightCtx,
			cancel:  cancel,
			done:    make(chan struct{}),
			waiters: 1,
		}
		s.flights[key] = flight
		go s.runRefresh(key, flight, fn)
	} else {
		flight.waiters++
	}
	s.mu.Unlock()

	select {
	case <-ctx.Done():
		s.leaveRefresh(key, flight)
		return nil, ctx.Err()
	case <-flight.done:
		s.leaveRefresh(key, flight)
		return flight.snapshot, flight.err
	}
}

func (s *usageRankingCacheState) runRefresh(
	key string,
	flight *usageRankingFlight,
	fn func(context.Context) (*usagestats.UsageRankingSnapshot, error),
) {
	snapshot, err := fn(flight.ctx)

	s.mu.Lock()
	flight.snapshot = snapshot
	flight.err = err
	flight.complete = true
	if s.flights[key] == flight {
		delete(s.flights, key)
	}
	close(flight.done)
	s.mu.Unlock()
	flight.cancel()
}

func (s *usageRankingCacheState) leaveRefresh(key string, flight *usageRankingFlight) {
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

func (s *usageRankingCacheState) loadL1(key string, now time.Time) (*usagestats.UsageRankingSnapshot, bool) {
	entry, ok := s.loadL1Entry(key, now)
	if !ok {
		return nil, false
	}
	return entry.snapshot, true
}

func (s *usageRankingCacheState) loadFreshL1(
	key string,
	exactKey string,
	now time.Time,
) (*usagestats.UsageRankingSnapshot, bool) {
	entry, ok := s.loadL1Entry(key, now)
	if !ok || entry.exactKey != exactKey {
		return nil, false
	}
	return entry.snapshot, true
}

func (s *usageRankingCacheState) loadL1Entry(key string, now time.Time) (usageRankingL1Entry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneExpiredL1Locked(now)

	entry, ok := s.l1[key]
	if !ok {
		return usageRankingL1Entry{}, false
	}
	s.l1Sequence++
	entry.lastAccess = s.l1Sequence
	s.l1[key] = entry
	return entry, true
}

func (s *usageRankingCacheState) storeL1(
	key string,
	exactKey string,
	snapshot *usagestats.UsageRankingSnapshot,
	now time.Time,
) {
	if key == "" || exactKey == "" || !isValidUsageRankingSnapshot(snapshot) {
		return
	}
	usagestats.PrepareUsageRankingSnapshot(snapshot)

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.l1 == nil {
		s.l1 = make(map[string]usageRankingL1Entry, usageRankingL1Capacity)
	}
	s.pruneExpiredL1Locked(now)
	if _, exists := s.l1[key]; !exists && len(s.l1) >= usageRankingL1Capacity {
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
	s.l1[key] = usageRankingL1Entry{
		snapshot:   snapshot,
		exactKey:   exactKey,
		expiresAt:  now.Add(usageRankingL1StaleTTL),
		lastAccess: s.l1Sequence,
	}
}

func (s *usageRankingCacheState) pruneExpiredL1Locked(now time.Time) {
	for key, entry := range s.l1 {
		if !now.Before(entry.expiresAt) {
			delete(s.l1, key)
		}
	}
}

func (s *usageRankingCacheState) markCacheReadUnavailable(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	until := now.Add(usageRankingCacheFailureBackoff)
	if until.After(s.cacheReadUnavailableUntil) {
		s.cacheReadUnavailableUntil = until
	}
}

func (s *usageRankingCacheState) markCacheWriteUnavailable(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	until := now.Add(usageRankingCacheFailureBackoff)
	if until.After(s.cacheWriteUnavailableUntil) {
		s.cacheWriteUnavailableUntil = until
	}
}

func (s *usageRankingCacheState) shouldBackOffCache(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return now.Before(s.cacheReadUnavailableUntil) ||
		now.Before(s.cacheWriteUnavailableUntil)
}

func (s *usageRankingCacheState) markCacheReadAvailable() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cacheReadUnavailableUntil = time.Time{}
}

func (s *usageRankingCacheState) markCacheWriteAvailable() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cacheWriteUnavailableUntil = time.Time{}
}

func (s *usageRankingCacheState) acquireLocalRefreshSlot(ctx context.Context) bool {
	s.mu.Lock()
	if s.localRefreshGate == nil {
		s.localRefreshGate = make(chan struct{}, 1)
	}
	gate := s.localRefreshGate
	s.mu.Unlock()

	select {
	case gate <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

func (s *usageRankingCacheState) tryAcquireLocalRefreshSlot() bool {
	s.mu.Lock()
	if s.localRefreshGate == nil {
		s.localRefreshGate = make(chan struct{}, 1)
	}
	gate := s.localRefreshGate
	s.mu.Unlock()

	select {
	case gate <- struct{}{}:
		return true
	default:
		return false
	}
}

func (s *usageRankingCacheState) releaseLocalRefreshSlot() {
	s.mu.Lock()
	gate := s.localRefreshGate
	s.mu.Unlock()
	if gate != nil {
		<-gate
	}
}

func normalizeUsageRankingQuery(query usagestats.UsageRankingQuery) usagestats.UsageRankingQuery {
	if query.Limit <= 0 || query.Limit > 10 {
		query.Limit = 10
	}
	if !usagestats.IsValidUsageRankingMetric(query.Metric) {
		query.Metric = usagestats.UsageRankingMetricTokens
	}
	// ComparisonEndTime advances every second with the current period. Without
	// bucketing, every page refresh would produce a distinct key and the cache
	// would never be reused. Query the same minute bucket that is represented by
	// the key so cached data and SQL boundaries cannot diverge.
	query.ComparisonEndTime = query.ComparisonEndTime.Truncate(time.Minute)
	return query
}

// usageRankingCacheKey identifies only the shared aggregation. Limit and
// CurrentUserID are intentionally excluded because they affect personalization,
// not the underlying leaderboard snapshot. The timezone name and offset are
// included separately from UTC instants because StartDate/EndDate are rendered
// in the start location.
func usageRankingCacheKey(query usagestats.UsageRankingQuery) string {
	query = normalizeUsageRankingQuery(query)
	timezoneName := "UTC"
	timezoneOffset := 0
	if location := query.StartTime.Location(); location != nil {
		timezoneName = location.String()
		_, timezoneOffset = query.StartTime.Zone()
	}

	identity := fmt.Sprintf(
		"metric=%s|period=%s|timezone=%s@%d|start=%s|end=%s|comparison_start=%s|comparison_end=%s",
		query.Metric,
		query.Period,
		timezoneName,
		timezoneOffset,
		query.StartTime.UTC().Format(time.RFC3339Nano),
		query.EndTime.UTC().Format(time.RFC3339Nano),
		query.ComparisonStartTime.UTC().Format(time.RFC3339Nano),
		query.ComparisonEndTime.UTC().Format(time.RFC3339Nano),
	)
	sum := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(sum[:])
}

// usageRankingL1Key groups adjacent minute buckets from the same logical
// leaderboard. ComparisonEndTime is the only moving dimension omitted. The
// snapshot is used solely as a short-lived fallback while a fresh exact-key
// value is unavailable; it is never written back under a different Redis key.
func usageRankingL1Key(query usagestats.UsageRankingQuery) string {
	query = normalizeUsageRankingQuery(query)
	timezoneName := "UTC"
	timezoneOffset := 0
	if location := query.StartTime.Location(); location != nil {
		timezoneName = location.String()
		_, timezoneOffset = query.StartTime.Zone()
	}

	identity := fmt.Sprintf(
		"metric=%s|period=%s|timezone=%s@%d|start=%s|end=%s|comparison_start=%s",
		query.Metric,
		query.Period,
		timezoneName,
		timezoneOffset,
		query.StartTime.UTC().Format(time.RFC3339Nano),
		query.EndTime.UTC().Format(time.RFC3339Nano),
		query.ComparisonStartTime.UTC().Format(time.RFC3339Nano),
	)
	sum := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(sum[:])
}

func (s *UsageService) getUsageRankingCached(ctx context.Context, query usagestats.UsageRankingQuery) (*usagestats.UsageRankingResponse, error) {
	query = normalizeUsageRankingQuery(query)
	snapshotRepo, supportsSnapshot := s.usageRepo.(usageRankingSnapshotRepository)
	if !supportsSnapshot {
		// Compatibility path for tests and alternate repository implementations.
		// A personalized response must never be stored under the shared key.
		ranking, err := s.fetchUsageRanking(ctx, query)
		if err != nil {
			return nil, normalizeUsageRankingFetchError(ctx, err)
		}
		return ranking, nil
	}
	if s.rankingCache == nil {
		// Disabling Redis caching must not remove the in-process pressure guard:
		// the leaderboard aggregation is still expensive enough to exhaust the
		// database pool if several metric/period combinations run together.
		if !s.rankingCacheState.acquireLocalRefreshSlot(ctx) {
			return nil, normalizeUsageRankingCallerError(ctx.Err())
		}
		defer s.rankingCacheState.releaseLocalRefreshSlot()

		queryCtx, cancel := context.WithTimeout(ctx, usageRankingFetchTimeout)
		defer cancel()
		snapshot, err := s.fetchUsageRankingSnapshot(queryCtx, snapshotRepo, query)
		if err != nil {
			return nil, normalizeUsageRankingFetchError(ctx, err)
		}
		return usagestats.PersonalizeUsageRanking(snapshot, query.CurrentUserID, query.Limit), nil
	}

	cacheKey := usageRankingCacheKey(query)
	l1Key := usageRankingL1Key(query)
	now := time.Now()
	if snapshot, ok := s.rankingCacheState.loadFreshL1(l1Key, cacheKey, now); ok {
		return usagestats.PersonalizeUsageRanking(snapshot, query.CurrentUserID, query.Limit), nil
	}
	if snapshot, ok := s.rankingCacheState.loadL1(l1Key, now); ok &&
		s.rankingCacheState.shouldBackOffCache(now) {
		return usagestats.PersonalizeUsageRanking(snapshot, query.CurrentUserID, query.Limit), nil
	}
	if snapshot, status := s.loadUsageRankingCache(ctx, cacheKey); status == usageRankingCacheReadHit {
		s.rankingCacheState.storeL1(l1Key, cacheKey, snapshot, time.Now())
		return usagestats.PersonalizeUsageRanking(snapshot, query.CurrentUserID, query.Limit), nil
	} else if status == usageRankingCacheReadUnavailable {
		s.rankingCacheState.markCacheReadUnavailable(time.Now())
		if snapshot, ok := s.rankingCacheState.loadL1(l1Key, time.Now()); ok {
			return usagestats.PersonalizeUsageRanking(snapshot, query.CurrentUserID, query.Limit), nil
		}
	}

	snapshot, err := s.rankingCacheState.doRefresh(ctx, cacheKey, func(flightCtx context.Context) (*usagestats.UsageRankingSnapshot, error) {
		return s.refreshUsageRankingSnapshot(flightCtx, snapshotRepo, query, cacheKey, l1Key)
	})
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, normalizeUsageRankingCallerError(ctxErr)
		}
		return nil, normalizeUsageRankingRefreshError(err)
	}
	if snapshot == nil {
		return nil, errors.New("get usage ranking: invalid shared result")
	}
	return usagestats.PersonalizeUsageRanking(snapshot, query.CurrentUserID, query.Limit), nil
}

func (s *UsageService) refreshUsageRankingSnapshot(
	ctx context.Context,
	repo usageRankingSnapshotRepository,
	query usagestats.UsageRankingQuery,
	cacheKey string,
	l1Key string,
) (*usagestats.UsageRankingSnapshot, error) {
	// Another request or process may have populated Redis while this request
	// waited for its local singleflight slot.
	if snapshot, ok := s.rankingCacheState.loadFreshL1(l1Key, cacheKey, time.Now()); ok {
		return snapshot, nil
	}
	if snapshot, status := s.loadUsageRankingCache(ctx, cacheKey); status == usageRankingCacheReadHit {
		s.rankingCacheState.storeL1(l1Key, cacheKey, snapshot, time.Now())
		return snapshot, nil
	} else if status == usageRankingCacheReadUnavailable {
		s.rankingCacheState.markCacheReadUnavailable(time.Now())
		return s.fallbackOrFetchUsageRankingSnapshot(ctx, repo, query, cacheKey, l1Key)
	}

	token, err := newUsageRankingRefreshToken()
	if err != nil {
		slog.Warn("failed to create usage ranking refresh token", "error", err)
		return s.fallbackOrFetchUsageRankingSnapshot(ctx, repo, query, cacheKey, l1Key)
	}
	acquired, err := s.tryAcquireUsageRankingRefresh(ctx, usageRankingGlobalRefreshLeaseKey, token)
	if err != nil {
		s.rankingCacheState.markCacheWriteUnavailable(time.Now())
		return s.fallbackOrFetchUsageRankingSnapshot(ctx, repo, query, cacheKey, l1Key)
	}
	if acquired {
		return s.fetchUsageRankingSnapshotWithLease(ctx, repo, query, cacheKey, l1Key, token)
	}

	// A different instance is already doing the expensive aggregation. The
	// previous good snapshot is preferable to making callers wait or issuing a
	// duplicate query. Cold instances without L1 poll Redis until the owner
	// publishes, the caller times out, or the bounded lease becomes available.
	if snapshot, ok := s.rankingCacheState.loadL1(l1Key, time.Now()); ok {
		return snapshot, nil
	}
	return s.waitForUsageRankingRefresh(ctx, repo, query, cacheKey, l1Key, token)
}

func (s *UsageService) waitForUsageRankingRefresh(
	ctx context.Context,
	repo usageRankingSnapshotRepository,
	query usagestats.UsageRankingQuery,
	cacheKey string,
	l1Key string,
	token string,
) (*usagestats.UsageRankingSnapshot, error) {
	ticker := time.NewTicker(usageRankingRefreshPollInterval)
	defer ticker.Stop()
	waitTimer := time.NewTimer(usageRankingRefreshWaitTimeout)
	defer waitTimer.Stop()

	for {
		select {
		case <-ctx.Done():
			if snapshot, ok := s.rankingCacheState.loadL1(l1Key, time.Now()); ok {
				return snapshot, nil
			}
			return nil, ctx.Err()
		case <-waitTimer.C:
			if snapshot, ok := s.rankingCacheState.loadL1(l1Key, time.Now()); ok {
				return snapshot, nil
			}
			return nil, errUsageRankingRefreshWaitTimeout
		case <-ticker.C:
		}

		if snapshot, status := s.loadUsageRankingCache(ctx, cacheKey); status == usageRankingCacheReadHit {
			s.rankingCacheState.storeL1(l1Key, cacheKey, snapshot, time.Now())
			return snapshot, nil
		} else if status == usageRankingCacheReadUnavailable {
			s.rankingCacheState.markCacheReadUnavailable(time.Now())
			return s.fallbackOrFetchUsageRankingSnapshot(ctx, repo, query, cacheKey, l1Key)
		}

		acquired, err := s.tryAcquireUsageRankingRefresh(ctx, usageRankingGlobalRefreshLeaseKey, token)
		if err != nil {
			s.rankingCacheState.markCacheWriteUnavailable(time.Now())
			return s.fallbackOrFetchUsageRankingSnapshot(ctx, repo, query, cacheKey, l1Key)
		}
		if acquired {
			return s.fetchUsageRankingSnapshotWithLease(ctx, repo, query, cacheKey, l1Key, token)
		}
		if snapshot, ok := s.rankingCacheState.loadL1(l1Key, time.Now()); ok {
			return snapshot, nil
		}
	}
}

func (s *UsageService) fetchUsageRankingSnapshotWithLease(
	ctx context.Context,
	repo usageRankingSnapshotRepository,
	query usagestats.UsageRankingQuery,
	cacheKey string,
	l1Key string,
	token string,
) (*usagestats.UsageRankingSnapshot, error) {
	// Never hold the cross-instance lease while waiting behind an in-process
	// fallback query. Otherwise the lease can expire before this owner reaches
	// the database and another instance may start the same expensive work.
	if !s.rankingCacheState.tryAcquireLocalRefreshSlot() {
		s.releaseUsageRankingRefresh(usageRankingGlobalRefreshLeaseKey, token)
		if !s.rankingCacheState.acquireLocalRefreshSlot(ctx) {
			return nil, ctx.Err()
		}
		s.rankingCacheState.releaseLocalRefreshSlot()
		return s.refreshUsageRankingSnapshot(ctx, repo, query, cacheKey, l1Key)
	}
	defer s.rankingCacheState.releaseLocalRefreshSlot()
	defer s.releaseUsageRankingRefresh(usageRankingGlobalRefreshLeaseKey, token)

	// The cache can be filled between the pre-lock GET and SET NX. Rechecking
	// after ownership avoids an unnecessary aggregation in that race.
	if snapshot, ok := s.rankingCacheState.loadFreshL1(l1Key, cacheKey, time.Now()); ok {
		return snapshot, nil
	}
	if snapshot, ok := s.rankingCacheState.loadL1(l1Key, time.Now()); ok &&
		s.rankingCacheState.shouldBackOffCache(time.Now()) {
		return snapshot, nil
	}
	if snapshot, status := s.loadUsageRankingCache(ctx, cacheKey); status == usageRankingCacheReadHit {
		s.rankingCacheState.storeL1(l1Key, cacheKey, snapshot, time.Now())
		return snapshot, nil
	} else if status == usageRankingCacheReadUnavailable {
		s.rankingCacheState.markCacheReadUnavailable(time.Now())
	}
	return s.fetchAndStoreUsageRankingSnapshotWithLocalSlot(ctx, repo, query, cacheKey, l1Key)
}

func (s *UsageService) fallbackOrFetchUsageRankingSnapshot(
	ctx context.Context,
	repo usageRankingSnapshotRepository,
	query usagestats.UsageRankingQuery,
	cacheKey string,
	l1Key string,
) (*usagestats.UsageRankingSnapshot, error) {
	if snapshot, ok := s.rankingCacheState.loadL1(l1Key, time.Now()); ok {
		return snapshot, nil
	}
	return s.fetchAndStoreUsageRankingSnapshot(ctx, repo, query, cacheKey, l1Key)
}

func (s *UsageService) fetchAndStoreUsageRankingSnapshot(
	ctx context.Context,
	repo usageRankingSnapshotRepository,
	query usagestats.UsageRankingQuery,
	cacheKey string,
	l1Key string,
) (*usagestats.UsageRankingSnapshot, error) {
	if !s.rankingCacheState.acquireLocalRefreshSlot(ctx) {
		return nil, ctx.Err()
	}
	defer s.rankingCacheState.releaseLocalRefreshSlot()

	return s.fetchAndStoreUsageRankingSnapshotWithLocalSlot(ctx, repo, query, cacheKey, l1Key)
}

func (s *UsageService) fetchAndStoreUsageRankingSnapshotWithLocalSlot(
	ctx context.Context,
	repo usageRankingSnapshotRepository,
	query usagestats.UsageRankingQuery,
	cacheKey string,
	l1Key string,
) (*usagestats.UsageRankingSnapshot, error) {
	if snapshot, ok := s.rankingCacheState.loadFreshL1(l1Key, cacheKey, time.Now()); ok {
		return snapshot, nil
	}
	if snapshot, ok := s.rankingCacheState.loadL1(l1Key, time.Now()); ok &&
		s.rankingCacheState.shouldBackOffCache(time.Now()) {
		return snapshot, nil
	}

	// Lease polling uses its own timer, so the repository receives a fresh query
	// budget here. The shared parent still cancels this work when every HTTP
	// waiter has left.
	queryCtx, cancel := context.WithTimeout(ctx, usageRankingFetchTimeout)
	defer cancel()
	snapshot, err := s.fetchUsageRankingSnapshot(queryCtx, repo, query)
	if err != nil {
		if stale, ok := s.rankingCacheState.loadL1(l1Key, time.Now()); ok {
			return stale, nil
		}
		return nil, normalizeUsageRankingFetchError(nil, err)
	}
	s.rankingCacheState.storeL1(l1Key, cacheKey, snapshot, time.Now())
	s.storeUsageRankingCache(cacheKey, snapshot)
	return snapshot, nil
}

func newUsageRankingRefreshToken() (string, error) {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(token[:]), nil
}

func (s *UsageService) tryAcquireUsageRankingRefresh(ctx context.Context, key string, token string) (bool, error) {
	cacheCtx, cancel := context.WithTimeout(ctx, usageRankingCacheTimeout)
	defer cancel()
	acquired, err := s.rankingCache.TryAcquireUsageRankingRefresh(
		cacheCtx,
		key,
		token,
		usageRankingRefreshLeaseTTL,
	)
	if err != nil {
		slog.Warn("failed to acquire usage ranking refresh lease", "error", err)
	}
	return acquired, err
}

func (s *UsageService) releaseUsageRankingRefresh(key string, token string) {
	cacheCtx, cancel := context.WithTimeout(context.Background(), usageRankingCacheTimeout)
	defer cancel()
	if err := s.rankingCache.ReleaseUsageRankingRefresh(cacheCtx, key, token); err != nil {
		slog.Warn("failed to release usage ranking refresh lease", "error", err)
		s.rankingCacheState.markCacheWriteUnavailable(time.Now())
	}
}

func (s *UsageService) fetchUsageRanking(ctx context.Context, query usagestats.UsageRankingQuery) (*usagestats.UsageRankingResponse, error) {
	ranking, err := s.usageRepo.GetUsageRanking(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("get usage ranking: %w", err)
	}
	return ranking, nil
}

func (s *UsageService) fetchUsageRankingSnapshot(
	ctx context.Context,
	repo usageRankingSnapshotRepository,
	query usagestats.UsageRankingQuery,
) (*usagestats.UsageRankingSnapshot, error) {
	snapshot, err := repo.GetUsageRankingSnapshot(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("get usage ranking: %w", err)
	}
	if !isValidUsageRankingSnapshot(snapshot) {
		return nil, errors.New("get usage ranking: invalid repository snapshot")
	}
	return snapshot, nil
}

func normalizeUsageRankingFetchError(callerCtx context.Context, err error) error {
	if callerCtx != nil {
		if ctxErr := callerCtx.Err(); ctxErr != nil {
			return normalizeUsageRankingCallerError(ctxErr)
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return infraerrors.GatewayTimeout(
			"USAGE_RANKING_TIMEOUT",
			"usage ranking query timed out",
		).WithCause(err)
	}
	return err
}

func normalizeUsageRankingCallerError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return infraerrors.GatewayTimeout(
			"USAGE_RANKING_REQUEST_TIMEOUT",
			"usage ranking request timed out",
		).WithCause(err)
	}
	return err
}

func normalizeUsageRankingRefreshError(err error) error {
	if err == nil || infraerrors.IsGatewayTimeout(err) || infraerrors.IsServiceUnavailable(err) {
		return err
	}
	if errors.Is(err, errUsageRankingRefreshWaitTimeout) || errors.Is(err, context.DeadlineExceeded) {
		return infraerrors.ServiceUnavailable(
			"USAGE_RANKING_REFRESH_BUSY",
			"usage ranking refresh is busy; retry shortly",
		).WithCause(err)
	}
	return err
}

func isValidUsageRankingSnapshot(snapshot *usagestats.UsageRankingSnapshot) bool {
	return snapshot != nil &&
		usagestats.IsValidUsageRankingMetric(snapshot.Metric) &&
		usagestats.IsValidUsageRankingPeriod(snapshot.Period) &&
		snapshot.GeneratedAt != "" &&
		snapshot.StartDate != "" &&
		snapshot.EndDate != "" &&
		snapshot.Items != nil
}

func (s *UsageService) loadUsageRankingCache(
	ctx context.Context,
	key string,
) (*usagestats.UsageRankingSnapshot, usageRankingCacheReadStatus) {
	cacheCtx, cancel := context.WithTimeout(ctx, usageRankingCacheTimeout)
	defer cancel()
	data, err := s.rankingCache.GetUsageRanking(cacheCtx, key)
	if err != nil {
		if errors.Is(err, ErrUsageRankingCacheMiss) {
			s.rankingCacheState.markCacheReadAvailable()
			return nil, usageRankingCacheReadMiss
		}
		slog.Warn("failed to read usage ranking cache", "error", err)
		return nil, usageRankingCacheReadUnavailable
	}
	s.rankingCacheState.markCacheReadAvailable()

	var snapshot usagestats.UsageRankingSnapshot
	if err := json.Unmarshal([]byte(data), &snapshot); err != nil || !isValidUsageRankingSnapshot(&snapshot) {
		slog.Warn("failed to decode usage ranking cache", "error", err)
		s.deleteUsageRankingCache(key)
		return nil, usageRankingCacheReadMiss
	}
	return &snapshot, usageRankingCacheReadHit
}

func (s *UsageService) storeUsageRankingCache(key string, snapshot *usagestats.UsageRankingSnapshot) {
	if !isValidUsageRankingSnapshot(snapshot) {
		return
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		slog.Warn("failed to encode usage ranking cache", "error", err)
		return
	}

	cacheCtx, cancel := context.WithTimeout(context.Background(), usageRankingCacheTimeout)
	defer cancel()
	if err := s.rankingCache.SetUsageRanking(cacheCtx, key, string(data), usageRankingCacheTTL); err != nil {
		slog.Warn("failed to store usage ranking cache", "error", err)
		s.rankingCacheState.markCacheWriteUnavailable(time.Now())
		return
	}
	s.rankingCacheState.markCacheWriteAvailable()
}

func (s *UsageService) deleteUsageRankingCache(key string) {
	cacheCtx, cancel := context.WithTimeout(context.Background(), usageRankingCacheTimeout)
	defer cancel()
	if err := s.rankingCache.DeleteUsageRanking(cacheCtx, key); err != nil {
		slog.Warn("failed to delete usage ranking cache", "error", err)
		s.rankingCacheState.markCacheWriteUnavailable(time.Now())
	}
}
