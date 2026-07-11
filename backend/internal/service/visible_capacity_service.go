package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// VisibleGroupConcurrency is the public, group-level concurrency aggregate.
// It intentionally contains no account identifiers or other administrative data.
type VisibleGroupConcurrency struct {
	Current        int     `json:"current"`
	Max            int     `json:"max"`
	Remaining      int     `json:"remaining"`
	LoadPercentage float64 `json:"load_percentage"`
	Waiting        int     `json:"waiting"`
}

// VisibleAccountConcurrency is the aggregate for schedulable accounts that
// have an explicit concurrency limit. A nil value means that no visible
// account in the group has configured a positive concurrency limit.
type VisibleAccountConcurrency struct {
	Current            int     `json:"current"`
	Max                int     `json:"max"`
	LoadPercentage     float64 `json:"load_percentage"`
	ConfiguredAccounts int     `json:"configured_accounts"`
}

// VisibleLoadCapacity describes how many linked upstream accounts are
// currently schedulable. Higher percentages indicate more available capacity.
type VisibleLoadCapacity struct {
	Available  int64   `json:"available"`
	Total      int64   `json:"total"`
	Percentage float64 `json:"percentage"`
}

// VisibleQuotaWindowLoad is a privacy-safe aggregate of one upstream quota
// window. Accounts without a sample are excluded from the equal-weighted
// average while TotalAccounts still reports the complete stable account pool.
type VisibleQuotaWindowLoad struct {
	LoadPercentage   float64 `json:"load_percentage"`
	AccountsWithData int     `json:"accounts_with_data"`
	TotalAccounts    int     `json:"total_accounts"`
}

// VisibleQuotaLoad contains the group-level 5-hour and 7-day quota pressure.
// A nil window means none of the group's stable accounts has data for it.
type VisibleQuotaLoad struct {
	FiveHour *VisibleQuotaWindowLoad `json:"five_hour"`
	SevenDay *VisibleQuotaWindowLoad `json:"seven_day"`
}

// VisibleCapacitySnapshot keeps one collection timestamp for the whole
// response so all group aggregates have one sampling time.
type VisibleCapacitySnapshot struct {
	CollectedAt time.Time              `json:"collected_at"`
	Groups      []VisibleGroupCapacity `json:"groups"`
}

// VisibleGroupCapacity is the safe response model exposed to authenticated
// users. Keep this type deliberately narrow: account identity, credentials,
// proxy information, and internal error state must never be added here.
type VisibleGroupCapacity struct {
	GroupID            int64                      `json:"group_id"`
	Name               string                     `json:"name"`
	Platform           string                     `json:"platform"`
	Concurrency        VisibleGroupConcurrency    `json:"concurrency"`
	AccountConcurrency *VisibleAccountConcurrency `json:"account_concurrency"`
	LoadCapacity       VisibleLoadCapacity        `json:"load_capacity"`
	QuotaLoad          VisibleQuotaLoad           `json:"quota_load"`
}

// GroupAccountQuotaLoadRow is the minimal account projection used to build
// public quota-window aggregates. It deliberately excludes account identity
// details, credentials, proxy configuration, and internal error state.
type GroupAccountQuotaLoadRow struct {
	GroupID          int64
	AccountID        int64
	Platform         string
	Extra            map[string]any
	SessionWindowEnd *time.Time
}

type visibleGroupLister interface {
	GetAvailableGroups(ctx context.Context, userID int64) ([]Group, error)
}

type visibleCapacityLoadReader interface {
	GetAccountsLoadBatch(ctx context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error)
}

type visibleQuotaLoadAccountLister interface {
	ListStableQuotaLoadByGroupIDs(ctx context.Context, groupIDs []int64) ([]GroupAccountQuotaLoadRow, error)
}

// VisibleCapacityService aggregates only the groups visible to the current
// user. The visibility source is intentionally the same service used by
// /groups/available so both endpoints share one authorization definition.
type VisibleCapacityService struct {
	visibleGroups visibleGroupLister
	accountRepo   AccountRepository
	loadReader    visibleCapacityLoadReader
}

// NewVisibleCapacityService creates the production visible-capacity service.
func NewVisibleCapacityService(
	apiKeyService *APIKeyService,
	accountRepo AccountRepository,
	concurrencyService *ConcurrencyService,
) *VisibleCapacityService {
	var loadReader visibleCapacityLoadReader
	if concurrencyService != nil {
		loadReader = concurrencyService
	}
	return newVisibleCapacityService(apiKeyService, accountRepo, loadReader)
}

func newVisibleCapacityService(
	visibleGroups visibleGroupLister,
	accountRepo AccountRepository,
	loadReader visibleCapacityLoadReader,
) *VisibleCapacityService {
	return &VisibleCapacityService{
		visibleGroups: visibleGroups,
		accountRepo:   accountRepo,
		loadReader:    loadReader,
	}
}

type visibleCapacityAccountRef struct {
	groupIndex int
	accountID  int64
	configured bool
}

// GetVisibleGroupCapacity returns capacity aggregates for the groups that the
// authenticated user may bind. Runtime load lookup is best-effort: cache
// failures degrade current/waiting values to zero while preserving configured
// capacity and availability data.
func (s *VisibleCapacityService) GetVisibleGroupCapacity(ctx context.Context, userID int64) (*VisibleCapacitySnapshot, error) {
	if s == nil || s.visibleGroups == nil {
		return &VisibleCapacitySnapshot{
			CollectedAt: time.Now().UTC(),
			Groups:      []VisibleGroupCapacity{},
		}, nil
	}

	groups, err := s.visibleGroups.GetAvailableGroups(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list visible groups: %w", err)
	}

	snapshot := &VisibleCapacitySnapshot{
		CollectedAt: time.Now().UTC(),
		Groups:      make([]VisibleGroupCapacity, len(groups)),
	}
	results := snapshot.Groups
	groupIDs := make([]int64, 0, len(groups))
	groupIndex := make(map[int64]int, len(groups))
	for i := range groups {
		group := &groups[i]
		results[i] = VisibleGroupCapacity{
			GroupID:  group.ID,
			Name:     group.Name,
			Platform: group.Platform,
			LoadCapacity: VisibleLoadCapacity{
				Available:  group.ActiveAccountCount,
				Total:      group.AccountCount,
				Percentage: capacityPercentage(group.ActiveAccountCount, group.AccountCount),
			},
		}
		if group.ID > 0 {
			if _, exists := groupIndex[group.ID]; !exists {
				groupIDs = append(groupIDs, group.ID)
				groupIndex[group.ID] = i
			}
		}
	}
	if len(groupIDs) == 0 {
		return snapshot, nil
	}

	rows, err := s.listVisibleCapacityAccounts(ctx, groupIDs)
	if err != nil {
		return nil, fmt.Errorf("list visible group capacity accounts: %w", err)
	}
	quotaRows, err := s.listVisibleQuotaLoadAccounts(ctx, groupIDs, snapshot.CollectedAt)
	if err != nil {
		return nil, fmt.Errorf("list visible group quota load accounts: %w", err)
	}
	aggregateVisibleQuotaLoad(results, groupIndex, quotaRows, snapshot.CollectedAt)

	refs := make([]visibleCapacityAccountRef, 0, len(rows))
	seenGroupAccount := make(map[groupCapacityAccountRef]struct{}, len(rows))
	batch := make([]AccountWithConcurrency, 0, len(rows))
	batchIndex := make(map[int64]int, len(rows))
	for _, row := range rows {
		idx, visible := groupIndex[row.GroupID]
		if !visible || row.AccountID <= 0 {
			continue
		}

		refKey := groupCapacityAccountRef{groupID: row.GroupID, accountID: row.AccountID}
		if _, duplicate := seenGroupAccount[refKey]; duplicate {
			continue
		}
		seenGroupAccount[refKey] = struct{}{}

		results[idx].Concurrency.Max += row.Concurrency
		configured := row.Concurrency > 0
		if configured {
			if results[idx].AccountConcurrency == nil {
				results[idx].AccountConcurrency = &VisibleAccountConcurrency{}
			}
			results[idx].AccountConcurrency.Max += row.Concurrency
			results[idx].AccountConcurrency.ConfiguredAccounts++
		}
		refs = append(refs, visibleCapacityAccountRef{
			groupIndex: idx,
			accountID:  row.AccountID,
			configured: configured,
		})

		if batchPos, exists := batchIndex[row.AccountID]; exists {
			if row.Concurrency > batch[batchPos].MaxConcurrency {
				batch[batchPos].MaxConcurrency = row.Concurrency
			}
			continue
		}
		batchIndex[row.AccountID] = len(batch)
		batch = append(batch, AccountWithConcurrency{
			ID:             row.AccountID,
			MaxConcurrency: row.Concurrency,
		})
	}

	loadMap := map[int64]*AccountLoadInfo{}
	if s.loadReader != nil && len(batch) > 0 {
		if loads, loadErr := s.loadReader.GetAccountsLoadBatch(ctx, batch); loadErr == nil && loads != nil {
			loadMap = loads
		}
	}

	for _, ref := range refs {
		load := loadMap[ref.accountID]
		if load == nil {
			continue
		}
		current := nonNegativeCapacityValue(load.CurrentConcurrency)
		waiting := nonNegativeCapacityValue(load.WaitingCount)
		results[ref.groupIndex].Concurrency.Current += current
		results[ref.groupIndex].Concurrency.Waiting += waiting
		if ref.configured && results[ref.groupIndex].AccountConcurrency != nil {
			results[ref.groupIndex].AccountConcurrency.Current += current
		}
	}

	for i := range results {
		concurrency := &results[i].Concurrency
		concurrency.Remaining = concurrency.Max - concurrency.Current
		if concurrency.Remaining < 0 {
			concurrency.Remaining = 0
		}
		concurrency.LoadPercentage = capacityPercentageInt(concurrency.Current, concurrency.Max)
		if account := results[i].AccountConcurrency; account != nil {
			account.LoadPercentage = capacityPercentageInt(account.Current, account.Max)
		}
	}

	return snapshot, nil
}

func (s *VisibleCapacityService) listVisibleCapacityAccounts(ctx context.Context, groupIDs []int64) ([]GroupAccountCapacityRow, error) {
	if s == nil || s.accountRepo == nil || len(groupIDs) == 0 {
		return []GroupAccountCapacityRow{}, nil
	}
	if lister, ok := s.accountRepo.(groupCapacityAccountLister); ok {
		return lister.ListSchedulableCapacityByGroupIDs(ctx, groupIDs)
	}

	rows := make([]GroupAccountCapacityRow, 0)
	for _, groupID := range groupIDs {
		accounts, err := s.accountRepo.ListSchedulableByGroupID(ctx, groupID)
		if err != nil {
			return nil, err
		}
		for i := range accounts {
			rows = append(rows, GroupAccountCapacityRow{
				GroupID:     groupID,
				AccountID:   accounts[i].ID,
				Concurrency: accounts[i].Concurrency,
			})
		}
	}
	return rows, nil
}

func (s *VisibleCapacityService) listVisibleQuotaLoadAccounts(ctx context.Context, groupIDs []int64, now time.Time) ([]GroupAccountQuotaLoadRow, error) {
	if s == nil || s.accountRepo == nil || len(groupIDs) == 0 {
		return []GroupAccountQuotaLoadRow{}, nil
	}
	if lister, ok := s.accountRepo.(visibleQuotaLoadAccountLister); ok {
		return lister.ListStableQuotaLoadByGroupIDs(ctx, groupIDs)
	}

	rows := make([]GroupAccountQuotaLoadRow, 0)
	for _, groupID := range groupIDs {
		accounts, err := s.accountRepo.ListByGroup(ctx, groupID)
		if err != nil {
			return nil, err
		}
		for i := range accounts {
			account := &accounts[i]
			if !isStableQuotaLoadAccount(account, now) {
				continue
			}
			rows = append(rows, GroupAccountQuotaLoadRow{
				GroupID:          groupID,
				AccountID:        account.ID,
				Platform:         account.Platform,
				Extra:            account.Extra,
				SessionWindowEnd: account.SessionWindowEnd,
			})
		}
	}
	return rows, nil
}

func isStableQuotaLoadAccount(account *Account, now time.Time) bool {
	if account == nil || account.Status != StatusActive || !account.Schedulable {
		return false
	}
	return !account.AutoPauseOnExpired || account.ExpiresAt == nil || now.Before(*account.ExpiresAt)
}

type visibleQuotaLoadAccumulator struct {
	totalAccounts int
	fiveHourSum   float64
	fiveHourCount int
	sevenDaySum   float64
	sevenDayCount int
}

func aggregateVisibleQuotaLoad(results []VisibleGroupCapacity, groupIndex map[int64]int, rows []GroupAccountQuotaLoadRow, now time.Time) {
	if len(results) == 0 || len(rows) == 0 {
		return
	}

	accumulators := make([]visibleQuotaLoadAccumulator, len(results))
	seen := make(map[groupCapacityAccountRef]struct{}, len(rows))
	for _, row := range rows {
		idx, visible := groupIndex[row.GroupID]
		if !visible || row.AccountID <= 0 {
			continue
		}
		key := groupCapacityAccountRef{groupID: row.GroupID, accountID: row.AccountID}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}

		accumulator := &accumulators[idx]
		accumulator.totalAccounts++
		if load, ok := quotaWindowLoad(row, "5h", now); ok {
			accumulator.fiveHourSum += load
			accumulator.fiveHourCount++
		}
		if load, ok := quotaWindowLoad(row, "7d", now); ok {
			accumulator.sevenDaySum += load
			accumulator.sevenDayCount++
		}
	}

	for i := range accumulators {
		accumulator := &accumulators[i]
		if accumulator.fiveHourCount > 0 {
			results[i].QuotaLoad.FiveHour = &VisibleQuotaWindowLoad{
				LoadPercentage:   accumulator.fiveHourSum / float64(accumulator.fiveHourCount),
				AccountsWithData: accumulator.fiveHourCount,
				TotalAccounts:    accumulator.totalAccounts,
			}
		}
		if accumulator.sevenDayCount > 0 {
			results[i].QuotaLoad.SevenDay = &VisibleQuotaWindowLoad{
				LoadPercentage:   accumulator.sevenDaySum / float64(accumulator.sevenDayCount),
				AccountsWithData: accumulator.sevenDayCount,
				TotalAccounts:    accumulator.totalAccounts,
			}
		}
	}
}

func quotaWindowLoad(row GroupAccountQuotaLoadRow, window string, now time.Time) (float64, bool) {
	switch row.Platform {
	case PlatformOpenAI:
		return codexQuotaWindowLoad(row.Extra, window, now)
	case PlatformAnthropic:
		return anthropicQuotaWindowLoad(row.Extra, row.SessionWindowEnd, window, now)
	default:
		return 0, false
	}
}

func codexQuotaWindowLoad(extra map[string]any, window string, now time.Time) (float64, bool) {
	if len(extra) == 0 {
		return 0, false
	}

	prefix := "codex_" + window
	load, ok := quotaLoadNumber(extra[prefix+"_used_percent"])
	if !ok {
		return 0, false
	}

	resetAt, hasReset := quotaLoadTime(extra[prefix+"_reset_at"])
	if !hasReset {
		resetAfter, hasResetAfter := quotaLoadNumber(extra[prefix+"_reset_after_seconds"])
		if !hasResetAfter {
			resetAfter, hasResetAfter = quotaLoadNumber(extra[prefix+"_reset_after"])
		}
		updatedAt, hasUpdatedAt := quotaLoadTime(extra["codex_usage_updated_at"])
		if hasResetAfter && resetAfter >= 0 && hasUpdatedAt {
			resetAt = updatedAt.Add(time.Duration(resetAfter * float64(time.Second)))
			hasReset = true
		}
	}
	if hasReset && !now.Before(resetAt) {
		load = 0
	}
	return clampQuotaLoad(load), true
}

func anthropicQuotaWindowLoad(extra map[string]any, sessionWindowEnd *time.Time, window string, now time.Time) (float64, bool) {
	if len(extra) == 0 {
		return 0, false
	}

	var (
		utilizationKey string
		resetAt        time.Time
		hasReset       bool
	)
	switch window {
	case "5h":
		utilizationKey = "session_window_utilization"
		if sessionWindowEnd != nil {
			resetAt = *sessionWindowEnd
			hasReset = true
		}
	case "7d":
		utilizationKey = "passive_usage_7d_utilization"
		resetAt, hasReset = quotaLoadTime(extra["passive_usage_7d_reset"])
	default:
		return 0, false
	}

	utilization, ok := quotaLoadNumber(extra[utilizationKey])
	if !ok {
		return 0, false
	}
	load := utilization * 100
	if hasReset && !now.Before(resetAt) {
		load = 0
	}
	return clampQuotaLoad(load), true
}

func quotaLoadNumber(value any) (float64, bool) {
	var (
		number float64
		err    error
	)
	switch typed := value.(type) {
	case float64:
		number = typed
	case float32:
		number = float64(typed)
	case int:
		number = float64(typed)
	case int8:
		number = float64(typed)
	case int16:
		number = float64(typed)
	case int32:
		number = float64(typed)
	case int64:
		number = float64(typed)
	case uint:
		number = float64(typed)
	case uint8:
		number = float64(typed)
	case uint16:
		number = float64(typed)
	case uint32:
		number = float64(typed)
	case uint64:
		number = float64(typed)
	case json.Number:
		number, err = typed.Float64()
	case string:
		number, err = strconv.ParseFloat(strings.TrimSpace(typed), 64)
	default:
		return 0, false
	}
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, false
	}
	return number, true
}

func quotaLoadTime(value any) (time.Time, bool) {
	switch typed := value.(type) {
	case time.Time:
		return typed, !typed.IsZero()
	case *time.Time:
		if typed == nil {
			return time.Time{}, false
		}
		return *typed, !typed.IsZero()
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return time.Time{}, false
		}
		if parsed, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
			return parsed, true
		}
	}

	seconds, ok := quotaLoadNumber(value)
	if !ok || seconds <= 0 {
		return time.Time{}, false
	}
	if seconds >= 1e12 {
		seconds /= 1000
	}
	whole := int64(seconds)
	nanos := int64((seconds - float64(whole)) * float64(time.Second))
	return time.Unix(whole, nanos), true
}

func clampQuotaLoad(load float64) float64 {
	if load < 0 {
		return 0
	}
	if load > 100 {
		return 100
	}
	return load
}

func capacityPercentage(available, total int64) float64 {
	if total <= 0 {
		return 0
	}
	return float64(available) / float64(total) * 100
}

func capacityPercentageInt(current, max int) float64 {
	if max <= 0 {
		return 0
	}
	return float64(current) / float64(max) * 100
}

func nonNegativeCapacityValue(value int) int {
	if value < 0 {
		return 0
	}
	return value
}
