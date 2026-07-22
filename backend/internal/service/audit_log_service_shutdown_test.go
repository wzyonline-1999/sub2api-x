package service

import (
	"context"
	"sync"
	"testing"
	"time"
)

type blockingAuditLogRepository struct {
	started     chan struct{}
	release     chan struct{}
	startedOnce sync.Once
}

func (r *blockingAuditLogRepository) BatchInsert(_ context.Context, logs []*AuditLog) (int64, error) {
	r.startedOnce.Do(func() { close(r.started) })
	<-r.release // Simulate a driver that ignores context cancellation.
	return int64(len(logs)), nil
}

func (r *blockingAuditLogRepository) Insert(context.Context, *AuditLog) error { return nil }
func (r *blockingAuditLogRepository) List(context.Context, *AuditLogFilter) (*AuditLogList, error) {
	return &AuditLogList{}, nil
}
func (r *blockingAuditLogRepository) GetByID(context.Context, int64) (*AuditLog, error) {
	return nil, ErrAuditLogNotFound
}
func (r *blockingAuditLogRepository) Count(context.Context) (int64, error) { return 0, nil }
func (r *blockingAuditLogRepository) TruncateAll(context.Context) error    { return nil }
func (r *blockingAuditLogRepository) DeleteBefore(context.Context, time.Time, int) (int64, error) {
	return 0, nil
}

func TestAuditLogServiceStopIsBoundedWhenRepositoryHangs(t *testing.T) {
	repo := &blockingAuditLogRepository{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	svc := NewAuditLogService(repo, nil)
	svc.shutdownWaitTimeout = 25 * time.Millisecond
	svc.Start()
	for i := 0; i < auditLogBatchSize; i++ {
		svc.Record(&AuditLog{Action: "test.write", StatusCode: 200})
	}

	select {
	case <-repo.started:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for blocked audit repository write")
	}
	startedAt := time.Now()
	if svc.Stop() {
		t.Fatal("Stop() reported a complete drain while the repository was blocked")
	}
	if elapsed := time.Since(startedAt); elapsed > 250*time.Millisecond {
		t.Fatalf("Stop() exceeded its shutdown bound: %v", elapsed)
	}

	close(repo.release)
	if !svc.Stop() {
		t.Fatal("Stop() should complete after the repository is released")
	}
}
