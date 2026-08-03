//go:build unit

package admin

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSnapshotCache_SetAndGet(t *testing.T) {
	c := newSnapshotCache(5 * time.Second)

	entry := c.Set("key1", map[string]string{"hello": "world"})
	require.NotEmpty(t, entry.ETag)
	require.NotNil(t, entry.Payload)

	got, ok := c.Get("key1")
	require.True(t, ok)
	require.Equal(t, entry.ETag, got.ETag)
}

func TestSnapshotCache_Expiration(t *testing.T) {
	c := newSnapshotCache(1 * time.Millisecond)

	c.Set("key1", "value")
	time.Sleep(5 * time.Millisecond)

	_, ok := c.Get("key1")
	require.False(t, ok, "expired entry should not be returned")
}

func TestSnapshotCache_GetEmptyKey(t *testing.T) {
	c := newSnapshotCache(5 * time.Second)
	_, ok := c.Get("")
	require.False(t, ok)
}

func TestSnapshotCache_GetMiss(t *testing.T) {
	c := newSnapshotCache(5 * time.Second)
	_, ok := c.Get("nonexistent")
	require.False(t, ok)
}

func TestSnapshotCache_NilReceiver(t *testing.T) {
	var c *snapshotCache
	_, ok := c.Get("key")
	require.False(t, ok)

	entry := c.Set("key", "value")
	require.Empty(t, entry.ETag)
}

func TestSnapshotCache_SetEmptyKey(t *testing.T) {
	c := newSnapshotCache(5 * time.Second)

	// Set with empty key should return entry but not store it
	entry := c.Set("", "value")
	require.NotEmpty(t, entry.ETag)

	_, ok := c.Get("")
	require.False(t, ok)
}

func TestSnapshotCache_DefaultTTL(t *testing.T) {
	c := newSnapshotCache(0)
	require.Equal(t, 30*time.Second, c.ttl)
	require.Equal(t, defaultSnapshotCacheLoadTimeout, c.loadTimeout)
	require.Equal(t, defaultSnapshotCacheMaxEntries, c.maxEntries)

	c2 := newSnapshotCache(-1 * time.Second)
	require.Equal(t, 30*time.Second, c2.ttl)
}

func TestSnapshotCache_CapacityEvictsOldestEntry(t *testing.T) {
	c := newSnapshotCacheWithCapacity(time.Minute, 2)

	c.Set("oldest", "a")
	time.Sleep(time.Millisecond)
	c.Set("newer", "b")
	c.Set("newest", "c")

	c.mu.RLock()
	require.Len(t, c.items, 2)
	c.mu.RUnlock()
	_, ok := c.Get("oldest")
	require.False(t, ok)
	_, ok = c.Get("newer")
	require.True(t, ok)
	_, ok = c.Get("newest")
	require.True(t, ok)
}

func TestSnapshotCache_SetSweepsExpiredEntriesBeforeCapacityEviction(t *testing.T) {
	c := newSnapshotCacheWithCapacity(time.Millisecond, 2)
	c.Set("expired-a", "a")
	c.Set("expired-b", "b")
	time.Sleep(5 * time.Millisecond)

	c.Set("fresh", "c")

	c.mu.RLock()
	require.Len(t, c.items, 1)
	_, ok := c.items["fresh"]
	c.mu.RUnlock()
	require.True(t, ok)
}

func TestSnapshotCache_ETagDeterministic(t *testing.T) {
	c := newSnapshotCache(5 * time.Second)
	payload := map[string]int{"a": 1, "b": 2}

	entry1 := c.Set("k1", payload)
	entry2 := c.Set("k2", payload)
	require.Equal(t, entry1.ETag, entry2.ETag, "same payload should produce same ETag")
}

func TestSnapshotCache_ETagFormat(t *testing.T) {
	c := newSnapshotCache(5 * time.Second)
	entry := c.Set("k", "test")
	// ETag should be quoted hex string: "abcdef..."
	require.True(t, len(entry.ETag) > 2)
	require.Equal(t, byte('"'), entry.ETag[0])
	require.Equal(t, byte('"'), entry.ETag[len(entry.ETag)-1])
}

func TestBuildETagFromAny_UnmarshalablePayload(t *testing.T) {
	// channels are not JSON-serializable
	etag := buildETagFromAny(make(chan int))
	require.Empty(t, etag)
}

func TestSnapshotCache_GetOrLoad_MissThenHit(t *testing.T) {
	c := newSnapshotCache(5 * time.Second)
	var loads atomic.Int32

	entry, hit, err := c.GetOrLoad(context.Background(), "key1", func(context.Context) (any, error) {
		loads.Add(1)
		return map[string]string{"hello": "world"}, nil
	})
	require.NoError(t, err)
	require.False(t, hit)
	require.NotEmpty(t, entry.ETag)
	require.Equal(t, int32(1), loads.Load())

	entry2, hit, err := c.GetOrLoad(context.Background(), "key1", func(context.Context) (any, error) {
		loads.Add(1)
		return map[string]string{"unexpected": "value"}, nil
	})
	require.NoError(t, err)
	require.True(t, hit)
	require.Equal(t, entry.ETag, entry2.ETag)
	require.Equal(t, int32(1), loads.Load())
}

func TestSnapshotCache_GetOrLoad_ConcurrentSingleflight(t *testing.T) {
	c := newSnapshotCache(5 * time.Second)
	var loads atomic.Int32
	start := make(chan struct{})
	const callers = 8
	errCh := make(chan error, callers)

	var wg sync.WaitGroup
	wg.Add(callers)
	for range callers {
		go func() {
			defer wg.Done()
			<-start
			_, _, err := c.GetOrLoad(context.Background(), "shared", func(context.Context) (any, error) {
				loads.Add(1)
				time.Sleep(20 * time.Millisecond)
				return "value", nil
			})
			errCh <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}

	require.Equal(t, int32(1), loads.Load())
}

func TestSnapshotCache_GetOrLoad_FirstCallerCancellationDoesNotCancelSharedLoad(t *testing.T) {
	c := newSnapshotCacheWithLoadTimeout(time.Minute, time.Second)
	started := make(chan struct{})
	release := make(chan struct{})
	var loads atomic.Int32
	var startedOnce sync.Once

	load := func(ctx context.Context) (any, error) {
		loads.Add(1)
		startedOnce.Do(func() { close(started) })
		select {
		case <-release:
			return "value", nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	type callResult struct {
		entry snapshotCacheEntry
		err   error
	}
	firstResult := make(chan callResult, 1)
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	go func() {
		entry, _, err := c.GetOrLoad(firstCtx, "shared", load)
		firstResult <- callResult{entry: entry, err: err}
	}()
	<-started

	secondResult := make(chan callResult, 1)
	go func() {
		entry, _, err := c.GetOrLoad(context.Background(), "shared", load)
		secondResult <- callResult{entry: entry, err: err}
	}()
	require.Eventually(t, func() bool {
		c.mu.RLock()
		defer c.mu.RUnlock()
		flight := c.flights["shared"]
		return flight != nil && flight.waiters == 2
	}, time.Second, time.Millisecond)

	cancelFirst()
	first := <-firstResult
	require.ErrorIs(t, first.err, context.Canceled)

	c.mu.RLock()
	remainingFlight := c.flights["shared"]
	require.NotNil(t, remainingFlight)
	require.Equal(t, 1, remainingFlight.waiters)
	require.NoError(t, remainingFlight.ctx.Err())
	c.mu.RUnlock()

	close(release)
	second := <-secondResult
	require.NoError(t, second.err)
	require.Equal(t, "value", second.entry.Payload)
	require.Equal(t, int32(1), loads.Load())
}

func TestSnapshotCache_GetOrLoad_LastWaiterCancelsAndRemovesFlight(t *testing.T) {
	c := newSnapshotCacheWithLoadTimeout(time.Minute, time.Second)
	started := make(chan struct{})
	loaderCanceled := make(chan struct{})
	callerCtx, cancelCaller := context.WithCancel(context.Background())
	resultCh := make(chan error, 1)

	go func() {
		_, _, err := c.GetOrLoad(callerCtx, "abandoned", func(ctx context.Context) (any, error) {
			close(started)
			<-ctx.Done()
			close(loaderCanceled)
			return nil, ctx.Err()
		})
		resultCh <- err
	}()
	<-started
	cancelCaller()

	require.ErrorIs(t, <-resultCh, context.Canceled)
	require.Eventually(t, func() bool {
		c.mu.RLock()
		defer c.mu.RUnlock()
		_, exists := c.flights["abandoned"]
		return !exists
	}, time.Second, time.Millisecond)
	select {
	case <-loaderCanceled:
	case <-time.After(time.Second):
		t.Fatal("last waiter did not cancel the loader context")
	}

	entry, hit, err := c.GetOrLoad(context.Background(), "abandoned", func(context.Context) (any, error) {
		return "replacement", nil
	})
	require.NoError(t, err)
	require.False(t, hit)
	require.Equal(t, "replacement", entry.Payload)
}

func TestSnapshotCache_GetOrLoad_LoaderTimeoutIsBounded(t *testing.T) {
	c := newSnapshotCacheWithLoadTimeout(time.Minute, 20*time.Millisecond)
	releaseIgnoredLoader := make(chan struct{})
	startedAt := time.Now()

	_, _, err := c.GetOrLoad(context.Background(), "slow", func(context.Context) (any, error) {
		<-releaseIgnoredLoader
		return "late", nil
	})
	close(releaseIgnoredLoader)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(startedAt), time.Second)
	_, ok := c.Get("slow")
	require.False(t, ok, "timed-out loader must not populate the cache")
}

func TestSnapshotCache_GetOrLoad_PreservesContextValuesWithoutCallerCancellation(t *testing.T) {
	type contextKey string
	const key contextKey = "timezone"
	c := newSnapshotCacheWithLoadTimeout(time.Minute, time.Second)
	callerCtx, cancel := context.WithCancel(context.WithValue(context.Background(), key, "Asia/Shanghai"))
	defer cancel()

	entry, _, err := c.GetOrLoad(callerCtx, "value", func(loadCtx context.Context) (any, error) {
		return loadCtx.Value(key), nil
	})
	require.NoError(t, err)
	require.Equal(t, "Asia/Shanghai", entry.Payload)
}

func TestSnapshotCache_GetOrLoad_LoaderPanicReturnsError(t *testing.T) {
	c := newSnapshotCache(time.Minute)
	_, _, err := c.GetOrLoad(context.Background(), "panic", func(context.Context) (any, error) {
		panic("boom")
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "snapshot cache loader panic: boom")
}

func TestParseBoolQueryWithDefault(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		def  bool
		want bool
	}{
		{"empty returns default true", "", true, true},
		{"empty returns default false", "", false, false},
		{"1", "1", false, true},
		{"true", "true", false, true},
		{"TRUE", "TRUE", false, true},
		{"yes", "yes", false, true},
		{"on", "on", false, true},
		{"0", "0", true, false},
		{"false", "false", true, false},
		{"FALSE", "FALSE", true, false},
		{"no", "no", true, false},
		{"off", "off", true, false},
		{"whitespace trimmed", "  true  ", false, true},
		{"unknown returns default true", "maybe", true, true},
		{"unknown returns default false", "maybe", false, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseBoolQueryWithDefault(tc.raw, tc.def)
			require.Equal(t, tc.want, got)
		})
	}
}
