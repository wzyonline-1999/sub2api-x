<template>
  <article
    class="group relative flex h-full flex-col overflow-hidden rounded-[22px] border border-gray-200/80 bg-white shadow-[0_10px_30px_-18px_rgba(15,23,42,0.35)] transition duration-300 hover:-translate-y-0.5 hover:shadow-[0_18px_44px_-20px_rgba(15,23,42,0.4)] dark:border-dark-700/80 dark:bg-dark-800/95"
    data-testid="capacity-group-card"
    :data-group-id="group.group_id"
    :data-pressure-level="groupLevel"
  >
    <div class="absolute inset-x-0 top-0 h-1" :class="overallTone.bar" aria-hidden="true"></div>
    <div
      class="pointer-events-none absolute -right-20 -top-24 h-56 w-56 rounded-full blur-3xl"
      :class="overallTone.glow"
      aria-hidden="true"
    ></div>

    <header class="relative flex items-start justify-between gap-4 px-5 pb-5 pt-6 sm:px-6">
      <div class="flex min-w-0 items-center gap-3.5">
        <div
          class="flex h-11 w-11 flex-shrink-0 items-center justify-center rounded-2xl shadow-sm ring-1 ring-black/5 dark:ring-white/10"
          :class="platformTone"
          data-testid="capacity-platform-icon"
        >
          <PlatformIcon :platform="platformIcon" size="lg" />
        </div>
        <div class="min-w-0">
          <h3 class="truncate text-lg font-semibold tracking-tight text-gray-950 dark:text-white">
            {{ group.name }}
          </h3>
          <p class="mt-0.5 text-xs font-medium text-gray-500 dark:text-dark-400">
            {{ displayPlatform }}
          </p>
        </div>
      </div>
      <span
        class="inline-flex flex-shrink-0 items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-semibold ring-1 ring-inset"
        :class="overallTone.badge"
      >
        <span class="h-1.5 w-1.5 rounded-full" :class="overallTone.bar" aria-hidden="true"></span>
        {{ t(`capacity.status.${groupLevel}`) }}
      </span>
    </header>

    <div class="relative flex flex-1 flex-col px-5 pb-5 sm:px-6 sm:pb-6">
      <section class="capacity-pressure-container pb-5">
        <div
          class="capacity-metric-grid grid gap-3"
          :data-columns="group.account_concurrency ? '2' : '1'"
          data-testid="concurrency-grid"
        >
          <div
            class="min-w-0 rounded-2xl border border-gray-200/80 bg-gradient-to-b from-white to-gray-50/80 p-4 shadow-sm dark:border-dark-700/80 dark:from-dark-800 dark:to-dark-900/70"
            data-testid="group-concurrency"
            :data-level="groupConcurrencyLevel"
          >
            <div class="flex items-center justify-between gap-3">
              <div class="flex min-w-0 items-center gap-2.5">
                <span class="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-xl bg-primary-50 text-primary-600 dark:bg-primary-900/25 dark:text-primary-300">
                  <Icon name="grid" size="sm" />
                </span>
                <p class="truncate text-sm font-semibold text-gray-700 dark:text-dark-200">
                  {{ t('capacity.groupConcurrency') }}
                </p>
              </div>
              <span class="font-mono text-xs font-bold" :class="groupConcurrencyTone.text">
                {{ formatPercentage(group.concurrency.load_percentage) }}%
              </span>
            </div>

            <div class="mt-4 flex items-end justify-between gap-3">
              <p
                class="font-mono text-3xl font-bold leading-none tracking-tight text-gray-950 dark:text-white"
                data-testid="group-concurrency-value"
              >
                {{ group.concurrency.current }}
                <span class="ml-1 text-sm font-semibold text-gray-400 dark:text-dark-500">
                  / {{ group.concurrency.max }}
                </span>
              </p>
              <span class="rounded-lg bg-gray-100 px-2 py-1 text-xs font-semibold dark:bg-dark-700/80" :class="groupConcurrencyTone.text">
                {{ t('capacity.remaining', { count: group.concurrency.remaining }) }}
              </span>
            </div>

            <div
              class="mt-4 h-1.5 overflow-hidden rounded-full bg-gray-200/90 dark:bg-dark-700"
              role="progressbar"
              :aria-label="t('capacity.groupConcurrency')"
              :aria-valuenow="groupConcurrencyPercentage"
              aria-valuemin="0"
              aria-valuemax="100"
              :data-level="groupConcurrencyLevel"
            >
              <div
                class="h-full rounded-full transition-[width] duration-300"
                :class="groupConcurrencyTone.bar"
                :style="{ width: `${groupConcurrencyPercentage}%` }"
              ></div>
            </div>

            <div class="mt-3 flex flex-wrap items-center justify-between gap-x-2 gap-y-1 text-xs">
              <span class="text-gray-500 dark:text-dark-400">
                {{ t('capacity.waiting', { count: group.concurrency.waiting }) }}
              </span>
              <span class="font-medium text-gray-400 dark:text-dark-500">
                {{ t('capacity.groupLoadLabel') }}
              </span>
            </div>
          </div>

          <div
            v-if="group.account_concurrency"
            class="min-w-0 rounded-2xl border border-gray-200/80 bg-gradient-to-b from-white to-gray-50/80 p-4 shadow-sm dark:border-dark-700/80 dark:from-dark-800 dark:to-dark-900/70"
            data-testid="account-concurrency"
            :data-level="accountLevel"
          >
            <div class="flex items-center justify-between gap-3">
              <div class="flex min-w-0 items-center gap-2.5">
                <span class="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-xl bg-violet-50 text-violet-600 dark:bg-violet-900/25 dark:text-violet-300">
                  <Icon name="users" size="sm" />
                </span>
                <p class="truncate text-sm font-semibold text-gray-700 dark:text-dark-200">
                  {{ t('capacity.accountConcurrency') }}
                </p>
              </div>
              <span class="font-mono text-xs font-bold" :class="accountTone.text">
                {{ formatPercentage(group.account_concurrency.load_percentage) }}%
              </span>
            </div>

            <div class="mt-4 flex items-end justify-between gap-3">
              <p
                class="font-mono text-3xl font-bold leading-none tracking-tight text-gray-950 dark:text-white"
                data-testid="account-concurrency-value"
              >
                {{ group.account_concurrency.current }}
                <span class="ml-1 text-sm font-semibold text-gray-400 dark:text-dark-500">
                  / {{ group.account_concurrency.max }}
                </span>
              </p>
              <span class="rounded-lg bg-gray-100 px-2 py-1 text-xs font-semibold dark:bg-dark-700/80" :class="accountTone.text">
                {{ t('capacity.remaining', { count: accountConcurrencyRemaining }) }}
              </span>
            </div>

            <div
              class="mt-4 h-1.5 overflow-hidden rounded-full bg-gray-200/90 dark:bg-dark-700"
              role="progressbar"
              :aria-label="t('capacity.accountConcurrency')"
              :aria-valuenow="accountConcurrencyPercentage"
              aria-valuemin="0"
              aria-valuemax="100"
              :data-level="accountLevel"
            >
              <div
                class="h-full rounded-full transition-[width] duration-300"
                :class="accountTone.bar"
                :style="{ width: `${accountConcurrencyPercentage}%` }"
              ></div>
            </div>

            <div class="mt-3 flex flex-wrap items-center justify-between gap-x-2 gap-y-1 text-xs">
              <span class="text-gray-500 dark:text-dark-400">
                {{ t('capacity.configuredAccounts', { count: group.account_concurrency.configured_accounts }) }}
              </span>
              <span class="font-medium text-gray-400 dark:text-dark-500">
                {{ t('capacity.accountLoadLabel') }}
              </span>
            </div>
          </div>
        </div>

        <div
          v-if="hasQuotaLoad"
          class="mt-5 border-t border-gray-200/80 pt-5 dark:border-dark-700/80"
          data-testid="quota-load"
        >
          <div class="flex items-center justify-between gap-3">
            <div class="flex items-center gap-2.5">
              <span class="flex h-8 w-8 items-center justify-center rounded-xl bg-sky-50 text-sky-600 dark:bg-sky-900/25 dark:text-sky-300">
                <Icon name="chart" size="sm" />
              </span>
              <p class="text-sm font-semibold text-gray-700 dark:text-dark-200">
                {{ t('capacity.quotaLoad') }}
              </p>
            </div>
            <span class="rounded-full bg-gray-100 px-2.5 py-1 text-[11px] font-medium text-gray-500 dark:bg-dark-700/70 dark:text-dark-400">
              {{ t('capacity.lowerIsBetter') }}
            </span>
          </div>

          <div
            class="capacity-metric-grid mt-4 grid gap-3"
            :data-columns="hasBothQuotaWindows ? '2' : '1'"
          >
            <div
              v-if="group.quota_load?.five_hour"
              class="min-w-0 rounded-2xl border border-gray-200/70 bg-gray-50/70 p-4 dark:border-dark-700/70 dark:bg-dark-900/45"
              data-testid="quota-five-hour"
              :data-level="fiveHourLevel"
            >
              <div class="flex items-center justify-between gap-3">
                <div class="flex items-center gap-2 text-sm font-semibold text-gray-600 dark:text-dark-300">
                  <Icon name="clock" size="sm" class="text-gray-400 dark:text-dark-500" />
                  <span>{{ t('capacity.fiveHour') }}</span>
                </div>
                <span
                  class="font-mono text-2xl font-bold tracking-tight"
                  :class="fiveHourTone.text"
                  data-testid="quota-five-hour-value"
                >
                  {{ formatPercentage(group.quota_load.five_hour.load_percentage) }}%
                </span>
              </div>
              <div
                class="mt-4 h-1.5 overflow-hidden rounded-full bg-gray-200/90 dark:bg-dark-700"
                role="progressbar"
                :aria-label="t('capacity.fiveHour')"
                :aria-valuenow="fiveHourPercentage"
                aria-valuemin="0"
                aria-valuemax="100"
                :data-level="fiveHourLevel"
              >
                <div
                  class="h-full rounded-full transition-[width] duration-300"
                  :class="fiveHourTone.bar"
                  :style="{ width: `${fiveHourPercentage}%` }"
                ></div>
              </div>
              <p class="mt-3 text-xs text-gray-500 dark:text-dark-400">
                {{ t('capacity.quotaCoverage', {
                  covered: group.quota_load.five_hour.accounts_with_data,
                  total: group.quota_load.five_hour.total_accounts,
                }) }}
              </p>
            </div>

            <div
              v-if="group.quota_load?.seven_day"
              class="min-w-0 rounded-2xl border border-gray-200/70 bg-gray-50/70 p-4 dark:border-dark-700/70 dark:bg-dark-900/45"
              data-testid="quota-seven-day"
              :data-level="sevenDayLevel"
            >
              <div class="flex items-center justify-between gap-3">
                <div class="flex items-center gap-2 text-sm font-semibold text-gray-600 dark:text-dark-300">
                  <Icon name="calendar" size="sm" class="text-gray-400 dark:text-dark-500" />
                  <span>{{ t('capacity.sevenDay') }}</span>
                </div>
                <span
                  class="font-mono text-2xl font-bold tracking-tight"
                  :class="sevenDayTone.text"
                  data-testid="quota-seven-day-value"
                >
                  {{ formatPercentage(group.quota_load.seven_day.load_percentage) }}%
                </span>
              </div>
              <div
                class="mt-4 h-1.5 overflow-hidden rounded-full bg-gray-200/90 dark:bg-dark-700"
                role="progressbar"
                :aria-label="t('capacity.sevenDay')"
                :aria-valuenow="sevenDayPercentage"
                aria-valuemin="0"
                aria-valuemax="100"
                :data-level="sevenDayLevel"
              >
                <div
                  class="h-full rounded-full transition-[width] duration-300"
                  :class="sevenDayTone.bar"
                  :style="{ width: `${sevenDayPercentage}%` }"
                ></div>
              </div>
              <p class="mt-3 text-xs text-gray-500 dark:text-dark-400">
                {{ t('capacity.quotaCoverage', {
                  covered: group.quota_load.seven_day.accounts_with_data,
                  total: group.quota_load.seven_day.total_accounts,
                }) }}
              </p>
            </div>
          </div>
        </div>
      </section>

      <section
        class="mt-auto overflow-hidden rounded-2xl border p-4 sm:p-5"
        :class="capabilityTone.panel"
        data-testid="load-capability"
        :data-level="capabilityLevel"
      >
        <div class="flex items-center justify-between gap-4">
          <div class="flex min-w-0 items-center gap-3">
            <span class="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-2xl bg-white/75 shadow-sm ring-1 ring-black/5 dark:bg-dark-800/70 dark:ring-white/10">
              <Icon name="bolt" size="md" :class="capabilityTone.text" />
            </span>
            <div class="min-w-0">
              <p class="text-sm font-semibold text-gray-700 dark:text-dark-200">
                {{ t('capacity.loadCapability') }}
              </p>
              <p class="mt-0.5 truncate text-xs font-medium" :class="capabilityTone.text">
                {{ t('capacity.schedulable', {
                  available: group.load_capacity.available,
                  total: group.load_capacity.total,
                }) }}
                · {{ t('capacity.higherIsBetter') }}
              </p>
            </div>
          </div>
          <p class="flex-shrink-0 font-mono text-3xl font-bold tracking-tight" :class="capabilityTone.text">
            {{ formatPercentage(group.load_capacity.percentage) }}%
          </p>
        </div>
        <div
          class="mt-4 h-2 overflow-hidden rounded-full bg-white/80 shadow-inner dark:bg-dark-900/45"
          role="progressbar"
          :aria-label="t('capacity.loadCapability')"
          :aria-valuenow="loadCapabilityPercentage"
          aria-valuemin="0"
          aria-valuemax="100"
        >
          <div
            class="h-full rounded-full transition-[width] duration-300"
            :class="capabilityTone.bar"
            :style="{ width: `${loadCapabilityPercentage}%` }"
          ></div>
        </div>
      </section>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { VisibleCapacityGroup } from '@/api/capacity'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import Icon from '@/components/icons/Icon.vue'
import type { GroupPlatform } from '@/types'
import {
  clampPercentage,
  concurrencyLevel,
  loadCapabilityLevel,
  platformLabel,
  worstCapacityLevel,
  type CapacityLevel,
} from './capacityLevels'

const props = defineProps<{
  group: VisibleCapacityGroup
}>()

const { t } = useI18n()

const tones: Record<CapacityLevel, {
  badge: string
  bar: string
  glow: string
  text: string
  panel: string
}> = {
  healthy: {
    badge: 'bg-emerald-50 text-emerald-700 ring-emerald-200/80 dark:bg-emerald-900/30 dark:text-emerald-300 dark:ring-emerald-800/70',
    bar: 'bg-emerald-500',
    glow: 'bg-emerald-200/35 dark:bg-emerald-700/10',
    text: 'text-emerald-700 dark:text-emerald-300',
    panel: 'border-emerald-200/80 bg-gradient-to-r from-emerald-50 to-teal-50/70 dark:border-emerald-800/60 dark:from-emerald-900/25 dark:to-teal-900/15',
  },
  warning: {
    badge: 'bg-amber-50 text-amber-700 ring-amber-200/80 dark:bg-amber-900/30 dark:text-amber-300 dark:ring-amber-800/70',
    bar: 'bg-amber-500',
    glow: 'bg-amber-200/35 dark:bg-amber-700/10',
    text: 'text-amber-700 dark:text-amber-300',
    panel: 'border-amber-200/80 bg-gradient-to-r from-amber-50 to-orange-50/70 dark:border-amber-800/60 dark:from-amber-900/25 dark:to-orange-900/15',
  },
  critical: {
    badge: 'bg-red-50 text-red-700 ring-red-200/80 dark:bg-red-900/30 dark:text-red-300 dark:ring-red-800/70',
    bar: 'bg-red-500',
    glow: 'bg-red-200/35 dark:bg-red-700/10',
    text: 'text-red-700 dark:text-red-300',
    panel: 'border-red-200/80 bg-gradient-to-r from-red-50 to-rose-50/70 dark:border-red-800/60 dark:from-red-900/25 dark:to-rose-900/15',
  },
}

const platformTones: Partial<Record<GroupPlatform, string>> = {
  anthropic: 'bg-[#D97757] text-white',
  openai: 'bg-gray-950 text-white dark:bg-white dark:text-gray-950',
  gemini: 'bg-gradient-to-br from-blue-500 to-violet-500 text-white',
  antigravity: 'bg-gradient-to-br from-cyan-500 to-blue-600 text-white',
  grok: 'bg-gray-950 text-white dark:bg-white dark:text-gray-950',
}

const groupConcurrencyLevel = computed(() => concurrencyLevel(
  props.group.concurrency.load_percentage,
  props.group.concurrency.waiting,
))
const accountLevel = computed<CapacityLevel | null>(() => props.group.account_concurrency
  ? concurrencyLevel(props.group.account_concurrency.load_percentage)
  : null)
const fiveHourLevel = computed<CapacityLevel | null>(() => props.group.quota_load?.five_hour
  ? concurrencyLevel(props.group.quota_load.five_hour.load_percentage)
  : null)
const sevenDayLevel = computed<CapacityLevel | null>(() => props.group.quota_load?.seven_day
  ? concurrencyLevel(props.group.quota_load.seven_day.load_percentage)
  : null)
const capabilityLevel = computed(() => loadCapabilityLevel(props.group.load_capacity.percentage))
const groupLevel = computed(() => worstCapacityLevel(
  groupConcurrencyLevel.value,
  accountLevel.value,
  fiveHourLevel.value,
  sevenDayLevel.value,
  capabilityLevel.value,
))

const overallTone = computed(() => tones[groupLevel.value])
const groupConcurrencyTone = computed(() => tones[groupConcurrencyLevel.value])
const accountTone = computed(() => tones[accountLevel.value ?? 'healthy'])
const fiveHourTone = computed(() => tones[fiveHourLevel.value ?? 'healthy'])
const sevenDayTone = computed(() => tones[sevenDayLevel.value ?? 'healthy'])
const capabilityTone = computed(() => tones[capabilityLevel.value])
const displayPlatform = computed(() => platformLabel(props.group.platform))
const platformIcon = computed(() => props.group.platform.trim().toLowerCase() as GroupPlatform)
const platformTone = computed(() => platformTones[platformIcon.value]
  ?? 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-300')
const groupConcurrencyPercentage = computed(() => clampPercentage(props.group.concurrency.load_percentage))
const accountConcurrencyPercentage = computed(() => clampPercentage(
  props.group.account_concurrency?.load_percentage ?? 0,
))
const accountConcurrencyRemaining = computed(() => Math.max(
  0,
  (props.group.account_concurrency?.max ?? 0) - (props.group.account_concurrency?.current ?? 0),
))
const fiveHourPercentage = computed(() => clampPercentage(
  props.group.quota_load?.five_hour?.load_percentage ?? 0,
))
const sevenDayPercentage = computed(() => clampPercentage(
  props.group.quota_load?.seven_day?.load_percentage ?? 0,
))
const loadCapabilityPercentage = computed(() => clampPercentage(props.group.load_capacity.percentage))
const hasQuotaLoad = computed(() => Boolean(
  props.group.quota_load?.five_hour || props.group.quota_load?.seven_day,
))
const hasBothQuotaWindows = computed(() => Boolean(
  props.group.quota_load?.five_hour && props.group.quota_load?.seven_day,
))

function formatPercentage(value: number): number {
  return Math.round(clampPercentage(value))
}
</script>

<style scoped>
.capacity-pressure-container {
  container-type: inline-size;
}

.capacity-metric-grid {
  grid-template-columns: minmax(0, 1fr);
}

@container (min-width: 520px) {
  .capacity-metric-grid[data-columns='2'] {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
