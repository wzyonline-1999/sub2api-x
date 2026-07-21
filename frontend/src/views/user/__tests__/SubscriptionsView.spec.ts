import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'

import type { SubscriptionResetCardInventoryItem, UserSubscription } from '@/types'
import SubscriptionsView from '../SubscriptionsView.vue'

const {
  getMySubscriptions,
  getInventory,
  getUsages,
  useCard,
  updateAutoUsePreference,
  showError,
  showSuccess
} = vi.hoisted(() => ({
  getMySubscriptions: vi.fn(),
  getInventory: vi.fn(),
  getUsages: vi.fn(),
  useCard: vi.fn(),
  updateAutoUsePreference: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn()
}))

vi.mock('@/api/subscriptions', () => ({
  default: { getMySubscriptions }
}))

vi.mock('@/api/subscriptionResetCards', () => ({
  default: {
    getInventory,
    getUsages,
    useCard,
    updateAutoUsePreference
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    cachedPublicSettings: null,
    showError,
    showSuccess
  })
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${key}:${JSON.stringify(params)}` : key
    })
  }
})

const AppLayoutStub = { template: '<main><slot /></main>' }
const ToggleStub = {
  inheritAttrs: false,
  props: ['modelValue', 'disabled'],
  emits: ['update:modelValue'],
  template: `
    <button
      v-bind="$attrs"
      type="button"
      :disabled="disabled"
      :aria-pressed="modelValue"
      @click="$emit('update:modelValue', !modelValue)"
    >toggle</button>
  `
}
const ConfirmDialogStub = {
  props: ['show', 'title', 'message'],
  emits: ['confirm', 'cancel'],
  template: `
    <div v-if="show" data-testid="use-reset-card-confirm">
      <span>{{ message }}</span>
      <button data-testid="confirm-use-reset-card" @click="$emit('confirm')">confirm</button>
      <button @click="$emit('cancel')">cancel</button>
    </div>
  `
}

const wrappers: VueWrapper[] = []

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

function inventory(
  overrides: Partial<SubscriptionResetCardInventoryItem> = {}
): SubscriptionResetCardInventoryItem {
  return {
    group_id: 8,
    group_name: 'Codex Pro',
    platform: 'openai',
    remaining_count: 2,
    next_expires_at: '2026-08-01T00:00:00Z',
    auto_use_enabled: false,
    auto_use_available: true,
    eligible_subscription_id: 31,
    can_use: true,
    unavailable_reason: null,
    ...overrides
  }
}

function subscription(): UserSubscription {
  return {
    id: 31,
    user_id: 2,
    group_id: 8,
    status: 'active',
    starts_at: '2026-07-01T00:00:00Z',
    daily_usage_usd: 4,
    weekly_usage_usd: 9,
    monthly_usage_usd: 18,
    daily_window_start: '2026-07-21T00:00:00Z',
    weekly_window_start: '2026-07-20T00:00:00Z',
    monthly_window_start: '2026-07-01T00:00:00Z',
    created_at: '2026-07-01T00:00:00Z',
    updated_at: '2026-07-21T00:00:00Z',
    expires_at: '2026-08-01T00:00:00Z',
    group: {
      id: 8,
      name: 'Codex Pro',
      description: null,
      platform: 'openai',
      rate_multiplier: 1,
      status: 'active',
      subscription_type: 'subscription',
      daily_limit_usd: 10,
      weekly_limit_usd: 50,
      monthly_limit_usd: 200
    } as UserSubscription['group']
  }
}

function mountView(): VueWrapper {
  const wrapper = mount(SubscriptionsView, {
    global: {
      stubs: {
        AppLayout: AppLayoutStub,
        Icon: true,
        Toggle: ToggleStub,
        ConfirmDialog: ConfirmDialogStub
      }
    }
  })
  wrappers.push(wrapper)
  return wrapper
}

describe('user SubscriptionsView reset cards', () => {
  beforeEach(() => {
    getMySubscriptions.mockReset().mockResolvedValue([])
    getInventory.mockReset().mockResolvedValue([inventory()])
    getUsages.mockReset().mockResolvedValue([])
    useCard.mockReset().mockResolvedValue({ subscription: { id: 31 }, usage: { id: 41 } })
    updateAutoUsePreference.mockReset().mockImplementation(async (_groupId, enabled) =>
      inventory({ auto_use_enabled: enabled })
    )
    showError.mockReset()
    showSuccess.mockReset()
    vi.spyOn(globalThis.crypto, 'randomUUID').mockReturnValue(
      '11111111-1111-4111-8111-111111111111'
    )
  })

  afterEach(() => {
    for (const wrapper of wrappers.splice(0)) wrapper.unmount()
    vi.restoreAllMocks()
  })

  it('keeps the reset-card wallet visible when the user has no active subscription', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="reset-card-wallet"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="reset-card-inventory-8"]').text()).toContain('Codex Pro')
    expect(wrapper.text()).toContain('userSubscriptions.noActiveSubscriptions')
  })

  it('confirms manual use, sends one idempotency key, and refreshes all card data', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="reset-card-use-8"]').trigger('click')
    expect(wrapper.find('[data-testid="use-reset-card-confirm"]').exists()).toBe(true)

    await wrapper.get('[data-testid="confirm-use-reset-card"]').trigger('click')
    await flushPromises()

    expect(useCard).toHaveBeenCalledTimes(1)
    expect(useCard).toHaveBeenCalledWith(
      31,
      'subscription-reset-card-use-31-11111111-1111-4111-8111-111111111111'
    )
    expect(getMySubscriptions).toHaveBeenCalledTimes(2)
    expect(getInventory).toHaveBeenCalledTimes(2)
    expect(getUsages).toHaveBeenCalledTimes(2)
    expect(showSuccess).toHaveBeenCalledWith('userSubscriptions.resetCards.useSuccess')
  })

  it('persists automatic use only through the preference endpoint', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="reset-card-auto-8"]').trigger('click')
    await flushPromises()

    expect(updateAutoUsePreference).toHaveBeenCalledWith(8, true)
    expect(showSuccess).toHaveBeenCalledWith('userSubscriptions.resetCards.autoUseEnabled')
  })

  it('does not let an older manual-use refresh overwrite a newer auto-use preference', async () => {
    const staleInventory = deferred<SubscriptionResetCardInventoryItem[]>()
    getInventory
      .mockReset()
      .mockResolvedValueOnce([inventory()])
      .mockReturnValueOnce(staleInventory.promise)
    updateAutoUsePreference.mockResolvedValueOnce(
      inventory({ remaining_count: 1, auto_use_enabled: true })
    )
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="reset-card-use-8"]').trigger('click')
    await wrapper.get('[data-testid="confirm-use-reset-card"]').trigger('click')
    await flushPromises()

    const toggle = wrapper.get('[data-testid="reset-card-auto-8"]')
    await toggle.trigger('click')
    await flushPromises()
    expect(toggle.attributes('aria-pressed')).toBe('true')

    staleInventory.resolve([
      inventory({ remaining_count: 1, auto_use_enabled: false })
    ])
    await flushPromises()

    expect(toggle.attributes('aria-pressed')).toBe('true')
    expect(updateAutoUsePreference).toHaveBeenCalledWith(8, true)
    expect(showSuccess).toHaveBeenCalledWith('userSubscriptions.resetCards.useSuccess')
  })

  it('disables actions when the backend marks the inventory item unavailable', async () => {
    getInventory.mockResolvedValue([
      inventory({
        can_use: false,
        auto_use_available: false,
        unavailable_reason: 'no_active_subscription',
        eligible_subscription_id: null
      })
    ])
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-testid="reset-card-use-8"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="reset-card-auto-8"]').attributes('disabled')).toBeDefined()
  })

  it('allows an unavailable subscription to turn off an already-enabled auto-use preference', async () => {
    getInventory.mockResolvedValue([
      inventory({
        auto_use_enabled: true,
        auto_use_available: false,
        can_use: false,
        unavailable_reason: 'no_active_subscription',
        eligible_subscription_id: null
      })
    ])
    updateAutoUsePreference.mockResolvedValue(
      inventory({ auto_use_enabled: false, auto_use_available: false })
    )
    const wrapper = mountView()
    await flushPromises()

    const toggle = wrapper.get('[data-testid="reset-card-auto-8"]')
    expect(toggle.attributes('disabled')).toBeUndefined()
    await toggle.trigger('click')
    await flushPromises()

    expect(updateAutoUsePreference).toHaveBeenCalledWith(8, false)
  })

  it('keeps subscription content available when reset-card support APIs fail', async () => {
    getMySubscriptions.mockResolvedValue([subscription()])
    getInventory.mockRejectedValue(new Error('inventory unavailable'))
    getUsages.mockRejectedValue(new Error('usage unavailable'))
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('Codex Pro')
    expect(wrapper.find('[data-testid="reset-card-wallet"]').exists()).toBe(false)
    expect(showError).toHaveBeenCalledWith('userSubscriptions.resetCards.failedToLoad')
  })

  it('renders subscription content without waiting for reset-card support APIs', async () => {
    getMySubscriptions.mockResolvedValue([subscription()])
    getInventory.mockReturnValue(new Promise(() => {}))
    getUsages.mockReturnValue(new Promise(() => {}))

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('Codex Pro')
    expect(wrapper.find('[data-testid="reset-card-wallet"]').exists()).toBe(false)
  })
})
