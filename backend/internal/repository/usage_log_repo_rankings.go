package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

func maskedUsageRankingDisplayName(email string, userID int64) string {
	email = strings.TrimSpace(email)
	if email == "" {
		return fmt.Sprintf("User #%d", userID)
	}
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		runes := []rune(email)
		if len(runes) == 0 {
			return fmt.Sprintf("User #%d", userID)
		}
		if len(runes) <= 2 {
			return string(runes[0]) + "*"
		}
		return string(runes[0]) + "***" + string(runes[len(runes)-1])
	}
	local := []rune(parts[0])
	domain := parts[1]
	if len(local) <= 1 {
		return string(local) + "*@" + domain
	}
	if len(local) == 2 {
		return string(local[0]) + "*@" + domain
	}
	return string(local[0]) + "***" + string(local[len(local)-1]) + "@" + domain
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
				COUNT(*) as requests,
				COALESCE(SUM(u.input_tokens + u.output_tokens + u.cache_creation_tokens + u.cache_read_tokens), 0) as total_tokens,
				COALESCE(SUM(u.actual_cost), 0) as actual_cost
			FROM usage_logs u
			LEFT JOIN users us ON u.user_id = us.id
			WHERE u.created_at >= $1 AND u.created_at < $2
			GROUP BY u.user_id, us.email
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
				requests,
				total_tokens,
				actual_cost,
				COALESCE(SUM(total_tokens) OVER (), 0) as summary_total_tokens,
				COALESCE(SUM(actual_cost) OVER (), 0) as summary_total_actual_cost,
				COALESCE(SUM(requests) OVER (), 0) as summary_total_requests,
				COUNT(*) OVER () as ranked_users
			FROM user_usage
		)
		SELECT
			rank,
			user_id,
			email,
			requests,
			total_tokens,
			actual_cost,
			summary_total_tokens,
			summary_total_actual_cost,
			summary_total_requests,
			ranked_users
		FROM ranked
		WHERE rank <= $3 OR user_id = $5
		ORDER BY rank ASC
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
	for rows.Next() {
		var item usagestats.UsageRankingItem
		var email string
		if err = rows.Scan(
			&item.Rank,
			&item.UserID,
			&email,
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
		item.DisplayName = maskedUsageRankingDisplayName(email, item.UserID)
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

	return response, nil
}
