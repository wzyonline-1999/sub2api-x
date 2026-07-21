package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type lateFillUserSubRepo struct {
	userSubRepoNoop

	mu           sync.Mutex
	sub          UserSubscription
	getCalls     int
	firstLoaded  chan struct{}
	releaseFirst chan struct{}
}

func (r *lateFillUserSubRepo) GetActiveByUserIDAndGroupID(_ context.Context, userID, groupID int64) (*UserSubscription, error) {
	r.mu.Lock()
	r.getCalls++
	call := r.getCalls
	snapshot := r.sub
	r.mu.Unlock()

	if snapshot.UserID != userID || snapshot.GroupID != groupID {
		return nil, ErrSubscriptionNotFound
	}
	if call == 1 {
		close(r.firstLoaded)
		<-r.releaseFirst
	}
	return &snapshot, nil
}

func (r *lateFillUserSubRepo) replace(sub UserSubscription) {
	r.mu.Lock()
	r.sub = sub
	r.mu.Unlock()
}

func (r *lateFillUserSubRepo) calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.getCalls
}

func TestGetActiveSubscription_InvalidationForgetsOldLoaderAndRejectsLateFill(t *testing.T) {
	repo := &lateFillUserSubRepo{
		sub: UserSubscription{
			ID:        1,
			UserID:    10,
			GroupID:   20,
			Status:    SubscriptionStatusActive,
			ExpiresAt: time.Now().Add(time.Hour),
			Notes:     "old snapshot",
		},
		firstLoaded:  make(chan struct{}),
		releaseFirst: make(chan struct{}),
	}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, &config.Config{
		SubscriptionCache: config.SubscriptionCacheConfig{
			// Ristretto includes roughly 64 units of internal cost per entry.
			L1Size:       128,
			L1TTLSeconds: 60,
		},
	})
	t.Cleanup(svc.Stop)

	type result struct {
		sub *UserSubscription
		err error
	}
	firstResult := make(chan result, 1)
	go func() {
		sub, err := svc.GetActiveSubscription(context.Background(), 10, 20)
		firstResult <- result{sub: sub, err: err}
	}()

	<-repo.firstLoaded
	fresh := repo.sub
	fresh.Notes = "fresh snapshot"
	repo.replace(fresh)
	svc.InvalidateSubCacheSync(10, 20)

	// A request that starts after invalidation must not join the old
	// singleflight. It should start a new DB load and observe the committed
	// snapshot even while the pre-invalidation loader remains blocked.
	secondResult := make(chan result, 1)
	go func() {
		sub, err := svc.GetActiveSubscription(context.Background(), 10, 20)
		secondResult <- result{sub: sub, err: err}
	}()
	var second result
	select {
	case second = <-secondResult:
	case <-time.After(time.Second):
		close(repo.releaseFirst)
		t.Fatal("post-invalidation request joined the stale singleflight loader")
	}
	require.NoError(t, second.err)
	require.Equal(t, "fresh snapshot", second.sub.Notes)
	require.Equal(t, 2, repo.calls())

	close(repo.releaseFirst)

	first := <-firstResult
	require.NoError(t, first.err)
	require.Equal(t, "old snapshot", first.sub.Notes, "the in-flight caller may finish with its already-read snapshot")

	svc.subCacheL1.Wait()
	cachedValue, cached := svc.subCacheL1.Get(subCacheKey(10, 20))
	require.True(t, cached)
	cachedSub, ok := cachedValue.(*UserSubscription)
	require.True(t, ok)
	require.Equal(t, "fresh snapshot", cachedSub.Notes, "the old loader must not overwrite the post-invalidation snapshot")

	third, err := svc.GetActiveSubscription(context.Background(), 10, 20)
	require.NoError(t, err)
	require.Equal(t, "fresh snapshot", third.Notes)
	require.Equal(t, 2, repo.calls(), "the guarded fresh snapshot should remain cached")
}

func TestGetActiveSubscription_InvalidationForgetsOldLoaderWithoutL1(t *testing.T) {
	repo := &lateFillUserSubRepo{
		sub: UserSubscription{
			ID:        1,
			UserID:    30,
			GroupID:   40,
			Status:    SubscriptionStatusActive,
			ExpiresAt: time.Now().Add(time.Hour),
			Notes:     "old snapshot",
		},
		firstLoaded:  make(chan struct{}),
		releaseFirst: make(chan struct{}),
	}
	svc := &SubscriptionService{userSubRepo: repo}

	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		_, _ = svc.GetActiveSubscription(context.Background(), 30, 40)
	}()
	<-repo.firstLoaded

	fresh := repo.sub
	fresh.Notes = "fresh snapshot"
	repo.replace(fresh)
	svc.InvalidateSubCacheSync(30, 40)

	secondResult := make(chan *UserSubscription, 1)
	go func() {
		sub, _ := svc.GetActiveSubscription(context.Background(), 30, 40)
		secondResult <- sub
	}()
	select {
	case second := <-secondResult:
		require.NotNil(t, second)
		require.Equal(t, "fresh snapshot", second.Notes)
	case <-time.After(time.Second):
		close(repo.releaseFirst)
		t.Fatal("post-invalidation request joined the stale singleflight loader while L1 was disabled")
	}

	close(repo.releaseFirst)
	<-firstDone
	require.Equal(t, 2, repo.calls())
}
