<template>
  <AppLayout>
    <div class="rankings-page">
      <section class="ranking-controls panel">
        <div class="control-group">
          <span class="control-label">{{ t('rankings.controls.metric') }}</span>
          <div class="segmented" role="group" :aria-label="t('rankings.controls.metric')">
            <button
              type="button"
              data-testid="ranking-metric-tokens"
              class="segment-button"
              :class="{ active: metric === 'tokens' }"
              :aria-pressed="metric === 'tokens'"
              @click="setMetric('tokens')"
            >
              {{ t('rankings.controls.usage') }}
            </button>
            <button
              type="button"
              data-testid="ranking-metric-cost"
              class="segment-button"
              :class="{ active: metric === 'cost' }"
              :aria-pressed="metric === 'cost'"
              @click="setMetric('cost')"
            >
              {{ t('rankings.controls.cost') }}
            </button>
          </div>
        </div>

        <div class="control-group">
          <span class="control-label">{{ t('rankings.controls.period') }}</span>
          <div class="segmented" role="group" :aria-label="t('rankings.controls.period')">
            <button
              v-for="item in periods"
              :key="item.value"
              type="button"
              :data-testid="`ranking-period-${item.value}`"
              class="segment-button compact"
              :class="{ active: period === item.value }"
              :aria-pressed="period === item.value"
              @click="setPeriod(item.value)"
            >
              {{ item.label }}
            </button>
          </div>
        </div>

        <div class="selection-summary">
          <span>{{ t('rankings.controls.current') }}</span>
          <strong>{{ periodLabel }} · {{ metricLabel }}</strong>
        </div>
      </section>

      <section v-if="!loading" class="stats-grid" :aria-label="t('rankings.overview')">
        <article class="stat-card panel">
          <span class="stat-icon token">T</span>
          <div>
            <p>{{ t('rankings.totalTokens') }}</p>
            <strong>{{ formatTokens(summary.total_tokens) }}</strong>
            <small>{{ dateRangeLabel }}</small>
          </div>
        </article>
        <article class="stat-card panel">
          <span class="stat-icon cost">$</span>
          <div>
            <p>{{ t('rankings.actualCost') }}</p>
            <strong>{{ formatUsd(summary.total_actual_cost) }}</strong>
            <small>{{ metric === 'cost' ? t('rankings.currentSortBasis') : t('rankings.syncedCost') }}</small>
          </div>
        </article>
        <article class="stat-card panel">
          <span class="stat-icon users">U</span>
          <div>
            <p>{{ t('rankings.rankedAccounts') }}</p>
            <strong>{{ formatCount(summary.ranked_users) }}</strong>
            <small>{{ t('rankings.calledThisPeriod') }}</small>
          </div>
        </article>
        <article class="stat-card panel">
          <span class="stat-icon mine">#</span>
          <div>
            <p>{{ t('rankings.myRank') }}</p>
            <strong :class="['mine-status-text', mineStatusTone]">{{ mineStatusLabel }}</strong>
            <small>{{ mineStatSubtitle }}</small>
          </div>
        </article>
      </section>

      <div v-if="loading" class="loading-panel panel" role="status" aria-live="polite">
        {{ t('rankings.loading') }}
      </div>

      <template v-else>
        <section class="ranking-main">
          <article class="podium-panel panel">
            <div class="panel-heading">
              <h2>{{ t('rankings.topThree', { period: periodLabel }) }}</h2>
              <span>{{ dateRangeLabel }}</span>
            </div>

            <div v-if="topThree.length > 0" class="podium-grid">
              <div v-if="firstPlace" class="podium-card first">
                <img
                  class="podium-frame"
                  :src="goldDragonFrame"
                  alt=""
                  aria-hidden="true"
                  decoding="async"
                />
                <span class="podium-rank baked-in-frame">{{ t('rankings.rankPosition', { rank: 1 }) }}</span>
                <div class="podium-card-content">
                  <RankAvatar
                    :name="firstPlace.display_name"
                    :avatar-url="firstPlace.avatar_url ?? ''"
                    tone="gold"
                    large
                  />
                  <strong>{{ firstPlace.display_name }}</strong>
                  <b>{{ primaryMetricLabel(firstPlace) }}</b>
                  <small>{{ secondaryMetricLabel(firstPlace) }}</small>
                </div>
              </div>

              <div v-if="secondPlace" class="podium-card second">
                <img
                  class="podium-frame"
                  :src="silverPythonFrame"
                  alt=""
                  aria-hidden="true"
                  decoding="async"
                />
                <span class="podium-rank baked-in-frame">{{ t('rankings.rankPosition', { rank: 2 }) }}</span>
                <div class="podium-card-content">
                  <RankAvatar
                    :name="secondPlace.display_name"
                    :avatar-url="secondPlace.avatar_url ?? ''"
                    tone="silver"
                  />
                  <strong>{{ secondPlace.display_name }}</strong>
                  <b>{{ primaryMetricLabel(secondPlace) }}</b>
                  <small>{{ secondaryMetricLabel(secondPlace) }}</small>
                </div>
              </div>

              <div v-if="thirdPlace" class="podium-card third">
                <img
                  class="podium-frame"
                  :src="bronzeSnakeFrame"
                  alt=""
                  aria-hidden="true"
                  decoding="async"
                />
                <span class="podium-rank baked-in-frame">{{ t('rankings.rankPosition', { rank: 3 }) }}</span>
                <div class="podium-card-content">
                  <RankAvatar
                    :name="thirdPlace.display_name"
                    :avatar-url="thirdPlace.avatar_url ?? ''"
                    tone="bronze"
                  />
                  <strong>{{ thirdPlace.display_name }}</strong>
                  <b>{{ primaryMetricLabel(thirdPlace) }}</b>
                  <small>{{ secondaryMetricLabel(thirdPlace) }}</small>
                </div>
              </div>
            </div>

            <div v-else class="empty-state">{{ t('rankings.noRankingData') }}</div>
          </article>

          <aside class="side-stack">
            <section class="mine-card panel" :class="mineStatusTone">
              <div class="mine-card-header">
                <h2>{{ t('rankings.myRank') }}</h2>
                <span class="mine-status-badge">{{ mineStatusLabel }}</span>
              </div>
              <div class="mine-status-body">
                <strong>{{ mineHeadline }}</strong>
                <span>{{ mineDetail }}</span>
              </div>
              <div class="progress-track">
                <span :style="{ width: `${mineProgress}%` }"></span>
              </div>
              <p class="mine-progress-caption">{{ mineProgressCaption }}</p>
            </section>

            <section class="status-card panel">
              <h2>{{ t('rankings.status.title') }}</h2>
              <p><strong>{{ t('rankings.status.period') }}</strong>{{ dateRangeLabel }}</p>
              <p><strong>{{ t('rankings.status.updatedAt') }}</strong>{{ updatedAtLabel }}</p>
              <p><strong>{{ t('rankings.status.scope') }}</strong>{{ t('rankings.status.scopeValue') }}</p>
            </section>
          </aside>
        </section>

        <section class="list-panel panel">
          <div class="panel-heading list-heading">
            <h2>{{ t('rankings.fourthToTenth') }}</h2>
          </div>

          <div v-if="restRanking.length > 0" class="rank-list" role="list">
            <article
              v-for="item in restRanking"
              :key="item.user_id"
              class="rank-row"
              :class="{ 'is-current': currentUser?.user_id === item.user_id }"
              :data-testid="`ranking-row-${item.rank}`"
              role="listitem"
            >
              <span class="rank-number" :aria-label="t('rankings.rankPosition', { rank: item.rank })">
                <img
                  class="rank-number-sticker"
                  :src="jadeRankSticker"
                  alt=""
                  aria-hidden="true"
                  decoding="async"
                  draggable="false"
                />
                <strong aria-hidden="true">{{ item.rank }}</strong>
              </span>
              <div class="rank-identity">
                <RankAvatar
                  :name="item.display_name"
                  :avatar-url="item.avatar_url ?? ''"
                  tone="neutral"
                  compact
                  :data-testid="`ranking-avatar-${item.rank}`"
                />
                <div class="rank-profile">
                  <strong>{{ item.display_name }}</strong>
                  <div class="rank-profile-meta">
                    <span>{{ formatCalls(item.requests) }}</span>
                    <span v-if="currentUser?.user_id === item.user_id" class="current-user-label">
                      {{ t('rankings.myAccount') }}
                    </span>
                  </div>
                </div>
              </div>
              <div
                class="rank-trend"
                :class="trendTone(item)"
                :aria-label="`${comparisonLabel} ${trendValueLabel(item)}`"
                :data-testid="`ranking-trend-${item.rank}`"
              >
                <div class="rank-trend-summary">
                  <span>{{ comparisonLabel }}</span>
                  <strong>{{ trendValueLabel(item) }}</strong>
                </div>
                <small>{{ previousPeriodLabel }} {{ previousMetricLabel(item) }}</small>
              </div>
              <div class="rank-progress">
                <div class="rank-progress-label">
                  <span>{{ t('rankings.relativeToLeader') }}</span>
                  <strong>{{ relativeMetricLabel(item) }}</strong>
                </div>
                <span
                  class="metric-track"
                  role="progressbar"
                  :aria-label="t('rankings.relativeProgress')"
                  aria-valuemin="0"
                  aria-valuemax="100"
                  :aria-valuenow="Math.round(relativeMetricPercent(item))"
                >
                  <i :style="{ width: `${barPercent(item)}%` }"></i>
                </span>
              </div>
              <div class="rank-value">
                <b>{{ primaryMetricLabel(item) }}</b>
                <small>{{ secondaryMetricLabel(item) }}</small>
              </div>
            </article>
          </div>
          <div v-else class="empty-state compact">{{ t('rankings.noMoreRanks') }}</div>
        </section>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import RankAvatar from '@/views/user/components/RankAvatar.vue'
import jadeRankSticker from '@/assets/rankings/badges/jade-rank-sticker.png'
import bronzeSnakeFrame from '@/assets/rankings/podium/bronze-twin-snake-frame-v2.png'
import goldDragonFrame from '@/assets/rankings/podium/gold-twin-dragon-frame-v2.png'
import silverPythonFrame from '@/assets/rankings/podium/silver-twin-python-frame.png'
import {
  getRankings,
  type UsageRankingItem,
  type UsageRankingMetric,
  type UsageRankingPeriod,
  type UsageRankingSummary,
  type UsageRankingTarget,
} from '@/api/usage'
import { useAppStore } from '@/stores/app'

const appStore = useAppStore()
const { t, locale } = useI18n()

const metric = ref<UsageRankingMetric>('tokens')
const period = ref<UsageRankingPeriod>('day')
const loading = ref(false)
const updatedAt = ref<Date | null>(null)
const ranking = ref<UsageRankingItem[]>([])
const currentUser = ref<UsageRankingItem | null>(null)
const currentUserTarget = ref<UsageRankingTarget | null>(null)
const summary = ref<UsageRankingSummary>({
  total_tokens: 0,
  total_actual_cost: 0,
  total_requests: 0,
  ranked_users: 0,
})
const startDate = ref('')
const endDate = ref('')
let latestRankingRequest = 0

const periods = computed<Array<{ value: UsageRankingPeriod; label: string }>>(() => [
  { value: 'day', label: t('rankings.periods.day') },
  { value: 'week', label: t('rankings.periods.week') },
  { value: 'month', label: t('rankings.periods.month') },
])

const periodLabel = computed(
  () => periods.value.find((item) => item.value === period.value)?.label ?? t('rankings.periods.day')
)
const metricLabel = computed(() =>
  metric.value === 'tokens' ? t('rankings.controls.usage') : t('rankings.controls.cost')
)
const comparisonLabel = computed(() => t(`rankings.comparisons.${period.value}`))
const previousPeriodLabel = computed(() => t(`rankings.previousPeriods.${period.value}`))
const dateRangeLabel = computed(() => {
  if (!startDate.value || !endDate.value) return t('rankings.currentPeriod')
  return t('rankings.dateRange', { start: startDate.value, end: endDate.value })
})
const numberLocale = computed(() => locale.value === 'zh' ? 'zh-CN' : 'en-US')
const updatedAtLabel = computed(() => updatedAt.value?.toLocaleString(numberLocale.value) ?? '-')
const topThree = computed(() => ranking.value.slice(0, 3))
const firstPlace = computed(() => topThree.value[0] ?? null)
const secondPlace = computed(() => topThree.value[1] ?? null)
const thirdPlace = computed(() => topThree.value[2] ?? null)
const restRanking = computed(() => ranking.value.slice(3, 10))
const rankingThreshold = computed(() => ranking.value.length >= 10 ? ranking.value[9] : null)
const maxMetricValue = computed(() => {
  const values = ranking.value.map((item) => metricValue(item))
  const maximum = Math.max(...values, 0)
  return maximum > 0 ? maximum : 1
})
const currentMetricValue = computed(() => currentUser.value ? metricValue(currentUser.value) : 0)
const thresholdMetricValue = computed(() => rankingThreshold.value ? metricValue(rankingThreshold.value) : 0)
const minimumGap = computed(() => metric.value === 'tokens' ? 1 : 0.01)
const isCurrentUserRanked = computed(() => Boolean(currentUser.value && currentUser.value.rank <= 10))
const isCurrentUserFirst = computed(() => Boolean(isCurrentUserRanked.value && currentUser.value?.rank === 1))
const mineStatusTone = computed(() => isCurrentUserRanked.value ? 'ranked' : 'unranked')
const mineStatusLabel = computed(() =>
  isCurrentUserRanked.value ? t('rankings.ranked') : t('rankings.unranked')
)
const hasSummitTarget = computed(() => isCurrentUserFirst.value && (!currentUserTarget.value || currentUserTarget.value.target_type === 'none'))
const mineGapValue = computed(() => {
  if (isCurrentUserRanked.value) return 0
  if (!rankingThreshold.value) return minimumGap.value
  return Math.max(minimumGap.value, thresholdMetricValue.value - currentMetricValue.value + minimumGap.value)
})
const mineGapLabel = computed(() => formatMetricValue(mineGapValue.value))
const targetGapValue = computed(() => {
  if (!currentUserTarget.value) return mineGapValue.value
  return metric.value === 'tokens' ? currentUserTarget.value.gap_tokens : currentUserTarget.value.gap_actual_cost
})
const targetGapLabel = computed(() => formatMetricValue(targetGapValue.value))
const mineHeadline = computed(() => {
  if (hasSummitTarget.value) return t('rankings.summit')
  if (isCurrentUserRanked.value && currentUser.value) {
    return t('rankings.rankPosition', { rank: currentUser.value.rank })
  }
  if (currentUserTarget.value?.target_type === 'threshold') {
    return t('rankings.gapToRanking', { gap: targetGapLabel.value })
  }
  return t('rankings.gapToRanking', { gap: mineGapLabel.value })
})
const mineDetail = computed(() => {
  if (hasSummitTarget.value && currentUser.value) {
    return t('rankings.keepLeadWithMetric', { metric: primaryMetricLabel(currentUser.value) })
  }
  if (isCurrentUserRanked.value && currentUser.value) {
    if (currentUserTarget.value?.target_type === 'previous') {
      return t('rankings.gapToPreviousWithMetric', {
        metric: primaryMetricLabel(currentUser.value),
        gap: targetGapLabel.value,
      })
    }
    return `${primaryMetricLabel(currentUser.value)} · ${secondaryMetricLabel(currentUser.value)}`
  }
  if (currentUser.value) {
    return t('rankings.currentMetric', { metric: formatMetricValue(currentMetricValue.value) })
  }
  return t('rankings.noCallsThisPeriod')
})
const mineStatSubtitle = computed(() => {
  if (hasSummitTarget.value && currentUser.value) {
    return t('rankings.summitWithMetric', { metric: primaryMetricLabel(currentUser.value) })
  }
  if (isCurrentUserRanked.value && currentUser.value) {
    if (currentUserTarget.value?.target_type === 'previous') {
      return t('rankings.gapToPrevious', { gap: targetGapLabel.value })
    }
    return t('rankings.rankedWithMetric', {
      rank: currentUser.value.rank,
      metric: primaryMetricLabel(currentUser.value),
    })
  }
  if (currentUserTarget.value?.target_type === 'threshold') {
    return t('rankings.gapToRanking', { gap: targetGapLabel.value })
  }
  return t('rankings.gapToRanking', { gap: mineGapLabel.value })
})
const mineProgress = computed(() => {
  if (currentUserTarget.value) {
    return Math.min(100, Math.max(0, currentUserTarget.value.progress_percent))
  }
  if (isCurrentUserRanked.value) return 100
  const threshold = thresholdMetricValue.value
  if (threshold <= 0) return currentMetricValue.value > 0 ? 50 : 0
  const percent = Math.round((currentMetricValue.value / threshold) * 100)
  return Math.min(98, Math.max(currentMetricValue.value > 0 ? 8 : 0, percent))
})
const mineProgressCaption = computed(() => {
  if (hasSummitTarget.value) return t('rankings.keepLead')
  if (currentUserTarget.value?.target_type === 'previous') {
    return currentUserTarget.value.target_rank
      ? t('rankings.chaseRankTarget', { rank: currentUserTarget.value.target_rank })
      : t('rankings.oneStepBehindPrevious')
  }
  if (currentUserTarget.value?.target_type === 'threshold') {
    return currentUserTarget.value.target_rank
      ? t('rankings.rankThresholdTarget', { rank: currentUserTarget.value.target_rank })
      : t('rankings.topTenThreshold')
  }
  if (isCurrentUserRanked.value) return t('rankings.enteredTopTen')
  if (rankingThreshold.value) return t('rankings.tenthPlaceThreshold')
  return t('rankings.openRankingHint')
})

async function loadRankings() {
  const requestID = ++latestRankingRequest
  const requestedMetric = metric.value
  const requestedPeriod = period.value
  loading.value = true
  try {
    const response = await getRankings({
      metric: requestedMetric,
      period: requestedPeriod,
      limit: 10,
    })
    if (requestID !== latestRankingRequest) return
    ranking.value = response.ranking ?? []
    summary.value = response.summary ?? summary.value
    currentUser.value = response.current_user ?? null
    currentUserTarget.value = response.current_user_target ?? null
    startDate.value = response.start_date
    endDate.value = response.end_date
    const generatedAt = response.generated_at ? new Date(response.generated_at) : new Date()
    updatedAt.value = Number.isNaN(generatedAt.getTime()) ? new Date() : generatedAt
  } catch (error: any) {
    if (requestID !== latestRankingRequest) return
    ranking.value = []
    currentUser.value = null
    currentUserTarget.value = null
    summary.value = {
      total_tokens: 0,
      total_actual_cost: 0,
      total_requests: 0,
      ranked_users: 0,
    }
    startDate.value = ''
    endDate.value = ''
    updatedAt.value = null
    appStore.showError(error?.message || t('rankings.loadFailed'))
  } finally {
    if (requestID === latestRankingRequest) {
      loading.value = false
    }
  }
}

function setMetric(next: UsageRankingMetric) {
  if (metric.value === next) return
  metric.value = next
  void loadRankings()
}

function setPeriod(next: UsageRankingPeriod) {
  if (period.value === next) return
  period.value = next
  void loadRankings()
}

function formatTokens(value: number): string {
  if (value >= 1_000_000_000) return `${(value / 1_000_000_000).toFixed(1)}B`
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`
  if (value >= 1_000) return `${(value / 1_000).toFixed(1)}K`
  return Math.round(value).toLocaleString(numberLocale.value)
}

function formatUsd(value: number): string {
  return `$${value.toLocaleString(numberLocale.value, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`
}

function formatCount(value: number): string {
  return value.toLocaleString(numberLocale.value)
}

function metricValue(item: UsageRankingItem): number {
  return metric.value === 'tokens' ? item.total_tokens : item.actual_cost
}

function formatMetricValue(value: number): string {
  if (metric.value !== 'tokens') return formatUsd(value)
  return formatTokenQuantity(value)
}

function formatTokenQuantity(value: number): string {
  const unit = Math.round(value) === 1
    ? t('rankings.tokenSingular')
    : t('rankings.tokenPlural')
  return `${formatTokens(value)} ${unit}`
}

function formatCalls(value: number): string {
  const key = Math.round(value) === 1
    ? 'rankings.callSingular'
    : 'rankings.callPlural'
  return t(key, { count: formatCount(value) })
}

function primaryMetricLabel(item: UsageRankingItem): string {
  return formatMetricValue(metricValue(item))
}

function secondaryMetricLabel(item: UsageRankingItem): string {
  return metric.value === 'tokens'
    ? t('rankings.actualCostValue', { cost: formatUsd(item.actual_cost) })
    : formatTokenQuantity(item.total_tokens)
}

function previousMetricValue(item: UsageRankingItem): number {
  return metric.value === 'tokens'
    ? (item.previous_total_tokens ?? 0)
    : (item.previous_actual_cost ?? 0)
}

function previousMetricLabel(item: UsageRankingItem): string {
  return formatMetricValue(previousMetricValue(item))
}

function trendPercent(item: UsageRankingItem): number | null {
  const previous = previousMetricValue(item)
  if (previous <= 0) return null
  return ((metricValue(item) - previous) / previous) * 100
}

function trendTone(item: UsageRankingItem): 'up' | 'down' | 'flat' {
  const previous = previousMetricValue(item)
  const current = metricValue(item)
  if (previous <= 0) return current > 0 ? 'up' : 'flat'
  const percent = trendPercent(item) ?? 0
  if (Math.abs(percent) < 0.05) return 'flat'
  return percent > 0 ? 'up' : 'down'
}

function trendValueLabel(item: UsageRankingItem): string {
  const previous = previousMetricValue(item)
  const current = metricValue(item)
  if (previous <= 0) return current > 0 ? t('rankings.trendNew') : t('rankings.trendFlat')
  const percent = trendPercent(item) ?? 0
  if (Math.abs(percent) < 0.05) return t('rankings.trendFlat')
  const absolute = Math.abs(percent)
  const formatted = absolute >= 100
    ? Math.round(absolute).toLocaleString(numberLocale.value)
    : absolute.toFixed(1)
  return `${percent > 0 ? '+' : '-'}${formatted}%`
}

function barPercent(item: UsageRankingItem): number {
  return Math.min(100, Math.max(6, relativeMetricPercent(item)))
}

function relativeMetricPercent(item: UsageRankingItem): number {
  return (metricValue(item) / maxMetricValue.value) * 100
}

function relativeMetricLabel(item: UsageRankingItem): string {
  const percent = relativeMetricPercent(item)
  return `${percent >= 10 ? Math.round(percent) : percent.toFixed(1)}%`
}

onMounted(() => {
  void loadRankings()
})

onBeforeUnmount(() => {
  // Invalidate any request that resolves after navigation. This prevents a
  // detached ranking view from mutating state or showing an unrelated toast on
  // the page the user navigated to.
  latestRankingRequest++
})
</script>

<style scoped>
.rankings-page {
  display: flex;
  flex-direction: column;
  gap: 1.125rem;
  color: #0f172a;
}

.panel {
  border: 1px solid #e2e8f0;
  border-radius: 0.875rem;
  background: #fff;
  box-shadow: 0 8px 18px rgb(15 23 42 / 0.07);
}

.ranking-controls {
  display: flex;
  align-items: center;
  gap: 2.125rem;
  padding: 1.125rem 1.375rem;
}

.control-group {
  display: grid;
  gap: 0.5625rem;
}

.control-label {
  color: #475569;
  font-size: 0.8125rem;
  font-weight: 800;
}

.segmented {
  display: flex;
  gap: 0.375rem;
  align-items: center;
  padding: 0.125rem;
  border-radius: 0.75rem;
  background: #f8fafc;
}

.segment-button {
  height: 2.5rem;
  min-width: 8.75rem;
  border: 1px solid #cbd5e1;
  border-radius: 0.625rem;
  background: #fff;
  color: #334155;
  font-size: 0.9375rem;
  font-weight: 800;
  line-height: 1;
  transition:
    background-color 0.16s ease,
    color 0.16s ease,
    border-color 0.16s ease;
}

.segment-button.compact {
  min-width: 5.25rem;
}

.segment-button.active {
  border-color: #0f9f8f;
  background: #0f9f8f;
  color: #fff;
}

.selection-summary {
  display: grid;
  gap: 0.25rem;
  justify-items: end;
  margin-left: auto;
}

.selection-summary span {
  color: #94a3b8;
  font-size: 0.75rem;
  font-weight: 700;
}

.selection-summary strong {
  color: #0f766e;
  font-size: 1.125rem;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 1.25rem;
}

.stat-card {
  display: flex;
  min-height: 6.875rem;
  align-items: center;
  gap: 0.875rem;
  padding: 1.125rem;
}

.stat-icon {
  display: inline-flex;
  width: 2.75rem;
  height: 2.75rem;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  border-radius: 0.625rem;
  font-size: 1.125rem;
  font-weight: 900;
}

.stat-icon.token {
  background: #ccfbf1;
  color: #0f9f8f;
}

.stat-icon.cost {
  background: #dcfce7;
  color: #16a34a;
}

.stat-icon.users {
  background: #dbeafe;
  color: #2563eb;
}

.stat-icon.mine {
  background: #fef3c7;
  color: #d97706;
}

.stat-card p {
  margin: 0;
  color: #64748b;
  font-size: 0.8125rem;
  font-weight: 700;
}

.stat-card strong {
  display: block;
  margin-top: 0.125rem;
  color: #0f172a;
  font-size: 1.5rem;
  font-weight: 900;
  line-height: 1.18;
}

.stat-card small {
  display: block;
  margin-top: 0.125rem;
  color: #94a3b8;
  font-size: 0.75rem;
}

.mine-status-text.ranked {
  color: #0f766e;
}

.mine-status-text.unranked {
  color: #d97706;
}

.loading-panel {
  padding: 2rem;
  color: #64748b;
  text-align: center;
}

.ranking-main {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 23.75rem;
  gap: 1.25rem;
}

.podium-panel {
  min-height: 28rem;
  padding: 1.375rem 1.625rem 1.5rem;
}

.panel-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  margin-bottom: 1.125rem;
}

.panel-heading h2,
.mine-card h2,
.status-card h2 {
  margin: 0;
  font-size: 1.125rem;
  font-weight: 900;
}

.panel-heading span {
  color: #94a3b8;
  font-size: 0.75rem;
  font-weight: 700;
}

.podium-grid {
  display: grid;
  width: 100%;
  max-width: 75rem;
  grid-template-columns: minmax(0, 0.9fr) minmax(0, 1.1fr) minmax(0, 0.9fr);
  gap: 0.875rem;
  align-items: end;
  margin-inline: auto;
  padding-top: 0.625rem;
}

.podium-card {
  position: relative;
  display: block;
  height: auto;
  aspect-ratio: 4 / 3;
  min-width: 0;
  isolation: isolate;
  overflow: visible;
  border: 0;
  border-radius: 1.5rem;
  background: #fcfdff;
  box-shadow: 0 16px 32px rgb(15 23 42 / 0.1);
  text-align: center;
}

.podium-card.first {
  grid-column: 2;
  grid-row: 1;
  height: auto;
  background: #fffcf4;
  box-shadow: 0 18px 38px rgb(180 83 9 / 0.16);
}

.podium-card.second {
  grid-column: 1;
  grid-row: 1;
  background: #fcfdff;
}

.podium-card.third {
  grid-column: 3;
  grid-row: 1;
  background: #fffaf7;
}

.podium-frame {
  position: absolute;
  z-index: 2;
  inset: 0;
  display: block;
  width: 100%;
  height: 100%;
  object-fit: contain;
  pointer-events: none;
  user-select: none;
}

.podium-card.first .podium-frame {
  filter: drop-shadow(0 8px 10px rgb(180 83 9 / 0.18));
}

.podium-card.second .podium-frame {
  filter: drop-shadow(0 8px 10px rgb(71 85 105 / 0.16));
}

.podium-card.third .podium-frame {
  filter: drop-shadow(0 8px 10px rgb(154 52 18 / 0.16));
}

.podium-rank.baked-in-frame {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  border: 0;
  clip: rect(0 0 0 0);
  clip-path: inset(50%);
  transform: none;
  white-space: nowrap;
}

.podium-card-content {
  position: relative;
  z-index: 1;
  display: grid;
  height: 100%;
  min-width: 0;
  align-content: start;
  justify-items: center;
  gap: 0.35rem;
  padding: 5rem 2.75rem 1.25rem;
}

.podium-card.second .podium-card-content,
.podium-card.third .podium-card-content {
  gap: 0.25rem;
  padding-top: 4.75rem;
}

.podium-card.second .podium-card-content small,
.podium-card.third .podium-card-content small {
  transform: translateY(-0.25rem);
}

.podium-card.first .podium-card-content {
  gap: 0.45rem;
  padding: 5.875rem 3.25rem 1.375rem;
}

.podium-card strong {
  max-width: 100%;
  margin-top: 0.25rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 1rem;
  font-weight: 900;
}

.podium-card.first strong {
  font-size: 1.2rem;
}

.podium-card b {
  color: #475569;
  font-size: 1.375rem;
  line-height: 1.15;
}

.podium-card.first b {
  color: #b45309;
  font-size: 1.875rem;
}

.podium-card.third b {
  color: #9a3412;
}

.podium-card small {
  color: #64748b;
  font-size: 0.75rem;
}

.side-stack {
  display: grid;
  gap: 1.125rem;
}

.mine-card,
.status-card {
  padding: 1.25rem 1.375rem;
}

.mine-card {
  border-color: #dbeafe;
}

.mine-card.ranked {
  border-color: #99f6e4;
}

.mine-card.unranked {
  border-color: #fde68a;
}

.mine-card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
}

.mine-status-badge {
  display: inline-flex;
  min-width: 4rem;
  height: 1.75rem;
  align-items: center;
  justify-content: center;
  border-radius: 999px;
  background: #fef3c7;
  color: #b45309;
  font-size: 0.75rem;
  font-weight: 900;
}

.mine-card.ranked .mine-status-badge {
  background: #ccfbf1;
  color: #0f766e;
}

.mine-status-body {
  display: grid;
  gap: 0.375rem;
  margin-top: 1rem;
}

.mine-status-body strong {
  color: #d97706;
  font-size: 1.375rem;
  line-height: 1.2;
}

.mine-card.ranked .mine-status-body strong {
  color: #0f9f8f;
}

.mine-status-body span,
.mine-progress-caption,
.status-card p {
  color: #64748b;
  font-size: 0.8125rem;
}

.progress-track {
  height: 0.5rem;
  margin-top: 0.875rem;
  overflow: hidden;
  border-radius: 999px;
  background: #e2e8f0;
}

.progress-track span {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: #0f9f8f;
}

.mine-card.unranked .progress-track span {
  background: #f59e0b;
}

.mine-progress-caption {
  margin: 0.625rem 0 0;
}

.status-card {
  display: grid;
  gap: 0.85rem;
}

.status-card p {
  display: flex;
  justify-content: space-between;
  gap: 1rem;
  margin: 0;
}

.status-card strong {
  color: #334155;
}

.list-panel {
  padding: 1.25rem 1.5rem 1.5rem;
  background: #f8fafc;
}

.list-heading {
  margin-bottom: 1rem;
}

.rank-list {
  display: grid;
  gap: 0.625rem;
}

.rank-row {
  display: grid;
  grid-template-columns: 2.75rem minmax(12rem, 1fr) minmax(10rem, 14rem) minmax(13rem, 22rem) minmax(10rem, auto);
  gap: 1rem;
  align-items: center;
  min-height: 4.25rem;
  padding: 0.625rem 1rem;
  border: 1px solid #e7edf3;
  border-radius: 0.875rem;
  background: #fff;
  box-shadow: 0 3px 10px rgb(15 23 42 / 0.035);
  transition:
    border-color 0.18s ease,
    box-shadow 0.18s ease,
    transform 0.18s ease;
}

.rank-row:hover {
  border-color: #99e6dc;
  box-shadow: 0 10px 22px rgb(15 118 110 / 0.1);
  transform: translateY(-1px);
}

.rank-row.is-current {
  border-color: #5eead4;
  background: #f0fdfa;
  box-shadow: 0 8px 20px rgb(15 118 110 / 0.1);
}

.rank-number {
  position: relative;
  display: inline-grid;
  width: 2.75rem;
  height: 2.75rem;
  place-items: center;
  color: #0f766e;
  isolation: isolate;
  transform: translateY(-1px);
  transition:
    transform 0.18s ease;
}

.rank-number-sticker {
  position: absolute;
  z-index: 0;
  top: 50%;
  left: 50%;
  display: block;
  width: 3.5rem;
  height: 3.5rem;
  object-fit: contain;
  filter: drop-shadow(0 0.24rem 0.3rem rgb(15 118 110 / 0.18));
  pointer-events: none;
  transform: translate(-50%, -50%);
  transition: filter 0.18s ease;
  user-select: none;
}

.rank-number strong {
  position: relative;
  z-index: 1;
  color: inherit;
  font-size: 1.25rem;
  font-variant-numeric: tabular-nums;
  font-weight: 950;
  letter-spacing: -0.06em;
  line-height: 1;
  text-shadow:
    0 1px 0 #fff,
    0 2px 0 #b6d8d4,
    0 3px 5px rgb(15 118 110 / 0.2);
}

.rank-row:hover .rank-number {
  transform: translateY(-2px);
}

.rank-row:hover .rank-number-sticker {
  filter: brightness(1.035) drop-shadow(0 0.32rem 0.4rem rgb(15 118 110 / 0.24));
}

.rank-identity {
  display: grid;
  min-width: 0;
  grid-template-columns: 2.75rem minmax(0, 1fr);
  gap: 0.75rem;
  align-items: center;
}

.rank-profile {
  display: grid;
  min-width: 0;
  gap: 0.375rem;
}

.rank-profile > strong {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: #1e293b;
  font-size: 0.9375rem;
  font-weight: 850;
}

.rank-profile-meta {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 0.5rem;
  color: #94a3b8;
  font-size: 0.75rem;
}

.current-user-label {
  display: inline-flex;
  align-items: center;
  padding: 0.125rem 0.5rem;
  border-radius: 999px;
  background: #ccfbf1;
  color: #0f766e;
  font-size: 0.6875rem;
  font-weight: 800;
}

.rank-trend {
  display: grid;
  min-width: 0;
  gap: 0.35rem;
  padding-left: 1rem;
  border-left: 1px solid #e7edf3;
}

.rank-trend-summary {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 0.5rem;
}

.rank-trend-summary > span {
  color: #94a3b8;
  font-size: 0.6875rem;
  font-weight: 700;
  white-space: nowrap;
}

.rank-trend-summary > strong {
  display: inline-flex;
  min-height: 1.375rem;
  align-items: center;
  padding: 0 0.5rem;
  border-radius: 999px;
  font-size: 0.75rem;
  font-weight: 900;
  white-space: nowrap;
}

.rank-trend.up .rank-trend-summary > strong {
  background: #ecfdf5;
  color: #15803d;
}

.rank-trend.down .rank-trend-summary > strong {
  background: #fff1f2;
  color: #be123c;
}

.rank-trend.flat .rank-trend-summary > strong {
  background: #f1f5f9;
  color: #64748b;
}

.rank-trend > small {
  overflow: hidden;
  color: #94a3b8;
  font-size: 0.6875rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.rank-progress {
  display: grid;
  min-width: 0;
  gap: 0.5rem;
}

.rank-progress-label {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  color: #94a3b8;
  font-size: 0.6875rem;
  font-weight: 700;
}

.rank-progress-label strong {
  color: #0f766e;
  font-size: 0.75rem;
  font-weight: 850;
}

.metric-track {
  height: 0.5rem;
  overflow: hidden;
  border-radius: 999px;
  background: #e8eef2;
}

.metric-track i {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: #0f9f8f;
  box-shadow: 0 0 0 1px rgb(15 118 110 / 0.05);
  transition: width 0.28s ease;
}

.rank-value {
  display: grid;
  justify-items: end;
  gap: 0.25rem;
  text-align: right;
}

.rank-value b {
  color: #334155;
  font-size: 0.9375rem;
  line-height: 1.15;
}

.rank-value small {
  color: #94a3b8;
  font-size: 0.6875rem;
  white-space: nowrap;
}

.empty-state {
  display: flex;
  min-height: 10rem;
  align-items: center;
  justify-content: center;
  color: #64748b;
}

.empty-state.compact {
  min-height: 4rem;
}

.dark .rankings-page {
  color: #e5e7eb;
}

.dark .panel,
.dark .podium-card {
  border-color: #273244;
  background: #111827;
  box-shadow: 0 12px 28px rgb(0 0 0 / 0.24);
}

.dark .ranking-controls,
.dark .stat-card,
.dark .list-panel,
.dark .mine-card,
.dark .status-card,
.dark .podium-panel {
  background: #111827;
}

.dark .control-label,
.dark .status-card strong {
  color: #cbd5e1;
}

.dark .selection-summary strong,
.dark .panel-heading h2,
.dark .mine-card h2,
.dark .status-card h2,
.dark .stat-card strong,
.dark .rank-profile > strong {
  color: #f8fafc;
}

.dark .stat-card p,
.dark .panel-heading span,
.dark .podium-card small,
.dark .mine-status-body span,
.dark .mine-progress-caption,
.dark .status-card p {
  color: #94a3b8;
}

.dark .loading-panel,
.dark .empty-state,
.dark .empty-state.compact {
  color: #a7b3c5;
}

.dark .segmented {
  background: #1f2937;
}

.dark .rank-row {
  border-color: #2a3648;
  background: #172033;
  box-shadow: 0 6px 16px rgb(0 0 0 / 0.12);
}

.dark .rank-row:hover {
  border-color: rgb(45 212 191 / 0.5);
  box-shadow: 0 10px 24px rgb(0 0 0 / 0.22);
}

.dark .rank-row.is-current {
  border-color: rgb(45 212 191 / 0.65);
  background: #102a2a;
}

.dark .selection-summary span {
  color: #7c8aa0;
}

.dark .segment-button {
  border-color: #374151;
  background: #111827;
  color: #d1d5db;
}

.dark .segment-button.active {
  border-color: #14b8a6;
  background: #0f766e;
  color: #fff;
}

.dark .stat-icon.token {
  background: rgb(20 184 166 / 0.16);
  color: #5eead4;
}

.dark .stat-icon.cost {
  background: rgb(34 197 94 / 0.16);
  color: #86efac;
}

.dark .stat-icon.users {
  background: rgb(59 130 246 / 0.16);
  color: #93c5fd;
}

.dark .stat-icon.mine {
  background: rgb(245 158 11 / 0.18);
  color: #fbbf24;
}

.dark .podium-card.first {
  background: #2a1f0b;
}

.dark .podium-card.second {
  background: #151c28;
}

.dark .podium-card.third {
  background: #25140b;
}

.dark .podium-card strong,
.dark .podium-card b,
.dark .rank-value b {
  color: #e5e7eb;
}

.dark .podium-card.first b {
  color: #facc15;
}

.dark .podium-card.third b {
  color: #fdba74;
}

.dark .mine-card.ranked {
  border-color: rgb(45 212 191 / 0.55);
  background: linear-gradient(180deg, rgb(20 184 166 / 0.10), #111827 62%);
}

.dark .mine-card.unranked {
  border-color: rgb(245 158 11 / 0.55);
  background: linear-gradient(180deg, rgb(245 158 11 / 0.10), #111827 62%);
}

.dark .mine-card.ranked .mine-status-body strong {
  color: #5eead4;
}

.dark .mine-status-body strong {
  color: #fbbf24;
}

.dark .mine-status-badge {
  background: rgb(245 158 11 / 0.18);
  color: #fbbf24;
}

.dark .mine-card.ranked .mine-status-badge {
  background: rgb(20 184 166 / 0.18);
  color: #5eead4;
}

.dark .progress-track,
.dark .metric-track {
  background: #253044;
}

.dark .progress-track span {
  background: #14b8a6;
}

.dark .mine-card.unranked .progress-track span {
  background: #f59e0b;
}

.dark .metric-track i {
  background: #2dd4bf;
}

.dark .rank-number {
  color: #ccfbf1;
}

.dark .rank-number-sticker {
  filter: saturate(0.78) brightness(0.82) drop-shadow(0 0.28rem 0.36rem rgb(0 0 0 / 0.35));
}

.dark .rank-number strong {
  text-shadow:
    0 1px 0 #071d1b,
    0 2px 0 #0f5c55,
    0 3px 6px rgb(0 0 0 / 0.45);
}

.dark .rank-row:hover .rank-number {
  color: #e6fffb;
}

.dark .rank-row:hover .rank-number-sticker {
  filter: saturate(0.86) brightness(0.9) drop-shadow(0 0.34rem 0.44rem rgb(0 0 0 / 0.42));
}

.dark .rank-profile-meta,
.dark .rank-trend-summary > span,
.dark .rank-trend > small,
.dark .rank-progress-label,
.dark .rank-value small {
  color: #7f8da3;
}

.dark .rank-trend {
  border-left-color: #2a3648;
}

.dark .rank-trend.up .rank-trend-summary > strong {
  background: rgb(34 197 94 / 0.15);
  color: #86efac;
}

.dark .rank-trend.down .rank-trend-summary > strong {
  background: rgb(244 63 94 / 0.15);
  color: #fda4af;
}

.dark .rank-trend.flat .rank-trend-summary > strong {
  background: rgb(148 163 184 / 0.14);
  color: #cbd5e1;
}

.dark .rank-progress-label strong {
  color: #5eead4;
}

.dark .current-user-label {
  background: rgb(20 184 166 / 0.16);
  color: #5eead4;
}

@media (max-width: 1600px) {
  .ranking-main {
    grid-template-columns: 1fr;
  }

  .side-stack {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 1180px) {
  .stats-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .ranking-main {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 1280px) {
  .podium-grid {
    max-width: 52rem;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    margin-inline: auto;
  }

  .podium-card.first {
    width: 100%;
    max-width: 26rem;
    height: auto;
    grid-column: 1 / -1;
    grid-row: 1;
    justify-self: center;
  }

  .podium-card.second,
  .podium-card.third {
    width: 100%;
    max-width: 23.5rem;
    height: auto;
    grid-row: 2;
  }

  .podium-card.second {
    grid-column: 1;
    justify-self: end;
  }

  .podium-card.third {
    grid-column: 2;
    justify-self: start;
  }
}

@media (max-width: 960px) {
  .selection-summary {
    display: none;
  }

  .rank-row {
    grid-template-columns: 2.5rem 2.5rem minmax(0, 1fr) auto;
    gap: 0.625rem;
    min-height: 7.25rem;
    padding: 0.75rem;
  }

  .rank-number {
    width: 2.5rem;
    height: 2.5rem;
    grid-row: 1;
    align-self: start;
  }

  .rank-number-sticker {
    width: 3.2rem;
    height: 3.2rem;
  }

  .rank-identity {
    display: contents;
  }

  .rank-avatar.compact {
    grid-column: 2;
    grid-row: 1;
  }

  .rank-profile {
    grid-column: 3;
    grid-row: 1;
  }

  .rank-profile-meta > span:first-child {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .rank-trend {
    display: flex;
    grid-column: 2 / -1;
    grid-row: 2;
    align-items: center;
    gap: 0.625rem;
    padding-left: 0;
    border-left: 0;
  }

  .rank-trend > small {
    margin-left: auto;
    font-size: 0.625rem;
  }

  .rank-progress {
    grid-column: 2 / -1;
    grid-row: 3;
  }

  .rank-value {
    grid-column: 4;
    grid-row: 1;
  }

  .rank-value small {
    display: none;
  }
}

@media (max-width: 760px) {
  .ranking-controls {
    display: grid;
    gap: 0.75rem;
    padding: 0.875rem;
  }

  .control-label {
    display: none;
  }

  .segmented {
    width: 100%;
  }

  .segment-button {
    min-width: 0;
    flex: 1 1 0;
    font-size: 0.875rem;
  }

  .stats-grid {
    grid-template-columns: 1fr 1fr;
    gap: 0.625rem;
  }

  .stat-card {
    min-height: 5.25rem;
    padding: 0.75rem;
  }

  .stat-card strong {
    font-size: 1.15rem;
  }

  .stat-icon {
    width: 2.125rem;
    height: 2.125rem;
  }

  .podium-grid {
    grid-template-columns: 1fr;
    gap: 1rem;
    padding-top: 0.25rem;
  }

  .podium-card,
  .podium-card.first,
  .podium-card.second,
  .podium-card.third {
    grid-column: 1;
    grid-row: auto;
    width: 100%;
    max-width: 23.5rem;
    height: auto;
    margin-inline: auto;
    justify-self: center;
  }

  .podium-card.first {
    max-width: 26rem;
    height: auto;
  }

  .podium-card-content {
    padding-right: 3.25rem;
    padding-left: 3.25rem;
  }

  .podium-card.first .podium-card-content {
    padding-top: 5.875rem;
  }

  .side-stack {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 480px) {
  .podium-card.first .podium-card-content {
    gap: 0.25rem;
    padding-top: 4.5rem;
  }

  .podium-card.first strong {
    font-size: 1rem;
  }

  .podium-card.first b {
    font-size: 1.375rem;
  }

  .podium-card.second .podium-card-content,
  .podium-card.third .podium-card-content {
    padding-top: 4.375rem;
  }
}
</style>
