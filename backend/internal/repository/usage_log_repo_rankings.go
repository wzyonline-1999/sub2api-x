package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

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

func (r *usageLogRepository) GetUsageRanking(ctx context.Context, query usagestats.UsageRankingQuery) (result *usagestats.UsageRankingResponse, err error) {
	snapshot, err := r.GetUsageRankingSnapshot(ctx, query)
	if err != nil {
		return nil, err
	}
	return usagestats.PersonalizeUsageRanking(snapshot, query.CurrentUserID, query.Limit), nil
}

// GetUsageRankingSnapshot performs the expensive two-period aggregation once
// and returns all ranked users. Personalization (Top N and current user) is
// intentionally performed by the service after the snapshot has been shared.
func (r *usageLogRepository) GetUsageRankingSnapshot(ctx context.Context, query usagestats.UsageRankingQuery) (result *usagestats.UsageRankingSnapshot, err error) {
	metric := query.Metric
	if !usagestats.IsValidUsageRankingMetric(metric) {
		metric = usagestats.UsageRankingMetricTokens
	}

	sqlQuery := `
		WITH previous_user_usage AS (
			SELECT
				user_id,
				COUNT(*) as previous_requests,
				COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) as previous_total_tokens,
				COALESCE(SUM(actual_cost), 0) as previous_actual_cost
			FROM usage_logs
			WHERE created_at >= $4 AND created_at < $5
			GROUP BY user_id
		),
		user_usage AS (
			SELECT
				u.user_id,
				COALESCE(us.email, '') as email,
				COALESCE(us.username, '') as username,
				COALESCE(ua.url, '') as avatar_url,
				COUNT(*) as requests,
				COALESCE(SUM(u.input_tokens + u.output_tokens + u.cache_creation_tokens + u.cache_read_tokens), 0) as total_tokens,
				COALESCE(SUM(u.actual_cost), 0) as actual_cost,
				COALESCE(pu.previous_requests, 0) as previous_requests,
				COALESCE(pu.previous_total_tokens, 0) as previous_total_tokens,
				COALESCE(pu.previous_actual_cost, 0) as previous_actual_cost
			FROM usage_logs u
			LEFT JOIN users us ON u.user_id = us.id
			LEFT JOIN user_avatars ua ON u.user_id = ua.user_id
			LEFT JOIN previous_user_usage pu ON u.user_id = pu.user_id
			WHERE u.created_at >= $1 AND u.created_at < $2
			GROUP BY u.user_id, us.email, us.username, ua.url, pu.previous_requests, pu.previous_total_tokens, pu.previous_actual_cost
		),
		ranked AS (
			SELECT
				ROW_NUMBER() OVER (
					ORDER BY
						CASE WHEN $3 = 'cost' THEN actual_cost ELSE total_tokens::numeric END DESC,
						total_tokens DESC,
						actual_cost DESC,
						user_id ASC
				) as rank,
				user_id,
				email,
				username,
				avatar_url,
				requests,
				total_tokens,
				actual_cost,
				previous_requests,
				previous_total_tokens,
				previous_actual_cost,
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
			username,
			avatar_url,
			requests,
			total_tokens,
			actual_cost,
			previous_requests,
			previous_total_tokens,
			previous_actual_cost,
			summary_total_tokens,
			summary_total_actual_cost,
			summary_total_requests,
			ranked_users
		FROM ranked
		ORDER BY rank ASC, user_id ASC
	`

	rows, err := r.sql.QueryContext(
		ctx,
		sqlQuery,
		query.StartTime,
		query.EndTime,
		string(metric),
		query.ComparisonStartTime,
		query.ComparisonEndTime,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			result = nil
		}
	}()

	snapshot := &usagestats.UsageRankingSnapshot{
		Metric:    metric,
		Period:    query.Period,
		StartDate: query.StartTime.Format("2006-01-02"),
		EndDate:   query.EndTime.AddDate(0, 0, -1).Format("2006-01-02"),
		Items:     make([]usagestats.UsageRankingItem, 0),
	}
	for rows.Next() {
		var item usagestats.UsageRankingItem
		var email string
		var username string
		if err = rows.Scan(
			&item.Rank,
			&item.UserID,
			&email,
			&username,
			&item.AvatarURL,
			&item.Requests,
			&item.TotalTokens,
			&item.ActualCost,
			&item.PreviousRequests,
			&item.PreviousTotalTokens,
			&item.PreviousActualCost,
			&snapshot.Summary.TotalTokens,
			&snapshot.Summary.TotalActualCost,
			&snapshot.Summary.TotalRequests,
			&snapshot.Summary.RankedUsers,
		); err != nil {
			return nil, err
		}
		item.DisplayName = usageRankingDisplayName(username, email, item.UserID)
		snapshot.Items = append(snapshot.Items, item)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	snapshot.GeneratedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return snapshot, nil
}
