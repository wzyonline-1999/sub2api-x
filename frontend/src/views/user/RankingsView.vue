<template>
  <AppLayout>
    <div class="rankings-page">
      <section class="ranking-controls panel">
        <div class="control-group">
          <span class="control-label">榜单类型</span>
          <div class="segmented" role="group" aria-label="榜单类型">
            <button
              type="button"
              data-testid="ranking-metric-tokens"
              class="segment-button"
              :class="{ active: metric === 'tokens' }"
              @click="setMetric('tokens')"
            >
              使用量榜
            </button>
            <button
              type="button"
              data-testid="ranking-metric-cost"
              class="segment-button"
              :class="{ active: metric === 'cost' }"
              @click="setMetric('cost')"
            >
              花费榜
            </button>
          </div>
        </div>

        <div class="control-group">
          <span class="control-label">时间范围</span>
          <div class="segmented" role="group" aria-label="时间范围">
            <button
              v-for="item in periods"
              :key="item.value"
              type="button"
              :data-testid="`ranking-period-${item.value}`"
              class="segment-button compact"
              :class="{ active: period === item.value }"
              @click="setPeriod(item.value)"
            >
              {{ item.label }}
            </button>
          </div>
        </div>

        <div class="selection-summary">
          <span>当前</span>
          <strong>{{ periodLabel }} · {{ metric === 'tokens' ? '使用量榜' : '花费榜' }}</strong>
        </div>
      </section>

      <section class="stats-grid" aria-label="榜单概览">
        <article class="stat-card panel">
          <span class="stat-icon token">T</span>
          <div>
            <p>总 Token</p>
            <strong>{{ formatTokens(summary.total_tokens) }}</strong>
            <small>{{ dateRangeLabel }}</small>
          </div>
        </article>
        <article class="stat-card panel">
          <span class="stat-icon cost">$</span>
          <div>
            <p>实际花费</p>
            <strong>{{ formatUsd(summary.total_actual_cost) }}</strong>
            <small>{{ metric === 'cost' ? '当前排序口径' : '同步展示花费' }}</small>
          </div>
        </article>
        <article class="stat-card panel">
          <span class="stat-icon users">U</span>
          <div>
            <p>上榜账号</p>
            <strong>{{ summary.ranked_users.toLocaleString() }}</strong>
            <small>当前周期有调用</small>
          </div>
        </article>
        <article class="stat-card panel">
          <span class="stat-icon mine">#</span>
          <div>
            <p>我的排名</p>
            <strong :class="['mine-status-text', mineStatusTone]">{{ mineStatusLabel }}</strong>
            <small>{{ mineStatSubtitle }}</small>
          </div>
        </article>
      </section>

      <div v-if="loading" class="loading-panel panel">正在加载排行榜...</div>

      <template v-else>
        <section class="ranking-main">
          <article class="podium-panel panel">
            <div class="panel-heading">
              <h2>{{ periodLabel }}前三名</h2>
              <span>{{ dateRangeLabel }}</span>
            </div>

            <div v-if="topThree.length > 0" class="podium-grid">
              <div v-if="secondPlace" class="podium-card second">
                <RankAvatar :name="secondPlace.display_name" tone="silver" />
                <span class="medal silver">银牌</span>
                <strong>{{ secondPlace.display_name }}</strong>
                <b>{{ primaryMetricLabel(secondPlace) }}</b>
                <small>{{ secondaryMetricLabel(secondPlace) }}</small>
              </div>

              <div v-if="firstPlace" class="podium-card first">
                <span class="crown" aria-label="榜首皇冠">♕</span>
                <RankAvatar :name="firstPlace.display_name" tone="gold" large />
                <span class="medal gold">金牌</span>
                <strong>{{ firstPlace.display_name }}</strong>
                <b>{{ primaryMetricLabel(firstPlace) }}</b>
                <small>{{ secondaryMetricLabel(firstPlace) }}</small>
              </div>

              <div v-if="thirdPlace" class="podium-card third">
                <RankAvatar :name="thirdPlace.display_name" tone="bronze" />
                <span class="medal bronze">铜牌</span>
                <strong>{{ thirdPlace.display_name }}</strong>
                <b>{{ primaryMetricLabel(thirdPlace) }}</b>
                <small>{{ secondaryMetricLabel(thirdPlace) }}</small>
              </div>
            </div>

            <div v-else class="empty-state">当前周期暂无排行数据</div>
          </article>

          <aside class="side-stack">
            <section class="mine-card panel" :class="mineStatusTone">
              <div class="mine-card-header">
                <h2>我的排名</h2>
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
              <h2>榜单状态</h2>
              <p><strong>统计周期</strong>{{ dateRangeLabel }}</p>
              <p><strong>更新时间</strong>{{ updatedAtLabel }}</p>
              <p><strong>数据范围</strong>全站实名账号</p>
            </section>
          </aside>
        </section>

        <section class="list-panel panel">
          <div class="panel-heading">
            <h2>第 4-10 名</h2>
            <span>{{ metric === 'tokens' ? '按 Token 从高到低' : '按实际花费从高到低' }}</span>
          </div>

          <div v-if="restRanking.length > 0" class="rank-list">
            <article v-for="item in restRanking" :key="item.user_id" class="rank-row">
              <span class="rank-number">{{ item.rank }}</span>
              <div class="rank-row-main">
                <strong>{{ item.display_name }}</strong>
                <span class="metric-track">
                  <i :style="{ width: `${barPercent(item)}%` }"></i>
                </span>
              </div>
              <b>{{ primaryMetricLabel(item) }}</b>
            </article>
          </div>
          <div v-else class="empty-state compact">暂无更多排名</div>
        </section>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
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

const periods: Array<{ value: UsageRankingPeriod; label: string }> = [
  { value: 'day', label: '日榜' },
  { value: 'week', label: '周榜' },
  { value: 'month', label: '月榜' },
]

const periodLabel = computed(() => periods.find((item) => item.value === period.value)?.label ?? '日榜')
const dateRangeLabel = computed(() => {
  if (!startDate.value || !endDate.value) return '当前周期'
  return `${startDate.value} 至 ${endDate.value}`
})
const updatedAtLabel = computed(() => updatedAt.value?.toLocaleString() ?? '-')
const topThree = computed(() => ranking.value.slice(0, 3))
const firstPlace = computed(() => topThree.value[0] ?? null)
const secondPlace = computed(() => topThree.value[1] ?? null)
const thirdPlace = computed(() => topThree.value[2] ?? null)
const restRanking = computed(() => ranking.value.slice(3, 10))
const rankingThreshold = computed(() => ranking.value.length >= 10 ? ranking.value[9] : null)
const maxMetricValue = computed(() => {
  const values = ranking.value.map((item) => metricValue(item))
  return Math.max(...values, 1)
})
const currentMetricValue = computed(() => currentUser.value ? metricValue(currentUser.value) : 0)
const thresholdMetricValue = computed(() => rankingThreshold.value ? metricValue(rankingThreshold.value) : 0)
const minimumGap = computed(() => metric.value === 'tokens' ? 1 : 0.01)
const isCurrentUserRanked = computed(() => Boolean(currentUser.value && currentUser.value.rank <= 10))
const isCurrentUserFirst = computed(() => Boolean(isCurrentUserRanked.value && currentUser.value?.rank === 1))
const mineStatusTone = computed(() => isCurrentUserRanked.value ? 'ranked' : 'unranked')
const mineStatusLabel = computed(() => isCurrentUserRanked.value ? '已上榜' : '未上榜')
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
  if (hasSummitTarget.value) return '已登顶'
  if (isCurrentUserRanked.value && currentUser.value) return `第 ${currentUser.value.rank} 名`
  if (currentUserTarget.value?.target_type === 'threshold') return `距离上榜还差 ${targetGapLabel.value}`
  return `距离上榜还差 ${mineGapLabel.value}`
})
const mineDetail = computed(() => {
  if (hasSummitTarget.value && currentUser.value) {
    return `${primaryMetricLabel(currentUser.value)} · 继续保持当前榜首`
  }
  if (isCurrentUserRanked.value && currentUser.value) {
    if (currentUserTarget.value?.target_type === 'previous') {
      return `${primaryMetricLabel(currentUser.value)} · 距离超过上一名还差 ${targetGapLabel.value}`
    }
    return `${primaryMetricLabel(currentUser.value)} · ${secondaryMetricLabel(currentUser.value)}`
  }
  if (currentUser.value) return `当前 ${formatMetricValue(currentMetricValue.value)}`
  return '当前周期暂无调用'
})
const mineStatSubtitle = computed(() => {
  if (hasSummitTarget.value && currentUser.value) return `已登顶 · ${primaryMetricLabel(currentUser.value)}`
  if (isCurrentUserRanked.value && currentUser.value) {
    if (currentUserTarget.value?.target_type === 'previous') return `距离超过上一名还差 ${targetGapLabel.value}`
    return `第 ${currentUser.value.rank} 名 · ${primaryMetricLabel(currentUser.value)}`
  }
  if (currentUserTarget.value?.target_type === 'threshold') return `距离上榜还差 ${targetGapLabel.value}`
  return `距离上榜还差 ${mineGapLabel.value}`
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
  if (hasSummitTarget.value) return '继续保持当前榜首'
  if (currentUserTarget.value?.target_type === 'previous') {
    return currentUserTarget.value.target_rank
      ? `以第 ${currentUserTarget.value.target_rank} 名作为追赶目标`
      : '距离超过上一名还差一步'
  }
  if (currentUserTarget.value?.target_type === 'threshold') {
    return currentUserTarget.value.target_rank
      ? `以第 ${currentUserTarget.value.target_rank} 名作为上榜门槛`
      : '以当前 Top10 末位作为上榜门槛'
  }
  if (isCurrentUserRanked.value) return '已进入当前 Top10'
  if (rankingThreshold.value) return '以第 10 名作为上榜门槛'
  return '当前榜单未满，产生用量即可上榜'
})

async function loadRankings() {
  loading.value = true
  try {
    const response = await getRankings({
      metric: metric.value,
      period: period.value,
      limit: 10,
    })
    ranking.value = response.ranking ?? []
    summary.value = response.summary ?? summary.value
    currentUser.value = response.current_user ?? null
    currentUserTarget.value = response.current_user_target ?? null
    startDate.value = response.start_date
    endDate.value = response.end_date
    updatedAt.value = new Date()
  } catch (error: any) {
    appStore.showError(error?.message || '排行榜加载失败')
  } finally {
    loading.value = false
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
  return Math.round(value).toLocaleString()
}

function formatUsd(value: number): string {
  return `$${value.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`
}

function metricValue(item: UsageRankingItem): number {
  return metric.value === 'tokens' ? item.total_tokens : item.actual_cost
}

function formatMetricValue(value: number): string {
  if (metric.value !== 'tokens') return formatUsd(value)
  const unit = Math.round(value) === 1 ? 'token' : 'tokens'
  return `${formatTokens(value)} ${unit}`
}

function primaryMetricLabel(item: UsageRankingItem): string {
  return formatMetricValue(metricValue(item))
}

function secondaryMetricLabel(item: UsageRankingItem): string {
  return metric.value === 'tokens'
    ? `${formatUsd(item.actual_cost)} 实际花费`
    : `${formatTokens(item.total_tokens)} tokens`
}

function barPercent(item: UsageRankingItem): number {
  const value = metricValue(item)
  return Math.min(100, Math.max(6, Math.round((value / maxMetricValue.value) * 100)))
}

const RankAvatar = defineComponent({
  props: {
    name: { type: String, required: true },
    tone: { type: String, required: true },
    large: { type: Boolean, default: false },
  },
  setup(props) {
    return () => h('span', {
      class: ['rank-avatar', props.tone, { large: props.large }],
    }, props.name.trim().charAt(0).toUpperCase() || 'U')
  },
})

onMounted(() => {
  void loadRankings()
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
  min-height: 24.375rem;
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
  grid-template-columns: minmax(0, 0.9fr) minmax(0, 1.1fr) minmax(0, 0.9fr);
  gap: 1.125rem;
  align-items: end;
}

.podium-card {
  display: grid;
  min-width: 0;
  justify-items: center;
  align-content: start;
  gap: 0.5rem;
  min-height: 14.75rem;
  padding: 1.125rem;
  border: 1px solid #e2e8f0;
  border-radius: 1rem;
  background: #fff;
  text-align: center;
}

.podium-card.first {
  min-height: 17.875rem;
  padding-top: 1.25rem;
  border: 2px solid #f59e0b;
  background: #fffbeb;
}

.podium-card.second {
  border-color: #94a3b8;
}

.podium-card.third {
  border-color: #c2410c;
}

.crown {
  display: inline-flex;
  width: 2.125rem;
  height: 1.5rem;
  align-items: center;
  justify-content: center;
  border: 1px solid #f59e0b;
  border-radius: 999px;
  background: #fef3c7;
  color: #b45309;
  font-size: 1.125rem;
  line-height: 1;
}

.rank-avatar {
  display: inline-flex;
  width: 3.625rem;
  height: 3.625rem;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  font-size: 1.25rem;
  font-weight: 900;
}

.rank-avatar.large {
  width: 4.75rem;
  height: 4.75rem;
  font-size: 1.7rem;
}

.rank-avatar.gold {
  border: 2px solid #f59e0b;
  background: #fef3c7;
  color: #b45309;
}

.rank-avatar.silver {
  border: 2px solid #94a3b8;
  background: #f1f5f9;
  color: #475569;
}

.rank-avatar.bronze {
  border: 2px solid #c2410c;
  background: #ffedd5;
  color: #9a3412;
}

.medal {
  display: inline-flex;
  min-width: 4.5rem;
  height: 1.75rem;
  align-items: center;
  justify-content: center;
  border-radius: 999px;
  font-size: 0.8125rem;
  font-weight: 900;
}

.medal.gold {
  background: #fef3c7;
  color: #b45309;
}

.medal.silver {
  background: #e2e8f0;
  color: #475569;
}

.medal.bronze {
  background: #fed7aa;
  color: #9a3412;
}

.podium-card strong {
  max-width: 100%;
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
  padding: 1.25rem 1.5rem;
}

.rank-list {
  display: grid;
  gap: 0.625rem;
}

.rank-row {
  display: grid;
  grid-template-columns: 2.125rem minmax(0, 1fr) 10rem;
  gap: 0.875rem;
  align-items: center;
  min-height: 3rem;
  padding: 0 0.875rem;
  border-radius: 0.625rem;
  background: #f8fafc;
}

.rank-number {
  display: inline-flex;
  width: 2.125rem;
  height: 2.125rem;
  align-items: center;
  justify-content: center;
  border-radius: 999px;
  background: #eef2f7;
  color: #64748b;
  font-size: 0.8125rem;
  font-weight: 900;
}

.rank-row-main {
  display: grid;
  min-width: 0;
  gap: 0.3125rem;
}

.rank-row-main strong {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 0.875rem;
}

.metric-track {
  height: 0.375rem;
  overflow: hidden;
  border-radius: 999px;
  background: #e2e8f0;
}

.metric-track i {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: #64748b;
}

.rank-row > b {
  color: #64748b;
  text-align: right;
  font-size: 0.875rem;
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
.dark .rank-row-main strong {
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

.dark .segmented,
.dark .rank-row {
  background: #1f2937;
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
  border-color: #f59e0b;
  background: #2a1f0b;
}

.dark .podium-card.second {
  border-color: #64748b;
  background: #151c28;
}

.dark .podium-card.third {
  border-color: #c2410c;
  background: #25140b;
}

.dark .crown {
  border-color: #f59e0b;
  background: rgb(245 158 11 / 0.18);
  color: #facc15;
}

.dark .rank-avatar.gold,
.dark .medal.gold {
  background: rgb(245 158 11 / 0.18);
  color: #facc15;
}

.dark .rank-avatar.silver,
.dark .medal.silver {
  background: rgb(148 163 184 / 0.16);
  color: #cbd5e1;
}

.dark .rank-avatar.bronze,
.dark .medal.bronze {
  background: rgb(194 65 12 / 0.18);
  color: #fdba74;
}

.dark .podium-card strong,
.dark .podium-card b,
.dark .rank-row > b {
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
  background: #94a3b8;
}

.dark .rank-number {
  background: #273244;
  color: #cbd5e1;
}

@media (max-width: 1180px) {
  .stats-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .ranking-main {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 760px) {
  .selection-summary {
    display: none;
  }

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
    grid-template-columns: 1fr 1fr;
  }

  .podium-card.first {
    grid-column: 1 / -1;
    grid-row: 1;
  }

  .podium-card.second,
  .podium-card.third {
    min-height: 7.75rem;
    padding: 0.75rem;
  }

  .podium-card.second .rank-avatar,
  .podium-card.third .rank-avatar {
    display: none;
  }

  .rank-row {
    grid-template-columns: 2.125rem minmax(0, 1fr);
  }

  .rank-row > b {
    display: none;
  }
}
</style>
