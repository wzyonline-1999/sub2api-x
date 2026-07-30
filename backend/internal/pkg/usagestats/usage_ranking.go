package usagestats

import "math"

func normalizeUsageRankingLimit(limit int) int {
	if limit <= 0 || limit > 10 {
		return 10
	}
	return limit
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

func usageRankingProgressPercent(metric UsageRankingMetric, current *UsageRankingItem, target UsageRankingItem) int {
	var currentValue float64
	var targetValue float64
	if current != nil {
		if metric == UsageRankingMetricCost {
			currentValue = current.ActualCost
		} else {
			currentValue = float64(current.TotalTokens)
		}
	}
	if metric == UsageRankingMetricCost {
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

func buildUsageRankingTarget(metric UsageRankingMetric, current *UsageRankingItem, target *UsageRankingItem, targetType UsageRankingTargetType) *UsageRankingTarget {
	if targetType == UsageRankingTargetNone || target == nil {
		progress := 0
		if current != nil && current.Rank == 1 {
			progress = 100
		}
		return &UsageRankingTarget{
			TargetType:      UsageRankingTargetNone,
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
	return &UsageRankingTarget{
		TargetType:        targetType,
		TargetRank:        &targetRank,
		TargetUserID:      &targetUserID,
		TargetDisplayName: &targetDisplayName,
		GapTokens:         usageRankingTokenGap(target.TotalTokens, currentTokens),
		GapActualCost:     usageRankingCostGap(target.ActualCost, currentCost),
		ProgressPercent:   usageRankingProgressPercent(metric, current, *target),
	}
}

func nearestHigherUsageRankingItem(currentRank int64, itemsByRank map[int64]UsageRankingItem) *UsageRankingItem {
	var nearestRank int64
	var nearest *UsageRankingItem
	for rank, item := range itemsByRank {
		if rank >= currentRank || rank <= nearestRank {
			continue
		}
		itemCopy := item
		nearestRank = rank
		nearest = &itemCopy
	}
	return nearest
}

func rankingThresholdUsageItem(limit int, itemsByRank map[int64]UsageRankingItem) *UsageRankingItem {
	limitRank := int64(limit)
	var thresholdRank int64
	var threshold *UsageRankingItem
	for rank, item := range itemsByRank {
		if rank > limitRank || rank <= thresholdRank {
			continue
		}
		itemCopy := item
		thresholdRank = rank
		threshold = &itemCopy
	}
	return threshold
}

func resolveUsageRankingTarget(metric UsageRankingMetric, current *UsageRankingItem, itemsByRank map[int64]UsageRankingItem, limit int) *UsageRankingTarget {
	if current != nil {
		if current.Rank == 1 {
			return buildUsageRankingTarget(metric, current, nil, UsageRankingTargetNone)
		}
		if current.Rank <= int64(limit) {
			if target := nearestHigherUsageRankingItem(current.Rank, itemsByRank); target != nil {
				return buildUsageRankingTarget(metric, current, target, UsageRankingTargetPrevious)
			}
			return buildUsageRankingTarget(metric, current, nil, UsageRankingTargetNone)
		}
	}
	if target := rankingThresholdUsageItem(limit, itemsByRank); target != nil {
		return buildUsageRankingTarget(metric, current, target, UsageRankingTargetThreshold)
	}
	return buildUsageRankingTarget(metric, current, nil, UsageRankingTargetNone)
}

// PrepareUsageRankingSnapshot builds immutable process-local indexes after a
// repository fetch or Redis decode and before the snapshot is shared through L1.
func PrepareUsageRankingSnapshot(snapshot *UsageRankingSnapshot) {
	if snapshot == nil {
		return
	}
	index := make(map[int64]int, len(snapshot.Items))
	for itemIndex, item := range snapshot.Items {
		if _, exists := index[item.UserID]; !exists {
			index[item.UserID] = itemIndex
		}
	}
	snapshot.itemIndexByUser = index
}

// PersonalizeUsageRanking turns a shared snapshot into the existing public API
// response without mutating the cached snapshot.
func PersonalizeUsageRanking(snapshot *UsageRankingSnapshot, currentUserID int64, limit int) *UsageRankingResponse {
	limit = normalizeUsageRankingLimit(limit)
	if snapshot == nil {
		return nil
	}

	response := &UsageRankingResponse{
		Metric:      snapshot.Metric,
		Period:      snapshot.Period,
		GeneratedAt: snapshot.GeneratedAt,
		StartDate:   snapshot.StartDate,
		EndDate:     snapshot.EndDate,
		Summary:     snapshot.Summary,
		Ranking:     make([]UsageRankingItem, 0, limit),
	}
	itemsByRank := make(map[int64]UsageRankingItem, limit)
	for _, item := range snapshot.Items {
		if item.Rank > int64(limit) {
			break
		}
		response.Ranking = append(response.Ranking, item)
		if _, exists := itemsByRank[item.Rank]; !exists {
			itemsByRank[item.Rank] = item
		}
		if item.UserID == currentUserID {
			itemCopy := item
			response.CurrentUser = &itemCopy
		}
	}
	if response.CurrentUser == nil {
		if itemIndex, exists := snapshot.itemIndexByUser[currentUserID]; exists &&
			itemIndex >= 0 && itemIndex < len(snapshot.Items) {
			itemCopy := snapshot.Items[itemIndex]
			response.CurrentUser = &itemCopy
		} else if snapshot.itemIndexByUser == nil {
			// Compatibility for direct repository callers that do not place the
			// snapshot in the cached service path.
			for _, item := range snapshot.Items {
				if item.UserID == currentUserID {
					itemCopy := item
					response.CurrentUser = &itemCopy
					break
				}
			}
		}
	}
	response.CurrentUserTarget = resolveUsageRankingTarget(
		snapshot.Metric,
		response.CurrentUser,
		itemsByRank,
		limit,
	)
	return response
}
