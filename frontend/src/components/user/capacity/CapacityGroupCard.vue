<template>
  <article
    class="flex h-full flex-col rounded-2xl border border-gray-200 bg-white p-5 shadow-card transition-shadow hover:shadow-card-hover dark:border-dark-700 dark:bg-dark-800/80 sm:p-6"
    data-testid="capacity-group-card"
    :data-group-id="group.group_id"
    :data-pressure-level="groupLevel"
  >
    <header class="mb-5 flex items-start justify-between gap-4">
      <div class="min-w-0">
        <h3 class="truncate text-lg font-semibold text-gray-900 dark:text-white">
          {{ group.name }}
        </h3>
        <div class="mt-1 flex items-center gap-1.5 text-xs font-medium text-gray-500 dark:text-dark-400">
          <span class="h-2 w-2 rounded-full bg-primary-500" aria-hidden="true"></span>
          <span>{{ displayPlatform }}</span>
        </div>
      </div>
      <span
        class="inline-flex flex-shrink-0 rounded-lg px-2.5 py-1 text-xs font-semibold"
        :class="overallTone.badge"
      >
        {{ t(`capacity.status.${groupLevel}`) }}
      </span>
    </header>

    <section class="capacity-pressure-container rounded-xl bg-gray-50 p-4 dark:bg-dark-900/60">
      <div
        class="capacity-metric-grid grid gap-3"
        :data-columns="group.account_concurrency ? '2' : '1'"
        data-testid="concurrency-grid"
      >
        <div
          class="min-w-0 rounded-lg bg-white/80 p-3 dark:bg-dark-800/70"
          data-testid="group-concurrency"
          :data-level="groupConcurrencyLevel"
        >
          <p class="text-sm font-semibold text-gray-600 dark:text-dark-300">
            {{ t('capacity.groupConcurrency') }}
          </p>
          <p
            class="mt-2 font-mono text-xl font-bold text-gray-900 dark:text-white"
            data-testid="group-concurrency-value"
          >
            {{ group.concurrency.current }} / {{ group.concurrency.max }}
          </p>

          <div
            class="mt-3 h-2 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700"
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

          <div class="mt-2.5 flex items-center justify-between gap-2 text-xs">
            <span class="text-gray-500 dark:text-dark-400">
              {{ t('capacity.groupLoadLabel') }}
            </span>
            <span class="font-mono font-semibold" :class="groupConcurrencyTone.text">
              {{ formatPercentage(group.concurrency.load_percentage) }}%
            </span>
          </div>
          <div class="mt-1 flex flex-wrap items-center justify-between gap-x-2 gap-y-1 text-xs">
            <span class="text-gray-500 dark:text-dark-400">
              {{ t('capacity.waiting', { count: group.concurrency.waiting }) }}
            </span>
            <span class="font-semibold" :class="groupConcurrencyTone.text">
              {{ t('capacity.remaining', { count: group.concurrency.remaining }) }}
            </span>
          </div>
        </div>

        <div
          v-if="group.account_concurrency"
          class="min-w-0 rounded-lg bg-white/80 p-3 dark:bg-dark-800/70"
          data-testid="account-concurrency"
          :data-level="accountLevel"
        >
          <p class="text-sm font-semibold text-gray-600 dark:text-dark-300">
            {{ t('capacity.accountConcurrency') }}
          </p>
          <p
            class="mt-2 font-mono text-xl font-bold text-gray-900 dark:text-white"
            data-testid="account-concurrency-value"
          >
            {{ group.account_concurrency.current }} / {{ group.account_concurrency.max }}
          </p>

          <div
            class="mt-3 h-2 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700"
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

          <div class="mt-2.5 flex items-center justify-between gap-2 text-xs">
            <span class="text-gray-500 dark:text-dark-400">
              {{ t('capacity.accountLoadLabel') }}
            </span>
            <span class="font-mono font-semibold" :class="accountTone.text">
              {{ formatPercentage(group.account_concurrency.load_percentage) }}%
            </span>
          </div>
          <div class="mt-1 flex flex-wrap items-center justify-between gap-x-2 gap-y-1 text-xs">
            <span class="text-gray-500 dark:text-dark-400">
              {{ t('capacity.configuredAccounts', { count: group.account_concurrency.configured_accounts }) }}
            </span>
            <span class="font-semibold" :class="accountTone.text">
              {{ t('capacity.remaining', { count: accountConcurrencyRemaining }) }}
            </span>
          </div>
        </div>
      </div>

      <div
        v-if="hasQuotaLoad"
        class="mt-4 border-t border-gray-200 pt-4 dark:border-dark-700"
        data-testid="quota-load"
      >
        <p class="text-sm font-semibold text-gray-600 dark:text-dark-300">
          {{ t('capacity.quotaLoad') }}
          <span class="font-normal text-gray-500 dark:text-dark-400">
            · {{ t('capacity.lowerIsBetter') }}
          </span>
        </p>

        <div
          class="capacity-metric-grid mt-3 grid gap-3"
          :data-columns="hasBothQuotaWindows ? '2' : '1'"
        >
          <div
            v-if="group.quota_load?.five_hour"
            class="min-w-0 rounded-lg bg-white/80 p-3 dark:bg-dark-800/70"
            data-testid="quota-five-hour"
            :data-level="fiveHourLevel"
          >
            <div class="flex items-end justify-between gap-3">
              <span class="text-sm font-semibold text-gray-600 dark:text-dark-300">
                {{ t('capacity.fiveHour') }}
              </span>
              <span
                class="font-mono text-xl font-bold"
                :class="fiveHourTone.text"
                data-testid="quota-five-hour-value"
              >
                {{ formatPercentage(group.quota_load.five_hour.load_percentage) }}%
              </span>
            </div>
            <div
              class="mt-3 h-2 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700"
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
            <p class="mt-2.5 text-xs text-gray-500 dark:text-dark-400">
              {{ t('capacity.quotaCoverage', {
                covered: group.quota_load.five_hour.accounts_with_data,
                total: group.quota_load.five_hour.total_accounts,
              }) }}
            </p>
          </div>

          <div
            v-if="group.quota_load?.seven_day"
            class="min-w-0 rounded-lg bg-white/80 p-3 dark:bg-dark-800/70"
            data-testid="quota-seven-day"
            :data-level="sevenDayLevel"
          >
            <div class="flex items-end justify-between gap-3">
              <span class="text-sm font-semibold text-gray-600 dark:text-dark-300">
                {{ t('capacity.sevenDay') }}
              </span>
              <span
                class="font-mono text-xl font-bold"
                :class="sevenDayTone.text"
                data-testid="quota-seven-day-value"
              >
                {{ formatPercentage(group.quota_load.seven_day.load_percentage) }}%
              </span>
            </div>
            <div
              class="mt-3 h-2 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700"
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
            <p class="mt-2.5 text-xs text-gray-500 dark:text-dark-400">
              {{ t('capacity.quotaCoverage', {
                covered: group.quota_load.seven_day.accounts_with_data,
                total: group.quota_load.seven_day.total_accounts,
              }) }}
            </p>
          </div>
        </div>
      </div>
    </section>

    <div class="my-4 h-px bg-gray-200 dark:bg-dark-700" aria-hidden="true"></div>

    <section
      class="mt-auto rounded-xl p-4"
      :class="capabilityTone.panel"
      data-testid="load-capability"
      :data-level="capabilityLevel"
    >
      <div class="flex items-end justify-between gap-4">
        <div>
          <p class="text-xs font-medium text-gray-500 dark:text-dark-300">
            {{ t('capacity.loadCapability') }} · {{ t('capacity.higherIsBetter') }}
          </p>
          <p class="mt-1 text-2xl font-bold" :class="capabilityTone.text">
            {{ formatPercentage(group.load_capacity.percentage) }}%
          </p>
        </div>
        <p class="text-right text-xs font-semibold" :class="capabilityTone.text">
          {{ t('capacity.schedulable', {
            available: group.load_capacity.available,
            total: group.load_capacity.total,
          }) }}
        </p>
      </div>
    </section>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { VisibleCapacityGroup } from '@/api/capacity'
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
  text: string
  panel: string
}> = {
  healthy: {
    badge: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300',
    bar: 'bg-emerald-500',
    text: 'text-emerald-700 dark:text-emerald-300',
    panel: 'bg-emerald-50 dark:bg-emerald-900/20',
  },
  warning: {
    badge: 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300',
    bar: 'bg-amber-500',
    text: 'text-amber-700 dark:text-amber-300',
    panel: 'bg-amber-50 dark:bg-amber-900/20',
  },
  critical: {
    badge: 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300',
    bar: 'bg-red-500',
    text: 'text-red-700 dark:text-red-300',
    panel: 'bg-red-50 dark:bg-red-900/20',
  },
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
