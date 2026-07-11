<template>
  <AppLayout>
    <div class="mx-auto max-w-[1280px] space-y-6">
      <section class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div class="flex min-w-0 items-center gap-2 text-xs font-medium">
          <span
            class="h-2 w-2 flex-shrink-0 rounded-full"
            :class="errorMessage ? 'bg-red-500' : 'bg-emerald-500'"
            aria-hidden="true"
          ></span>
          <span
            class="truncate"
            :class="errorMessage ? 'text-red-600 dark:text-red-400' : 'text-gray-500 dark:text-dark-400'"
            data-testid="capacity-refresh-status"
          >
            {{ t('capacity.autoRefresh', { seconds: AUTO_REFRESH_SECONDS }) }} · {{ refreshStatusText }}
          </span>
        </div>
        <button
          type="button"
          class="btn btn-secondary btn-icon self-start sm:self-auto"
          :disabled="isRefreshing"
          :aria-label="t('common.refresh')"
          :title="t('common.refresh')"
          data-testid="capacity-refresh"
          @click="manualReload"
        >
          <Icon name="refresh" size="sm" :class="isRefreshing ? 'animate-spin' : ''" />
        </button>
      </section>

      <section class="flex flex-col gap-4 xl:flex-row xl:items-center xl:justify-between">
        <div class="flex min-w-0 items-center gap-2 overflow-x-auto pb-1 scrollbar-hide">
          <span class="flex-shrink-0 text-xs font-semibold text-gray-500 dark:text-dark-400">
            {{ t('capacity.platform') }}
          </span>
          <button
            type="button"
            class="capacity-filter-chip"
            :class="selectedPlatform === 'all' ? 'capacity-filter-chip-active' : 'capacity-filter-chip-idle'"
            data-testid="capacity-platform-all"
            @click="selectedPlatform = 'all'"
          >
            {{ t('capacity.allPlatforms') }} {{ groups.length }}
          </button>
          <button
            v-for="option in platformOptions"
            :key="option.key"
            type="button"
            class="capacity-filter-chip"
            :class="selectedPlatform === option.key ? 'capacity-filter-chip-active' : 'capacity-filter-chip-idle'"
            :data-testid="`capacity-platform-${option.key}`"
            @click="selectedPlatform = option.key"
          >
            {{ option.label }} {{ option.count }}
          </button>
        </div>

        <div class="flex w-full flex-col gap-3 sm:flex-row xl:w-auto">
          <label class="relative block w-full sm:min-w-[260px]">
            <span class="sr-only">{{ t('capacity.searchPlaceholder') }}</span>
            <Icon
              name="search"
              size="sm"
              class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400"
            />
            <input
              v-model="searchQuery"
              type="search"
              class="input h-10 pl-10"
              :placeholder="t('capacity.searchPlaceholder')"
              data-testid="capacity-search"
            />
          </label>
          <label class="relative block min-w-[150px]">
            <span class="sr-only">{{ t('capacity.sortLabel') }}</span>
            <Icon
              name="sort"
              size="sm"
              class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400"
            />
            <select
              v-model="sortMode"
              class="input h-10 appearance-none pl-10 pr-8 text-xs font-medium"
              data-testid="capacity-sort"
            >
              <option value="pressure">{{ t('capacity.sortPressure') }}</option>
              <option value="default">{{ t('capacity.sortDefault') }}</option>
            </select>
            <Icon
              name="chevronDown"
              size="xs"
              class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-gray-400"
            />
          </label>
        </div>
      </section>

      <template v-if="initialLoading">
        <div data-testid="capacity-skeleton" class="space-y-6">
          <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
            <div v-for="index in 4" :key="`summary-${index}`" class="card p-5">
              <Skeleton width="45%" height="16px" />
              <Skeleton class="mt-5" width="62%" height="34px" />
              <Skeleton class="mt-4" width="78%" height="12px" />
            </div>
          </div>
          <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
            <div v-for="index in 2" :key="`group-${index}`" class="card p-6">
              <Skeleton width="42%" height="22px" />
              <Skeleton class="mt-6" width="100%" height="150px" />
              <Skeleton class="mt-4" width="100%" height="92px" />
            </div>
          </div>
        </div>
      </template>

      <section
        v-else-if="errorMessage && groups.length === 0"
        class="card border-red-100 dark:border-red-900/40"
        data-testid="capacity-error"
      >
        <EmptyState
          :title="t('capacity.errorTitle')"
          :description="errorMessage"
          :action-text="t('capacity.retry')"
          :action-icon="false"
          @action="manualReload"
        >
          <template #icon>
            <Icon name="exclamationTriangle" size="xl" class="text-red-400" />
          </template>
        </EmptyState>
      </section>

      <template v-else>
        <section
          v-if="groups.length > 0"
          class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4"
          data-testid="capacity-summary"
        >
          <article class="capacity-summary-card">
            <div class="capacity-summary-icon bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300">
              <Icon name="grid" size="md" />
            </div>
            <div class="min-w-0">
              <p class="capacity-summary-label">{{ t('capacity.summary.availableConcurrency') }}</p>
              <p class="capacity-summary-value text-primary-700 dark:text-primary-300">
                {{ summary.remaining }}
                <span class="capacity-summary-suffix">/ {{ summary.maxConcurrency }}</span>
              </p>
              <p class="capacity-summary-note">
                {{ t('capacity.summary.availableConcurrencyNote', {
                  percentage: summary.remainingPercentage,
                  current: summary.currentConcurrency,
                }) }}
              </p>
            </div>
          </article>

          <article class="capacity-summary-card">
            <div class="capacity-summary-icon bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300">
              <Icon name="users" size="md" />
            </div>
            <div class="min-w-0">
              <p class="capacity-summary-label">{{ t('capacity.summary.loadCapability') }}</p>
              <p class="capacity-summary-value" :class="summaryCapabilityTone">
                {{ summary.loadCapabilityPercentage }}%
                <span class="capacity-summary-suffix">
                  {{ summary.availableResources }} / {{ summary.totalResources }}
                </span>
              </p>
              <p class="capacity-summary-note">{{ t('capacity.summary.loadCapabilityNote') }}</p>
            </div>
          </article>

          <article class="capacity-summary-card">
            <div class="capacity-summary-icon bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300">
              <Icon name="clock" size="md" />
            </div>
            <div class="min-w-0">
              <p class="capacity-summary-label">{{ t('capacity.summary.waiting') }}</p>
              <p class="capacity-summary-value text-amber-700 dark:text-amber-300">
                {{ summary.waiting }}
                <span class="capacity-summary-suffix">{{ t('capacity.summary.requests') }}</span>
              </p>
              <p class="capacity-summary-note">
                {{ summary.waiting > 0 ? t('capacity.summary.waitingNote', { groups: summary.queuedGroups }) : t('capacity.summary.noWaiting') }}
              </p>
            </div>
          </article>

          <article class="capacity-summary-card">
            <div class="capacity-summary-icon bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300">
              <Icon name="exclamationCircle" size="md" />
            </div>
            <div class="min-w-0">
              <p class="capacity-summary-label">{{ t('capacity.summary.stressed') }}</p>
              <p class="capacity-summary-value text-red-700 dark:text-red-300">
                {{ summary.stressed }}
                <span class="capacity-summary-suffix">/ {{ filteredGroups.length }}</span>
              </p>
              <p class="capacity-summary-note">
                {{ t('capacity.summary.stressedNote', {
                  warning: summary.warningGroups,
                  critical: summary.criticalGroups,
                }) }}
              </p>
            </div>
          </article>
        </section>

        <section v-if="groups.length === 0" class="card" data-testid="capacity-empty">
          <EmptyState
            :title="t('capacity.emptyTitle')"
            :description="t('capacity.emptyDescription')"
          >
            <template #icon>
              <Icon name="server" size="xl" class="text-gray-300 dark:text-dark-500" />
            </template>
          </EmptyState>
        </section>

        <section v-else class="space-y-4">
          <header class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div class="flex items-baseline gap-2">
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t('capacity.visibleGroups') }}
              </h2>
              <span class="text-xs text-gray-400 dark:text-dark-500">
                {{ t('capacity.groupCount', { count: filteredGroups.length }) }}
              </span>
            </div>
            <div class="flex flex-wrap items-center gap-x-4 gap-y-2 text-[11px] font-medium text-gray-500 dark:text-dark-400">
              <span class="capacity-legend"><i class="bg-emerald-500"></i>{{ t('capacity.legend.healthy') }}</span>
              <span class="capacity-legend"><i class="bg-amber-500"></i>{{ t('capacity.legend.warning') }}</span>
              <span class="capacity-legend"><i class="bg-red-500"></i>{{ t('capacity.legend.critical') }}</span>
            </div>
          </header>

          <div
            v-if="filteredGroups.length > 0"
            class="grid gap-4"
            :class="gridClass"
            data-testid="capacity-grid"
          >
            <CapacityGroupCard
              v-for="group in filteredGroups"
              :key="group.group_id"
              :group="group"
            />
          </div>

          <div v-else class="card" data-testid="capacity-filter-empty">
            <EmptyState
              :title="t('capacity.noMatchesTitle')"
              :description="t('capacity.noMatchesDescription')"
            >
              <template #icon>
                <Icon name="search" size="xl" class="text-gray-300 dark:text-dark-500" />
              </template>
            </EmptyState>
          </div>
        </section>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Skeleton from '@/components/common/Skeleton.vue'
import Icon from '@/components/icons/Icon.vue'
import CapacityGroupCard from '@/components/user/capacity/CapacityGroupCard.vue'
import capacityAPI, { type VisibleCapacityGroup } from '@/api/capacity'
import { extractApiErrorMessage } from '@/utils/apiError'
import { useAutoRefresh } from '@/composables/useAutoRefresh'
import {
  concurrencyLevel,
  loadCapabilityLevel,
  platformLabel,
  worstCapacityLevel,
  type CapacityLevel,
} from '@/components/user/capacity/capacityLevels'

const AUTO_REFRESH_SECONDS = 30

const { t } = useI18n()

const groups = ref<VisibleCapacityGroup[]>([])
const collectedAt = ref('')
const loading = ref(false)
const hasLoaded = ref(false)
const errorMessage = ref('')
const searchQuery = ref('')
const selectedPlatform = ref('all')
const sortMode = ref<'pressure' | 'default'>('pressure')

let abortController: AbortController | null = null

const autoRefresh = useAutoRefresh({
  storageKey: 'capacity-visible-auto-refresh',
  intervals: [AUTO_REFRESH_SECONDS] as const,
  defaultInterval: AUTO_REFRESH_SECONDS,
  onRefresh: () => reload(true),
  shouldPause: () => document.hidden || loading.value,
})

const isRefreshing = computed(() => loading.value || autoRefresh.fetching.value)
const initialLoading = computed(() => loading.value && !hasLoaded.value)

const refreshStatusText = computed(() => {
  if (errorMessage.value) return t('capacity.refreshFailed')
  if (!collectedAt.value) return t('capacity.updating')

  const collected = new Date(collectedAt.value).getTime()
  if (!Number.isFinite(collected)) return t('capacity.justUpdated')
  const secondsAgo = Math.max(0, Math.round((Date.now() - collected) / 1000))
  if (secondsAgo < 60) return t('capacity.justUpdated')
  return t('capacity.updatedAt', {
    time: new Date(collected).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
  })
})

const platformOptions = computed(() => {
  const counts = new Map<string, { label: string; count: number }>()
  for (const group of groups.value) {
    const key = normalizePlatform(group.platform)
    const current = counts.get(key)
    if (current) current.count += 1
    else counts.set(key, { label: platformLabel(group.platform), count: 1 })
  }
  return Array.from(counts.entries())
    .map(([key, value]) => ({ key, ...value }))
    .sort((a, b) => a.label.localeCompare(b.label))
})

const filteredGroups = computed(() => {
  const query = searchQuery.value.trim().toLowerCase()
  const items = groups.value.filter((group) => {
    const platformMatches = selectedPlatform.value === 'all'
      || normalizePlatform(group.platform) === selectedPlatform.value
    if (!platformMatches) return false
    if (!query) return true
    return group.name.toLowerCase().includes(query)
      || platformLabel(group.platform).toLowerCase().includes(query)
  })

  if (sortMode.value === 'default') return items
  return [...items].sort((a, b) => {
    const scoreDelta = pressureScore(b) - pressureScore(a)
    if (scoreDelta !== 0) return scoreDelta
    const loadDelta = b.concurrency.load_percentage - a.concurrency.load_percentage
    if (loadDelta !== 0) return loadDelta
    return a.name.localeCompare(b.name)
  })
})

const gridClass = computed(() => {
  const count = filteredGroups.value.length
  if (count <= 1) return 'grid-cols-1'
  if (count === 2) return 'grid-cols-1 md:grid-cols-2'
  if (count === 3) return 'grid-cols-1 md:grid-cols-2 xl:grid-cols-3'
  return 'grid-cols-1 md:grid-cols-2'
})

const summary = computed(() => {
  let currentConcurrency = 0
  let maxConcurrency = 0
  let remaining = 0
  let waiting = 0
  let queuedGroups = 0
  let availableResources = 0
  let totalResources = 0
  let warningGroups = 0
  let criticalGroups = 0

  for (const group of filteredGroups.value) {
    currentConcurrency += safeNumber(group.concurrency.current)
    maxConcurrency += safeNumber(group.concurrency.max)
    remaining += safeNumber(group.concurrency.remaining)
    waiting += safeNumber(group.concurrency.waiting)
    if (group.concurrency.waiting > 0) queuedGroups += 1
    availableResources += safeNumber(group.load_capacity.available)
    totalResources += safeNumber(group.load_capacity.total)

    const level = pressureLevelForGroup(group)
    if (level === 'warning') warningGroups += 1
    if (level === 'critical') criticalGroups += 1
  }

  return {
    currentConcurrency,
    maxConcurrency,
    remaining,
    remainingPercentage: maxConcurrency > 0 ? Math.round((remaining / maxConcurrency) * 100) : 0,
    waiting,
    queuedGroups,
    availableResources,
    totalResources,
    loadCapabilityPercentage: totalResources > 0
      ? Math.round((availableResources / totalResources) * 100)
      : 0,
    warningGroups,
    criticalGroups,
    stressed: warningGroups + criticalGroups,
  }
})

const summaryCapabilityTone = computed(() => {
  const tones: Record<CapacityLevel, string> = {
    healthy: 'text-emerald-700 dark:text-emerald-300',
    warning: 'text-amber-700 dark:text-amber-300',
    critical: 'text-red-700 dark:text-red-300',
  }
  return tones[loadCapabilityLevel(summary.value.loadCapabilityPercentage)]
})

function normalizePlatform(platform: string): string {
  return platform.trim().toLowerCase() || 'unknown'
}

function safeNumber(value: number): number {
  return Number.isFinite(value) ? Math.max(0, value) : 0
}

function levelWeight(level: CapacityLevel): number {
  if (level === 'critical') return 3
  if (level === 'warning') return 2
  return 1
}

function pressureLevelForGroup(group: VisibleCapacityGroup): CapacityLevel {
  return worstCapacityLevel(
    concurrencyLevel(group.concurrency.load_percentage, group.concurrency.waiting),
    group.account_concurrency
      ? concurrencyLevel(group.account_concurrency.load_percentage)
      : null,
    group.quota_load?.five_hour
      ? concurrencyLevel(group.quota_load.five_hour.load_percentage)
      : null,
    group.quota_load?.seven_day
      ? concurrencyLevel(group.quota_load.seven_day.load_percentage)
      : null,
    loadCapabilityLevel(group.load_capacity.percentage),
  )
}

function pressureScore(group: VisibleCapacityGroup): number {
  const metricPressure = Math.max(
    safeNumber(group.concurrency.load_percentage),
    safeNumber(group.account_concurrency?.load_percentage ?? 0),
    safeNumber(group.quota_load?.five_hour?.load_percentage ?? 0),
    safeNumber(group.quota_load?.seven_day?.load_percentage ?? 0),
    Math.max(0, 100 - safeNumber(group.load_capacity.percentage)),
  )
  const queueBoost = group.concurrency.waiting > 0 ? 1_000 : 0
  return queueBoost
    + levelWeight(pressureLevelForGroup(group)) * 100
    + metricPressure
}

function armAutoRefresh() {
  // useAutoRefresh counts down once per second and refreshes on the tick after
  // zero. Starting at 29 keeps the actual refresh cadence at exactly 30s.
  autoRefresh.countdown.value = AUTO_REFRESH_SECONDS - 1
}

async function reload(silent: boolean) {
  abortController?.abort()
  const controller = new AbortController()
  abortController = controller
  if (!silent) loading.value = true

  try {
    const snapshot = await capacityAPI.getVisible({ signal: controller.signal })
    if (controller.signal.aborted || abortController !== controller) return
    groups.value = Array.isArray(snapshot.groups) ? snapshot.groups : []
    collectedAt.value = snapshot.collected_at || new Date().toISOString()
    errorMessage.value = ''
  } catch (error: unknown) {
    const candidate = error as { name?: string; code?: string }
    if (candidate?.name === 'AbortError' || candidate?.code === 'ERR_CANCELED') return
    errorMessage.value = extractApiErrorMessage(error, t('capacity.loadError'))
  } finally {
    if (abortController === controller) {
      hasLoaded.value = true
      if (!silent) loading.value = false
      abortController = null
      armAutoRefresh()
    }
  }
}

async function manualReload() {
  await reload(false)
}

onMounted(() => {
  autoRefresh.setEnabled(true)
  armAutoRefresh()
  void reload(false)
})

onBeforeUnmount(() => {
  abortController?.abort()
})
</script>

<style scoped>
.capacity-filter-chip {
  @apply flex-shrink-0 rounded-xl border px-3 py-2 text-xs font-medium transition-colors;
}

.capacity-filter-chip-active {
  @apply border-primary-500 bg-primary-500 text-white shadow-sm;
}

.capacity-filter-chip-idle {
  @apply border-gray-200 bg-white text-gray-600 hover:border-gray-300 hover:bg-gray-50 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-300 dark:hover:bg-dark-700;
}

.capacity-summary-card {
  @apply flex min-h-[138px] items-start gap-4 rounded-2xl border border-gray-200 bg-white p-5 shadow-card dark:border-dark-700 dark:bg-dark-800/80;
}

.capacity-summary-icon {
  @apply flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl;
}

.capacity-summary-label {
  @apply text-sm font-semibold text-gray-600 dark:text-dark-300;
}

.capacity-summary-value {
  @apply mt-3 whitespace-nowrap text-3xl font-bold;
}

.capacity-summary-suffix {
  @apply ml-1 text-xs font-medium text-gray-500 dark:text-dark-400;
}

.capacity-summary-note {
  @apply mt-3 text-xs leading-5 text-gray-400 dark:text-dark-500;
}

.capacity-legend {
  @apply inline-flex items-center gap-1.5;
}

.capacity-legend i {
  @apply h-2 w-2 rounded-full;
}
</style>
