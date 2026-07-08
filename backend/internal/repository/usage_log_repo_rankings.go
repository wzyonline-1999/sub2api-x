package repository

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

func usageRankingDisplayName(username string, email string, userID int64) string {
	username = strings.TrimSpace(username)
	if username != "" {
		return username
	}
	email = strings.TrimSpace(email)
	if email == "" {
		return fmt.Sprintf("User #%d", userID)
	}
	return email
}

func usageRankingTokenGap(target int64, current int64) int64 {
	gap := target - current + 1
	if gap < 1 {
		return 1
	}
	return gap
}

func usageRankingCostGap(target float64, current float64) float64 {
	targetCents := int64(math.Round(target * 100))
	currentCents := int64(math.Round(current * 100))
	gapCents := targetCents - currentCents + 1
	if gapCents < 1 {
		gapCents = 1
	}
	return float64(gapCents) / 100
}

func usageRankingProgressPercent(metric usagestats.UsageRankingMetric, current *usagestats.UsageRankingItem, target usagestats.UsageRankingItem) int {
	var currentValue float64
	var targetValue float64
	if current != nil {
		if metric == usagestats.UsageRankingMetricCost {
			currentValue = current.ActualCost
		} else {
			currentValue = float64(current.TotalTokens)
		}
	}
	if metric == usagestats.UsageRankingMetricCost {
		targetValue = target.ActualCost
	} else {
		targetValue = float64(target.TotalTokens)
	}
	if targetValue <= 0 {
		if currentValue > 0 {
			return 99
		}
		return 0
	}
	percent := int(math.Round(currentValue / targetValue * 100))
	if percent < 0 {
		return 0
	}
	if percent > 99 {
		return 99
	}
	return percent
}

func buildUsageRankingTarget(metric usagestats.UsageRankingMetric, current *usagestats.UsageRankingItem, target *usagestats.UsageRankingItem, targetType usagestats.UsageRankingTargetType) *usagestats.UsageRankingTarget {
	if targetType == usagestats.UsageRankingTargetNone || target == nil {
		progress := 0
		if current != nil && current.Rank == 1 {
			progress = 100
		}
		return &usagestats.UsageRankingTarget{
			TargetType:      usagestats.UsageRankingTargetNone,
			ProgressPercent: progress,
		}
	}
	targetRank := target.Rank
	targetUserID := target.UserID
	targetDisplayName := target.DisplayName
	var currentTokens int64
	var currentCost float64
	if current != nil {
		currentTokens = current.TotalTokens
		currentCost = current.ActualCost
	}
	return &usagestats.UsageRankingTarget{
		TargetType:        targetType,
		TargetRank:        &targetRank,
		TargetUserID:      &targetUserID,
		TargetDisplayName: &targetDisplayName,
		GapTokens:         usageRankingTokenGap(target.TotalTokens, currentTokens),
		GapActualCost:     usageRankingCostGap(target.ActualCost, currentCost),
		ProgressPercent:   usageRankingProgressPercent(metric, current, *target),
	}
}

func nearestHigherUsageRankingItem(currentRank int64, byRank map[int64]usagestats.UsageRankingItem) *usagestats.UsageRankingItem {
	var nearestRank int64
	var nearest *usagestats.UsageRankingItem
	for rank, item := range byRank {
		if rank >= currentRank || rank <= nearestRank {
			continue
		}
		itemCopy := item
		nearestRank = rank
		nearest = &itemCopy
	}
	return nearest
}

func rankingThresholdUsageItem(limit int, byRank map[int64]usagestats.UsageRankingItem) *usagestats.UsageRankingItem {
	limitRank := int64(limit)
	var thresholdRank int64
	var threshold *usagestats.UsageRankingItem
	for rank, item := range byRank {
		if rank > limitRank || rank <= thresholdRank {
			continue
		}
		itemCopy := item
		thresholdRank = rank
		threshold = &itemCopy
	}
	return threshold
}

func resolveUsageRankingTarget(metric usagestats.UsageRankingMetric, current *usagestats.UsageRankingItem, byRank map[int64]usagestats.UsageRankingItem, limit int) *usagestats.UsageRankingTarget {
	if current != nil {
		if current.Rank == 1 {
			return buildUsageRankingTarget(metric, current, nil, usagestats.UsageRankingTargetNone)
		}
		if current.Rank <= int64(limit) {
			if target := nearestHigherUsageRankingItem(current.Rank, byRank); target != nil {
				return buildUsageRankingTarget(metric, current, target, usagestats.UsageRankingTargetPrevious)
			}
			return buildUsageRankingTarget(metric, current, nil, usagestats.UsageRankingTargetNone)
		}
	}
	if target := rankingThresholdUsageItem(limit, byRank); target != nil {
		return buildUsageRankingTarget(metric, current, target, usagestats.UsageRankingTargetThreshold)
	}
	return buildUsageRankingTarget(metric, current, nil, usagestats.UsageRankingTargetNone)
}

func (r *usageLogRepository) GetUsageRanking(ctx context.Context, query usagestats.UsageRankingQuery) (result *usagestats.UsageRankingResponse, err error) {
	limit := query.Limit
	if limit <= 0 || limit > 10 {
		limit = 10
	}
	metric := query.Metric
	if !usagestats.IsValidUsageRankingMetric(metric) {
		metric = usagestats.UsageRankingMetricTokens
	}

	sqlQuery := `
		WITH user_usage AS (
			SELECT
				u.user_id,
				COALESCE(us.email, '') as email,
				COALESCE(us.username, '') as username,
				COUNT(*) as requests,
				COALESCE(SUM(u.input_tokens + u.output_tokens + u.cache_creation_tokens + u.cache_read_tokens), 0) as total_tokens,
				COALESCE(SUM(u.actual_cost), 0) as actual_cost
			FROM usage_logs u
			LEFT JOIN users us ON u.user_id = us.id
			WHERE u.created_at >= $1 AND u.created_at < $2
			GROUP BY u.user_id, us.email, us.username
		),
		ranked AS (
			SELECT
				ROW_NUMBER() OVER (
					ORDER BY
						CASE WHEN $4 = 'cost' THEN actual_cost ELSE total_tokens::numeric END DESC,
						total_tokens DESC,
						actual_cost DESC,
						user_id ASC
				) as rank,
				user_id,
				email,
				username,
				requests,
				total_tokens,
				actual_cost,
				COALESCE(SUM(total_tokens) OVER (), 0) as summary_total_tokens,
				COALESCE(SUM(actual_cost) OVER (), 0) as summary_total_actual_cost,
				COALESCE(SUM(requests) OVER (), 0) as summary_total_requests,
				COUNT(*) OVER () as ranked_users
			FROM user_usage
		),
		current_rank AS (
			SELECT rank
			FROM ranked
			WHERE user_id = $5
		)
		SELECT
			rank,
			user_id,
			email,
			username,
			requests,
			total_tokens,
			actual_cost,
			summary_total_tokens,
			summary_total_actual_cost,
			summary_total_requests,
			ranked_users
		FROM ranked
		WHERE rank <= $3
			OR user_id = $5
			OR rank = (
				SELECT MAX(r2.rank)
				FROM ranked r2, current_rank cr
				WHERE cr.rank > 1 AND cr.rank <= $3 AND r2.rank < cr.rank
			)
		ORDER BY rank ASC, user_id ASC
	`

	rows, err := r.sql.QueryContext(ctx, sqlQuery, query.StartTime, query.EndTime, limit, string(metric), query.CurrentUserID)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			result = nil
		}
	}()

	response := &usagestats.UsageRankingResponse{
		Metric:    metric,
		Period:    query.Period,
		StartDate: query.StartTime.Format("2006-01-02"),
		EndDate:   query.EndTime.AddDate(0, 0, -1).Format("2006-01-02"),
		Ranking:   make([]usagestats.UsageRankingItem, 0, limit),
	}
	itemsByRank := make(map[int64]usagestats.UsageRankingItem, limit+1)
	for rows.Next() {
		var item usagestats.UsageRankingItem
		var email string
		var username string
		if err = rows.Scan(
			&item.Rank,
			&item.UserID,
			&email,
			&username,
			&item.Requests,
			&item.TotalTokens,
			&item.ActualCost,
			&response.Summary.TotalTokens,
			&response.Summary.TotalActualCost,
			&response.Summary.TotalRequests,
			&response.Summary.RankedUsers,
		); err != nil {
			return nil, err
		}
		item.DisplayName = usageRankingDisplayName(username, email, item.UserID)
		if _, exists := itemsByRank[item.Rank]; !exists {
			itemsByRank[item.Rank] = item
		}
		if item.Rank <= int64(limit) {
			response.Ranking = append(response.Ranking, item)
		}
		if item.UserID == query.CurrentUserID {
			copyItem := item
			response.CurrentUser = &copyItem
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	response.CurrentUserTarget = resolveUsageRankingTarget(metric, response.CurrentUser, itemsByRank, limit)

	return response, nil
}
