package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionresetcardgrant"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// SQLite does not implement SELECT ... FOR UPDATE. The consume path is still
// exercised against a real Ent schema and database by advertising PostgreSQL
// query capabilities to Ent and removing only the unsupported lock suffix
// immediately before SQLite executes the statement. Transaction boundaries,
// predicates, updates, uniqueness constraints, and audit inserts remain real.
type resetCardSQLiteLockCompatDriver struct {
	dialect.Driver
}

func (d *resetCardSQLiteLockCompatDriver) Dialect() string {
	return dialect.Postgres
}

func (d *resetCardSQLiteLockCompatDriver) Query(ctx context.Context, query string, args, v any) error {
	return d.Driver.Query(ctx, stripResetCardSQLiteLock(query), args, v)
}

func (d *resetCardSQLiteLockCompatDriver) Exec(ctx context.Context, query string, args, v any) error {
	return d.Driver.Exec(ctx, stripResetCardSQLiteLock(query), args, v)
}

func (d *resetCardSQLiteLockCompatDriver) Tx(ctx context.Context) (dialect.Tx, error) {
	tx, err := d.Driver.Tx(ctx)
	if err != nil {
		return nil, err
	}
	return &resetCardSQLiteLockCompatTx{Tx: tx}, nil
}

type resetCardSQLiteLockCompatTx struct {
	dialect.Tx
}

func (tx *resetCardSQLiteLockCompatTx) Query(ctx context.Context, query string, args, v any) error {
	return tx.Tx.Query(ctx, stripResetCardSQLiteLock(query), args, v)
}

func (tx *resetCardSQLiteLockCompatTx) Exec(ctx context.Context, query string, args, v any) error {
	return tx.Tx.Exec(ctx, stripResetCardSQLiteLock(query), args, v)
}

func stripResetCardSQLiteLock(query string) string {
	query = strings.ReplaceAll(query, " FOR UPDATE SKIP LOCKED", "")
	return strings.ReplaceAll(query, " FOR UPDATE", "")
}

type resetCardEntUserSubscriptionRepo struct {
	userSubRepoNoop
	client *dbent.Client
}

func (r *resetCardEntUserSubscriptionRepo) GetByID(ctx context.Context, id int64) (*UserSubscription, error) {
	row, err := r.client.UserSubscription.Get(ctx, id)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, ErrSubscriptionNotFound
		}
		return nil, err
	}
	return &UserSubscription{
		ID:                   row.ID,
		UserID:               row.UserID,
		GroupID:              row.GroupID,
		StartsAt:             row.StartsAt,
		ExpiresAt:            row.ExpiresAt,
		Status:               row.Status,
		DailyWindowStart:     row.DailyWindowStart,
		WeeklyWindowStart:    row.WeeklyWindowStart,
		MonthlyWindowStart:   row.MonthlyWindowStart,
		DailyUsageUSD:        row.DailyUsageUsd,
		WeeklyUsageUSD:       row.WeeklyUsageUsd,
		MonthlyUsageUSD:      row.MonthlyUsageUsd,
		DailyWindowVersion:   row.DailyWindowVersion,
		WeeklyWindowVersion:  row.WeeklyWindowVersion,
		MonthlyWindowVersion: row.MonthlyWindowVersion,
		AssignedBy:           row.AssignedBy,
		AssignedAt:           row.AssignedAt,
		CreatedAt:            row.CreatedAt,
		UpdatedAt:            row.UpdatedAt,
		DeletedAt:            row.DeletedAt,
	}, nil
}

type resetCardConsumeFixture struct {
	ctx           context.Context
	client        *dbent.Client
	service       *SubscriptionService
	userID        int64
	groupID       int64
	subscription  *dbent.UserSubscription
	originalStart time.Time
	originalEnd   time.Time
}

func newResetCardConsumeFixture(t *testing.T, withUsage bool) *resetCardConsumeFixture {
	t.Helper()

	db, err := sql.Open("sqlite", fmt.Sprintf("file:reset_card_consume_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	sqliteDriver := entsql.OpenDB(dialect.SQLite, db)
	client := dbent.NewClient(dbent.Driver(sqliteDriver))
	ctx := context.Background()
	require.NoError(t, client.Schema.Create(ctx))

	user := client.User.Create().
		SetEmail(fmt.Sprintf("reset-card-consume-%d@example.com", time.Now().UnixNano())).
		SetPasswordHash("hash").
		SaveX(ctx)
	group := client.Group.Create().
		SetName(fmt.Sprintf("Reset Card Consume %d", time.Now().UnixNano())).
		SetSubscriptionType(domain.SubscriptionTypeSubscription).
		SetDailyLimitUsd(10).
		SetWeeklyLimitUsd(50).
		SetMonthlyLimitUsd(100).
		SaveX(ctx)

	now := time.Now().UTC().Truncate(time.Microsecond)
	startsAt := now.Add(-5 * 24 * time.Hour)
	expiresAt := now.Add(25 * 24 * time.Hour)
	dailyUsage, weeklyUsage, monthlyUsage := 0.0, 0.0, 0.0
	if withUsage {
		dailyUsage, weeklyUsage, monthlyUsage = 3.25, 22.5, 88.75
	}
	subscription := client.UserSubscription.Create().
		SetUserID(user.ID).
		SetGroupID(group.ID).
		SetStartsAt(startsAt).
		SetExpiresAt(expiresAt).
		SetStatus(SubscriptionStatusActive).
		SetDailyWindowStart(now.Add(-4 * time.Hour)).
		SetWeeklyWindowStart(now.Add(-3 * 24 * time.Hour)).
		SetMonthlyWindowStart(now.Add(-15 * 24 * time.Hour)).
		SetDailyUsageUsd(dailyUsage).
		SetWeeklyUsageUsd(weeklyUsage).
		SetMonthlyUsageUsd(monthlyUsage).
		SetDailyWindowVersion(4).
		SetWeeklyWindowVersion(5).
		SetMonthlyWindowVersion(6).
		SaveX(ctx)

	compatClient := dbent.NewClient(dbent.Driver(&resetCardSQLiteLockCompatDriver{Driver: sqliteDriver}))
	repo := &resetCardEntUserSubscriptionRepo{client: client}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, compatClient, nil)
	t.Cleanup(svc.Stop)

	return &resetCardConsumeFixture{
		ctx:           ctx,
		client:        client,
		service:       svc,
		userID:        user.ID,
		groupID:       group.ID,
		subscription:  subscription,
		originalStart: startsAt,
		originalEnd:   expiresAt,
	}
}

func (f *resetCardConsumeFixture) createGrant(t *testing.T, count int, expiresAt *time.Time) *dbent.SubscriptionResetCardGrant {
	t.Helper()
	return f.client.SubscriptionResetCardGrant.Create().
		SetUserID(f.userID).
		SetGroupID(f.groupID).
		SetIssuedCount(count).
		SetRemainingCount(count).
		SetStatus(resetCardGrantStatusActive).
		SetNillableExpiresAt(expiresAt).
		SaveX(f.ctx)
}

func TestUseSubscriptionResetCardEntTransactionResetsQuotaAndAuditsEarliestGrant(t *testing.T) {
	fixture := newResetCardConsumeFixture(t, true)
	now := time.Now().UTC()
	laterExpiry := now.Add(10 * 24 * time.Hour)
	earlierExpiry := now.Add(2 * 24 * time.Hour)
	laterGrant := fixture.createGrant(t, 2, &laterExpiry)
	earlierGrant := fixture.createGrant(t, 2, &earlierExpiry)

	result, usage, err := fixture.service.UseSubscriptionResetCard(
		fixture.ctx,
		fixture.userID,
		fixture.subscription.ID,
		"manual-ent-consume",
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, usage)

	loadedSub := fixture.client.UserSubscription.GetX(fixture.ctx, fixture.subscription.ID)
	require.Zero(t, loadedSub.DailyUsageUsd)
	require.Zero(t, loadedSub.WeeklyUsageUsd)
	require.Zero(t, loadedSub.MonthlyUsageUsd)
	require.Equal(t, int64(5), loadedSub.DailyWindowVersion)
	require.Equal(t, int64(6), loadedSub.WeeklyWindowVersion)
	require.Equal(t, int64(7), loadedSub.MonthlyWindowVersion)
	require.WithinDuration(t, fixture.originalStart, loadedSub.StartsAt, time.Microsecond)
	require.WithinDuration(t, fixture.originalEnd, loadedSub.ExpiresAt, time.Microsecond)
	require.WithinDuration(t, fixture.originalEnd, result.ExpiresAt, time.Microsecond)
	require.WithinDuration(t, time.Now().UTC(), *loadedSub.DailyWindowStart, 5*time.Second)
	require.WithinDuration(t, time.Now().UTC(), *loadedSub.WeeklyWindowStart, 5*time.Second)
	require.WithinDuration(t, time.Now().UTC(), *loadedSub.MonthlyWindowStart, 5*time.Second)

	loadedEarlierGrant := fixture.client.SubscriptionResetCardGrant.GetX(fixture.ctx, earlierGrant.ID)
	loadedLaterGrant := fixture.client.SubscriptionResetCardGrant.GetX(fixture.ctx, laterGrant.ID)
	require.Equal(t, 1, loadedEarlierGrant.RemainingCount, "the earliest-expiring grant must be consumed first")
	require.Equal(t, 2, loadedLaterGrant.RemainingCount)

	auditRows := fixture.client.SubscriptionResetCardUsage.Query().AllX(fixture.ctx)
	require.Len(t, auditRows, 1)
	audit := auditRows[0]
	require.Equal(t, earlierGrant.ID, audit.GrantID)
	require.Equal(t, fixture.subscription.ID, audit.SubscriptionID)
	require.Equal(t, fixture.userID, audit.UserID)
	require.Equal(t, fixture.groupID, audit.GroupID)
	require.Equal(t, resetCardUsageModeManual, audit.Mode)
	require.Equal(t, 3.25, audit.PreviousDailyUsageUsd)
	require.Equal(t, 22.5, audit.PreviousWeeklyUsageUsd)
	require.Equal(t, 88.75, audit.PreviousMonthlyUsageUsd)
	require.Equal(t, audit.ID, usage.ID)
	require.Equal(t, earlierGrant.ID, usage.GrantID)
	require.NotEmpty(t, usage.UserEmail)
	require.NotEmpty(t, usage.GroupName)
}

func TestUseSubscriptionResetCardEntTransactionIdempotentReplayDoesNotConsumeAgain(t *testing.T) {
	fixture := newResetCardConsumeFixture(t, true)
	grant := fixture.createGrant(t, 2, nil)

	firstSub, firstUsage, err := fixture.service.UseSubscriptionResetCard(
		fixture.ctx,
		fixture.userID,
		fixture.subscription.ID,
		"manual-ent-replay",
	)
	require.NoError(t, err)
	require.NotNil(t, firstUsage)

	secondSub, secondUsage, err := fixture.service.UseSubscriptionResetCard(
		fixture.ctx,
		fixture.userID,
		fixture.subscription.ID,
		"manual-ent-replay",
	)
	require.NoError(t, err)
	require.Equal(t, firstUsage.ID, secondUsage.ID)
	require.Equal(t, firstSub.DailyWindowVersion, secondSub.DailyWindowVersion)
	require.Equal(t, firstSub.WeeklyWindowVersion, secondSub.WeeklyWindowVersion)
	require.Equal(t, firstSub.MonthlyWindowVersion, secondSub.MonthlyWindowVersion)

	loadedGrant := fixture.client.SubscriptionResetCardGrant.GetX(fixture.ctx, grant.ID)
	require.Equal(t, 1, loadedGrant.RemainingCount)
	require.Equal(t, 1, fixture.client.SubscriptionResetCardUsage.Query().CountX(fixture.ctx))
	loadedSub := fixture.client.UserSubscription.GetX(fixture.ctx, fixture.subscription.ID)
	require.Equal(t, int64(5), loadedSub.DailyWindowVersion)
	require.Equal(t, int64(6), loadedSub.WeeklyWindowVersion)
	require.Equal(t, int64(7), loadedSub.MonthlyWindowVersion)
}

func TestUseSubscriptionResetCardEntTransactionRejectedCallsLeaveStateUntouched(t *testing.T) {
	t.Run("no usage", func(t *testing.T) {
		fixture := newResetCardConsumeFixture(t, false)
		grant := fixture.createGrant(t, 1, nil)

		_, _, err := fixture.service.UseSubscriptionResetCard(
			fixture.ctx,
			fixture.userID,
			fixture.subscription.ID,
			"manual-ent-no-usage",
		)
		require.ErrorIs(t, err, ErrResetCardNoUsage)

		loadedGrant := fixture.client.SubscriptionResetCardGrant.GetX(fixture.ctx, grant.ID)
		require.Equal(t, 1, loadedGrant.RemainingCount)
		require.Equal(t, resetCardGrantStatusActive, loadedGrant.Status)
		assertResetCardSubscriptionUntouched(t, fixture)
	})

	t.Run("no card", func(t *testing.T) {
		fixture := newResetCardConsumeFixture(t, true)

		_, _, err := fixture.service.UseSubscriptionResetCard(
			fixture.ctx,
			fixture.userID,
			fixture.subscription.ID,
			"manual-ent-no-card",
		)
		require.ErrorIs(t, err, ErrResetCardNotAvailable)

		assertResetCardSubscriptionUntouched(t, fixture)
	})
}

func assertResetCardSubscriptionUntouched(t *testing.T, fixture *resetCardConsumeFixture) {
	t.Helper()
	loadedSub := fixture.client.UserSubscription.GetX(fixture.ctx, fixture.subscription.ID)
	require.Equal(t, fixture.subscription.DailyUsageUsd, loadedSub.DailyUsageUsd)
	require.Equal(t, fixture.subscription.WeeklyUsageUsd, loadedSub.WeeklyUsageUsd)
	require.Equal(t, fixture.subscription.MonthlyUsageUsd, loadedSub.MonthlyUsageUsd)
	require.Equal(t, fixture.subscription.DailyWindowVersion, loadedSub.DailyWindowVersion)
	require.Equal(t, fixture.subscription.WeeklyWindowVersion, loadedSub.WeeklyWindowVersion)
	require.Equal(t, fixture.subscription.MonthlyWindowVersion, loadedSub.MonthlyWindowVersion)
	require.WithinDuration(t, fixture.originalEnd, loadedSub.ExpiresAt, time.Microsecond)
	require.Equal(t, 0, fixture.client.SubscriptionResetCardUsage.Query().CountX(fixture.ctx))
	require.Equal(t, 0, fixture.client.SubscriptionResetCardGrant.Query().Where(
		subscriptionresetcardgrant.RemainingCountLT(1),
	).CountX(fixture.ctx))
}
