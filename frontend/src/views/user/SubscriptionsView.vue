<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Loading State -->
      <div v-if="loading" class="flex justify-center py-12">
        <div
          class="h-8 w-8 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"
        ></div>
      </div>

      <template v-else>
        <!-- Reset Card Wallet -->
        <section
          v-if="resetCardInventory.length > 0"
          class="card overflow-hidden"
          data-testid="reset-card-wallet"
        >
          <div
            class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-100 px-5 py-4 dark:border-dark-700"
          >
            <div class="flex items-center gap-3">
              <div
                class="flex h-10 w-10 items-center justify-center rounded-xl bg-violet-100 text-violet-600 dark:bg-violet-900/30 dark:text-violet-300"
              >
                <Icon name="gift" size="md" />
              </div>
              <div>
                <h2 class="font-semibold text-gray-900 dark:text-white">
                  {{ t('userSubscriptions.resetCards.title') }}
                </h2>
                <p class="text-xs text-gray-500 dark:text-dark-400">
                  {{ t('userSubscriptions.resetCards.description') }}
                </p>
              </div>
            </div>
            <span
              class="rounded-full bg-violet-50 px-3 py-1 text-xs font-semibold text-violet-700 dark:bg-violet-900/20 dark:text-violet-300"
            >
              {{
                t('userSubscriptions.resetCards.totalAvailable', {
                  count: totalResetCardCount
                })
              }}
            </span>
          </div>

          <div class="divide-y divide-gray-100 dark:divide-dark-700">
            <div
              v-for="item in resetCardInventory"
              :key="item.group_id"
              class="flex flex-col gap-4 px-5 py-4 lg:flex-row lg:items-center lg:justify-between"
              :data-testid="`reset-card-inventory-${item.group_id}`"
            >
              <div class="min-w-0 flex-1">
                <div class="flex flex-wrap items-center gap-2">
                  <span
                    :class="[
                      'h-2 w-2 shrink-0 rounded-full',
                      platformAccentDotClass(item.platform)
                    ]"
                  ></span>
                  <h3 class="truncate text-sm font-semibold text-gray-900 dark:text-white">
                    {{ item.group_name }}
                  </h3>
                  <span
                    :class="[
                      'rounded-md border px-2 py-0.5 text-[11px] font-medium',
                      platformBadgeClass(item.platform)
                    ]"
                  >
                    {{ platformLabel(item.platform) }}
                  </span>
                </div>
                <div class="mt-2 flex flex-wrap gap-x-5 gap-y-1 text-xs text-gray-500 dark:text-dark-400">
                  <span>
                    {{ t('userSubscriptions.resetCards.remaining') }}
                    <strong class="ml-1 text-base text-violet-600 dark:text-violet-300">
                      {{ item.remaining_count }}
                    </strong>
                  </span>
                  <span>
                    {{ t('userSubscriptions.resetCards.nextExpires') }}:
                    <span class="font-medium text-gray-700 dark:text-gray-300">
                      {{ formatResetCardExpiration(item.next_expires_at) }}
                    </span>
                  </span>
                </div>
                <p
                  v-if="!item.can_use && item.unavailable_reason"
                  class="mt-1.5 text-xs text-amber-600 dark:text-amber-400"
                >
                  {{ formatResetCardUnavailableReason(item.unavailable_reason) }}
                </p>
              </div>

              <div class="flex flex-wrap items-center gap-4 lg:justify-end">
                <div class="flex items-center gap-2">
                  <div class="text-right">
                    <p class="text-xs font-medium text-gray-700 dark:text-gray-300">
                      {{ t('userSubscriptions.resetCards.autoUse') }}
                    </p>
                    <p class="text-[11px] text-gray-400 dark:text-gray-500">
                      {{ t('userSubscriptions.resetCards.autoUseHint') }}
                    </p>
                  </div>
                  <Toggle
                    :model-value="item.auto_use_enabled"
                    :disabled="
                      updatingAutoUseGroupId !== null ||
                      (!item.auto_use_available && !item.auto_use_enabled)
                    "
                    :data-testid="`reset-card-auto-${item.group_id}`"
                    :aria-label="`${t('userSubscriptions.resetCards.autoUse')}：${item.group_name}`"
                    :title="
                      item.auto_use_available || item.auto_use_enabled
                        ? t('userSubscriptions.resetCards.autoUseHint')
                        : t('userSubscriptions.resetCards.unavailableReasons.autoUseNotAvailable')
                    "
                    class="disabled:cursor-not-allowed disabled:opacity-50"
                    @update:model-value="(enabled) => handleAutoUseChange(item, enabled)"
                  />
                </div>
                <button
                  type="button"
                  class="btn btn-primary btn-sm"
                  :data-testid="`reset-card-use-${item.group_id}`"
                  :disabled="!item.can_use || usingResetCard"
                  :title="
                    item.can_use
                      ? t('userSubscriptions.resetCards.useNow')
                      : formatResetCardUnavailableReason(item.unavailable_reason)
                  "
                  @click="openUseResetCardConfirm(item)"
                >
                  <Icon
                    name="refresh"
                    size="sm"
                    class="mr-1.5"
                    :class="usingResetCard && useResetCardTarget?.group_id === item.group_id ? 'animate-spin' : ''"
                  />
                  {{ t('userSubscriptions.resetCards.useNow') }}
                </button>
              </div>
            </div>
          </div>
        </section>

      <!-- Empty State -->
      <div v-if="subscriptions.length === 0" class="card p-12 text-center">
        <div
          class="mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-gray-100 dark:bg-dark-700"
        >
          <Icon name="creditCard" size="xl" class="text-gray-400" />
        </div>
        <h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">
          {{ t('userSubscriptions.noActiveSubscriptions') }}
        </h3>
        <p class="text-gray-500 dark:text-dark-400">
          {{ t('userSubscriptions.noActiveSubscriptionsDesc') }}
        </p>
      </div>

      <!-- Subscriptions Grid -->
      <div v-else class="grid gap-6 lg:grid-cols-2">
        <div
          v-for="subscription in subscriptions"
          :key="subscription.id"
          class="overflow-hidden rounded-2xl border bg-white dark:bg-dark-800"
          :class="platformBorderClass(subscription.group?.platform || '')"
        >
          <!-- Header -->
          <div
            class="flex items-center justify-between border-b border-gray-100 p-4 dark:border-dark-700"
          >
            <div class="flex items-center gap-3">
              <div :class="['h-1.5 w-1.5 shrink-0 rounded-full', platformAccentDotClass(subscription.group?.platform || '')]" />
              <div>
                <div class="flex items-center gap-2">
                  <h3 class="font-semibold text-gray-900 dark:text-white">
                    {{ subscription.group?.name || `Group #${subscription.group_id}` }}
                  </h3>
                  <span :class="['rounded-md border px-2 py-0.5 text-[11px] font-medium', platformBadgeClass(subscription.group?.platform || '')]">
                    {{ platformLabel(subscription.group?.platform || '') }}
                  </span>
                </div>
                <p v-if="subscription.group?.description" class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">
                  {{ subscription.group.description }}
                </p>
                <div class="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-[11px] text-gray-400 dark:text-gray-500">
                  <span>{{ t('payment.planCard.rate') }}: ×{{ subscription.group?.rate_multiplier ?? 1 }}</span>
                  <span v-if="subscriptionHasPeakRate(subscription)" class="text-amber-700 dark:text-amber-300">
                    {{ t('payment.planCard.peakRate') }}: {{ subscriptionPeakRateLabel(subscription) }}
                  </span>
                </div>
              </div>
            </div>
            <div class="flex items-center gap-2">
              <span
                :class="[
                  'rounded-full px-2 py-0.5 text-xs font-medium',
                  subscription.status === 'active'
                    ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300'
                    : subscription.status === 'expired'
                      ? 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-400'
                      : 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300'
                ]"
              >
                {{ t(`userSubscriptions.status.${subscription.status}`) }}
              </span>
              <button
                v-if="subscription.status === 'active'"
                :class="['rounded-lg px-3 py-1.5 text-xs font-semibold text-white transition-colors', platformButtonClass(subscription.group?.platform || '')]"
                @click="router.push({ path: '/purchase', query: { tab: 'subscription', group: String(subscription.group_id) } })"
              >
                {{ t('payment.renewNow') }}
              </button>
            </div>
          </div>

          <!-- Usage Progress -->
          <div class="space-y-4 p-4">
            <!-- Expiration Info -->
            <div v-if="subscription.expires_at" class="flex items-center justify-between text-sm">
              <span class="text-gray-500 dark:text-dark-400">{{
                t('userSubscriptions.expires')
              }}</span>
              <span :class="getExpirationClass(subscription.expires_at)">
                {{ formatExpirationDate(subscription.expires_at) }}
              </span>
            </div>
            <div v-else class="flex items-center justify-between text-sm">
              <span class="text-gray-500 dark:text-dark-400">{{
                t('userSubscriptions.expires')
              }}</span>
              <span class="text-gray-700 dark:text-gray-300">{{
                t('userSubscriptions.noExpiration')
              }}</span>
            </div>

            <!-- Daily Usage -->
            <div v-if="subscription.group?.daily_limit_usd" class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('userSubscriptions.daily') }}
                </span>
                <span class="text-sm text-gray-500 dark:text-dark-400">
                  ${{ (subscription.daily_usage_usd || 0).toFixed(2) }} / ${{
                    subscription.group.daily_limit_usd.toFixed(2)
                  }}
                </span>
              </div>
              <div class="relative h-2 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                <div
                  class="absolute inset-y-0 left-0 rounded-full transition-all duration-300"
                  :class="
                    getProgressBarClass(
                      subscription.daily_usage_usd,
                      subscription.group.daily_limit_usd
                    )
                  "
                  :style="{
                    width: getProgressWidth(
                      subscription.daily_usage_usd,
                      subscription.group.daily_limit_usd
                    )
                  }"
                ></div>
              </div>
              <p
                v-if="subscription.daily_window_start"
                class="text-xs text-gray-500 dark:text-dark-400"
              >
                {{ formatDailyUsageWindow(subscription) }}
              </p>
            </div>

            <!-- Weekly Usage -->
            <div v-if="subscription.group?.weekly_limit_usd" class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('userSubscriptions.weekly') }}
                </span>
                <span class="text-sm text-gray-500 dark:text-dark-400">
                  ${{ (subscription.weekly_usage_usd || 0).toFixed(2) }} / ${{
                    subscription.group.weekly_limit_usd.toFixed(2)
                  }}
                </span>
              </div>
              <div class="relative h-2 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                <div
                  class="absolute inset-y-0 left-0 rounded-full transition-all duration-300"
                  :class="
                    getProgressBarClass(
                      subscription.weekly_usage_usd,
                      subscription.group.weekly_limit_usd
                    )
                  "
                  :style="{
                    width: getProgressWidth(
                      subscription.weekly_usage_usd,
                      subscription.group.weekly_limit_usd
                    )
                  }"
                ></div>
              </div>
              <p
                v-if="subscription.weekly_window_start"
                class="text-xs text-gray-500 dark:text-dark-400"
              >
                {{
                  t('userSubscriptions.resetIn', {
                    time: formatResetTime(subscription.weekly_window_start, 168)
                  })
                }}
              </p>
            </div>

            <!-- Monthly Usage -->
            <div v-if="subscription.group?.monthly_limit_usd" class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('userSubscriptions.monthly') }}
                </span>
                <span class="text-sm text-gray-500 dark:text-dark-400">
                  ${{ (subscription.monthly_usage_usd || 0).toFixed(2) }} / ${{
                    subscription.group.monthly_limit_usd.toFixed(2)
                  }}
                </span>
              </div>
              <div class="relative h-2 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                <div
                  class="absolute inset-y-0 left-0 rounded-full transition-all duration-300"
                  :class="
                    getProgressBarClass(
                      subscription.monthly_usage_usd,
                      subscription.group.monthly_limit_usd
                    )
                  "
                  :style="{
                    width: getProgressWidth(
                      subscription.monthly_usage_usd,
                      subscription.group.monthly_limit_usd
                    )
                  }"
                ></div>
              </div>
              <p
                v-if="subscription.monthly_window_start"
                class="text-xs text-gray-500 dark:text-dark-400"
              >
                {{
                  t('userSubscriptions.resetIn', {
                    time: formatResetTime(subscription.monthly_window_start, 720)
                  })
                }}
              </p>
            </div>

            <!-- No limits configured - Unlimited badge -->
            <div
              v-if="
                !subscription.group?.daily_limit_usd &&
                !subscription.group?.weekly_limit_usd &&
                !subscription.group?.monthly_limit_usd
              "
              class="flex items-center justify-center rounded-xl bg-gradient-to-r from-emerald-50 to-teal-50 py-6 dark:from-emerald-900/20 dark:to-teal-900/20"
            >
              <div class="flex items-center gap-3">
                <span class="text-4xl text-emerald-600 dark:text-emerald-400">∞</span>
                <div>
                  <p class="text-sm font-medium text-emerald-700 dark:text-emerald-300">
                    {{ t('userSubscriptions.unlimited') }}
                  </p>
                  <p class="text-xs text-emerald-600/70 dark:text-emerald-400/70">
                    {{ t('userSubscriptions.unlimitedDesc') }}
                  </p>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Recent reset-card usage -->
      <section
        v-if="resetCardUsages.length > 0"
        class="card overflow-hidden"
        data-testid="reset-card-usages"
      >
        <div class="border-b border-gray-100 px-5 py-4 dark:border-dark-700">
          <h2 class="font-semibold text-gray-900 dark:text-white">
            {{ t('userSubscriptions.resetCards.recentUsage') }}
          </h2>
        </div>
        <div class="divide-y divide-gray-100 dark:divide-dark-700">
          <div
            v-for="usage in resetCardUsages"
            :key="usage.id"
            class="flex flex-col gap-2 px-5 py-3 sm:flex-row sm:items-center sm:justify-between"
          >
            <div class="flex min-w-0 items-center gap-3">
              <div
                class="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-violet-50 text-violet-600 dark:bg-violet-900/20 dark:text-violet-300"
              >
                <Icon name="refresh" size="sm" />
              </div>
              <div class="min-w-0">
                <div class="flex flex-wrap items-center gap-2">
                  <p class="truncate text-sm font-medium text-gray-900 dark:text-white">
                    {{ usage.group_name }}
                  </p>
                  <span
                    class="rounded-full px-2 py-0.5 text-[11px] font-medium"
                    :class="
                      usage.mode === 'auto'
                        ? 'bg-blue-50 text-blue-700 dark:bg-blue-900/20 dark:text-blue-300'
                        : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
                    "
                  >
                    {{ t(`userSubscriptions.resetCards.mode.${usage.mode}`) }}
                  </span>
                </div>
                <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">
                  {{ formatResetCardPreviousUsage(usage) }}
                </p>
              </div>
            </div>
            <time class="shrink-0 text-xs text-gray-400 dark:text-gray-500">
              {{ formatDateTimeToMinute(usage.used_at) }}
            </time>
          </div>
        </div>
      </section>
      </template>
    </div>

    <ConfirmDialog
      :show="!!useResetCardTarget"
      :title="t('userSubscriptions.resetCards.confirmTitle')"
      :message="
        t('userSubscriptions.resetCards.confirmMessage', {
          group: useResetCardTarget?.group_name || ''
        })
      "
      :confirm-text="t('userSubscriptions.resetCards.confirmUse')"
      :cancel-text="t('common.cancel')"
      @confirm="confirmUseResetCard"
      @cancel="closeUseResetCardConfirm"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useAppStore } from '@/stores/app'
import subscriptionsAPI from '@/api/subscriptions'
import subscriptionResetCardsAPI from '@/api/subscriptionResetCards'
import type {
  SubscriptionResetCardInventoryItem,
  SubscriptionResetCardUsage,
  UserSubscription
} from '@/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'
import { formatDateTimeToMinute } from '@/utils/format'
import { hasPeakRate, formatPeakRateWindow, serverTimezoneLabel } from '@/utils/peak-rate'
import { platformBorderClass, platformBadgeClass, platformButtonClass, platformLabel } from '@/utils/platformColors'
import { getRemainingDurationParts, isOneTimeDailyQuota, type RemainingDurationParts } from '@/utils/subscriptionQuota'

function platformAccentDotClass(p: string): string {
  switch (p) {
    case 'anthropic': return 'bg-orange-500'
    case 'openai': return 'bg-emerald-500'
    case 'antigravity': return 'bg-purple-500'
    case 'gemini': return 'bg-blue-500'
    default: return 'bg-gray-400'
  }
}

const { t } = useI18n()
const router = useRouter()
const appStore = useAppStore()

const subscriptions = ref<UserSubscription[]>([])
const resetCardInventory = ref<SubscriptionResetCardInventoryItem[]>([])
const resetCardUsages = ref<SubscriptionResetCardUsage[]>([])
const loading = ref(true)
const usingResetCard = ref(false)
const useResetCardTarget = ref<SubscriptionResetCardInventoryItem | null>(null)
const useResetCardIdempotencyKey = ref('')
const updatingAutoUseGroupId = ref<number | null>(null)
let resetCardInventoryGeneration = 0

const totalResetCardCount = computed(() =>
  resetCardInventory.value.reduce((total, item) => total + item.remaining_count, 0)
)

function resetCardErrorMessage(error: any, fallback: string): string {
  return error?.response?.data?.detail || error?.message || fallback
}

function subscriptionHasPeakRate(subscription: UserSubscription): boolean {
  return hasPeakRate(subscription.group)
}

function subscriptionPeakRateLabel(subscription: UserSubscription): string {
  return formatPeakRateWindow(subscription.group, serverTimezoneLabel(appStore.cachedPublicSettings?.server_utc_offset))
}

async function loadSubscriptions() {
  loading.value = true

  // Reset-card data is supplementary. Load it in the background so a slow or
  // unavailable add-on endpoint never delays the core subscription page.
  void loadSubscriptionResetCardData()
  try {
    subscriptions.value = await subscriptionsAPI.getMySubscriptions()
  } catch (error: any) {
    console.error('Failed to load subscriptions:', error)
    appStore.showError(resetCardErrorMessage(error, t('userSubscriptions.failedToLoad')))
  } finally {
    loading.value = false
  }
}

async function loadSubscriptionResetCardData() {
  const inventoryGeneration = resetCardInventoryGeneration
  const [inventoryResult, usageResult] = await Promise.allSettled([
    subscriptionResetCardsAPI.getInventory(),
    subscriptionResetCardsAPI.getUsages(20)
  ])

  let resetCardDataFailed = false
  if (inventoryResult.status === 'fulfilled') {
    if (inventoryGeneration === resetCardInventoryGeneration) {
      resetCardInventory.value = inventoryResult.value
    }
  } else if (inventoryGeneration === resetCardInventoryGeneration) {
    resetCardInventory.value = []
    resetCardDataFailed = true
    console.error('Failed to load reset-card inventory:', inventoryResult.reason)
  }
  if (usageResult.status === 'fulfilled') {
    resetCardUsages.value = usageResult.value
  } else {
    resetCardUsages.value = []
    resetCardDataFailed = true
    console.error('Failed to load reset-card usage:', usageResult.reason)
  }
  if (resetCardDataFailed) {
    appStore.showError(t('userSubscriptions.resetCards.failedToLoad'))
  }
}

async function refreshSubscriptionResetCardData() {
  const inventoryGeneration = resetCardInventoryGeneration
  const [subscriptionResult, inventoryResult, usageResult] = await Promise.allSettled([
    subscriptionsAPI.getMySubscriptions(),
    subscriptionResetCardsAPI.getInventory(),
    subscriptionResetCardsAPI.getUsages(20)
  ])
  if (subscriptionResult.status === 'fulfilled') {
    subscriptions.value = subscriptionResult.value
  }
  if (
    inventoryResult.status === 'fulfilled' &&
    inventoryGeneration === resetCardInventoryGeneration
  ) {
    resetCardInventory.value = inventoryResult.value
  }
  if (usageResult.status === 'fulfilled') {
    resetCardUsages.value = usageResult.value
  }
  if (
    subscriptionResult.status === 'rejected' ||
    inventoryResult.status === 'rejected' ||
    usageResult.status === 'rejected'
  ) {
    appStore.showError(t('userSubscriptions.resetCards.refreshFailed'))
  }
}

function createResetCardIdempotencyKey(scope: string): string {
  const requestID =
    globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`
  return `subscription-reset-card-${scope}-${requestID}`
}

function openUseResetCardConfirm(item: SubscriptionResetCardInventoryItem) {
  if (!item.can_use || !item.eligible_subscription_id || usingResetCard.value) return
  useResetCardTarget.value = item
  useResetCardIdempotencyKey.value = createResetCardIdempotencyKey(
    `use-${item.eligible_subscription_id}`
  )
}

function closeUseResetCardConfirm() {
  if (usingResetCard.value) return
  useResetCardTarget.value = null
  useResetCardIdempotencyKey.value = ''
}

async function confirmUseResetCard() {
  const item = useResetCardTarget.value
  if (!item?.eligible_subscription_id || usingResetCard.value) return

  usingResetCard.value = true
  try {
    await subscriptionResetCardsAPI.useCard(
      item.eligible_subscription_id,
      useResetCardIdempotencyKey.value
    )
    resetCardInventoryGeneration++
    await refreshSubscriptionResetCardData()
    appStore.showSuccess(t('userSubscriptions.resetCards.useSuccess'))
    useResetCardTarget.value = null
    useResetCardIdempotencyKey.value = ''
  } catch (error: any) {
    appStore.showError(resetCardErrorMessage(error, t('userSubscriptions.resetCards.useFailed')))
  } finally {
    usingResetCard.value = false
  }
}

async function handleAutoUseChange(
  item: SubscriptionResetCardInventoryItem,
  enabled: boolean
) {
  if (
    (enabled && !item.auto_use_available) ||
    updatingAutoUseGroupId.value !== null
  ) {
    return
  }

  updatingAutoUseGroupId.value = item.group_id
  try {
    const updated = await subscriptionResetCardsAPI.updateAutoUsePreference(
      item.group_id,
      enabled
    )
    resetCardInventoryGeneration++
    const index = resetCardInventory.value.findIndex(
      (inventoryItem) => inventoryItem.group_id === item.group_id
    )
    if (index >= 0) {
      resetCardInventory.value[index] = updated
    }
    appStore.showSuccess(
      t(
        enabled
          ? 'userSubscriptions.resetCards.autoUseEnabled'
          : 'userSubscriptions.resetCards.autoUseDisabled'
      )
    )
  } catch (error: any) {
    appStore.showError(
      resetCardErrorMessage(error, t('userSubscriptions.resetCards.autoUseFailed'))
    )
  } finally {
    updatingAutoUseGroupId.value = null
  }
}

function formatResetCardExpiration(expiresAt: string | null): string {
  return expiresAt
    ? formatDateTimeToMinute(expiresAt)
    : t('userSubscriptions.resetCards.noExpiration')
}

function formatResetCardUnavailableReason(reason: string | null | undefined): string {
  if (!reason) return t('userSubscriptions.resetCards.unavailableReasons.unavailable')
  const reasonKey = reason.trim().toLowerCase().replace(/[ -]+/g, '_')
  const knownReasons: Record<string, string> = {
    no_available_cards: 'noAvailableCards',
    no_cards: 'noAvailableCards',
    no_active_subscription: 'noActiveSubscription',
    subscription_not_active: 'subscriptionNotActive',
    subscription_has_no_limits: 'subscriptionHasNoLimits',
    no_usage_limits: 'subscriptionHasNoLimits',
    no_configured_quota: 'subscriptionHasNoLimits',
    nothing_to_reset: 'nothingToReset',
    one_time_daily_quota: 'oneTimeDailyQuota',
    auto_use_not_available: 'autoUseNotAvailable'
  }
  const localeKey = knownReasons[reasonKey]
  return localeKey
    ? t(`userSubscriptions.resetCards.unavailableReasons.${localeKey}`)
    : reason
}

function formatResetCardPreviousUsage(usage: SubscriptionResetCardUsage): string {
  return t('userSubscriptions.resetCards.previousUsage', {
    daily: usage.previous_daily_usage_usd.toFixed(2),
    weekly: usage.previous_weekly_usage_usd.toFixed(2),
    monthly: usage.previous_monthly_usage_usd.toFixed(2)
  })
}

function getProgressWidth(used: number | undefined, limit: number | null | undefined): string {
  if (!limit || limit === 0) return '0%'
  const percentage = Math.min(((used || 0) / limit) * 100, 100)
  return `${percentage}%`
}

function getProgressBarClass(used: number | undefined, limit: number | null | undefined): string {
  if (!limit || limit === 0) return 'bg-gray-400'
  const percentage = ((used || 0) / limit) * 100
  if (percentage >= 90) return 'bg-red-500'
  if (percentage >= 70) return 'bg-orange-500'
  return 'bg-green-500'
}

function formatExpirationDate(expiresAt: string): string {
  const now = new Date()
  const expires = new Date(expiresAt)
  const diff = expires.getTime() - now.getTime()
  const days = Math.ceil(diff / (1000 * 60 * 60 * 24))

  if (days < 0) {
    return t('userSubscriptions.status.expired')
  }

  const dateStr = formatDateTimeToMinute(expires)

  if (days === 0) {
    return `${dateStr} (${t('common.today')})`
  }
  if (days === 1) {
    return `${dateStr} (${t('common.tomorrow')})`
  }

  return t('userSubscriptions.daysRemaining', { days }) + ` (${dateStr})`
}

function getExpirationClass(expiresAt: string): string {
  const now = new Date()
  const expires = new Date(expiresAt)
  const diff = expires.getTime() - now.getTime()
  const days = Math.ceil(diff / (1000 * 60 * 60 * 24))

  if (days <= 0) return 'text-red-600 dark:text-red-400 font-medium'
  if (days <= 3) return 'text-red-600 dark:text-red-400'
  if (days <= 7) return 'text-orange-600 dark:text-orange-400'
  return 'text-gray-700 dark:text-gray-300'
}

function formatDurationParts(parts: RemainingDurationParts): string {
  if (parts.days > 0) {
    return `${parts.days}d ${parts.hours}h`
  }

  if (parts.hours > 0) {
    return `${parts.hours}h ${parts.minutes}m`
  }

  return `${parts.minutes}m`
}

function formatDailyUsageWindow(subscription: UserSubscription): string {
  if (isOneTimeDailyQuota(subscription) && subscription.expires_at) {
    const parts = getRemainingDurationParts(subscription.expires_at)
    if (!parts) return t('userSubscriptions.windowNotActive')
    return t('userSubscriptions.quotaEndsIn', { time: formatDurationParts(parts) })
  }

  return t('userSubscriptions.resetIn', {
    time: formatResetTime(subscription.daily_window_start, 24)
  })
}

function formatResetTime(windowStart: string | null, windowHours: number): string {
  if (!windowStart) return t('userSubscriptions.windowNotActive')

  const start = new Date(windowStart)
  const end = new Date(start.getTime() + windowHours * 60 * 60 * 1000)
  const parts = getRemainingDurationParts(end)

  return parts ? formatDurationParts(parts) : t('userSubscriptions.windowNotActive')
}

onMounted(() => {
  loadSubscriptions()
})
</script>
