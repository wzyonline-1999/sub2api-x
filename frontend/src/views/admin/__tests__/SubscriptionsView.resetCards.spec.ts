import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'

import SubscriptionsView from '../SubscriptionsView.vue'

const {
  listSubscriptions,
  getAllGroups,
  searchUsers,
  grantResetCards,
  listResetCardGrants,
  listResetCardUsages,
  showError,
  showSuccess
} =
  vi.hoisted(() => ({
    listSubscriptions: vi.fn(),
    getAllGroups: vi.fn(),
    searchUsers: vi.fn(),
    grantResetCards: vi.fn(),
    listResetCardGrants: vi.fn(),
    listResetCardUsages: vi.fn(),
    showError: vi.fn(),
    showSuccess: vi.fn()
  }))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    subscriptions: {
      list: listSubscriptions,
      assign: vi.fn(),
      extend: vi.fn(),
      revoke: vi.fn(),
      restore: vi.fn(),
      resetQuota: vi.fn()
    },
    subscriptionResetCards: {
      grant: grantResetCards,
      listGrants: listResetCardGrants,
      listUsages: listResetCardUsages
    },
    groups: {
      getAll: getAllGroups
    },
    usage: {
      searchUsers
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess
  })
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

const TablePageLayoutStub = {
  template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
}
const DataTableStub = {
  props: ['data'],
  emits: ['sort'],
  template: `
    <div>
      <div v-for="row in data" :key="row.id">
        <slot name="cell-actions" :row="row" />
      </div>
    </div>
  `
}
const BaseDialogStub = {
  props: ['show', 'title'],
  emits: ['close'],
  template: `
    <section v-if="show" data-testid="base-dialog">
      <h2>{{ title }}</h2>
      <slot />
      <slot name="footer" />
    </section>
  `
}
const SelectStub = {
  props: ['modelValue', 'options'],
  emits: ['update:modelValue', 'change'],
  template: `
    <select
      :value="modelValue == null ? '' : String(modelValue)"
      @change="$emit('update:modelValue', Number($event.target.value) || $event.target.value)"
    >
      <option value=""></option>
      <option v-for="option in options" :key="option.value" :value="option.value">
        {{ option.label }}
      </option>
    </select>
  `
}

const wrappers: VueWrapper[] = []

const makeResetCardGrant = (id: number) => ({
  id,
  user_id: 2,
  user_email: 'member@example.com',
  group_id: 8,
  group_name: 'Codex Pro',
  issued_count: 3,
  remaining_count: 2,
  expires_at: '2026-08-01T00:00:00Z',
  status: 'active',
  source: 'admin_grant',
  issued_by: 1,
  notes: 'retention',
  created_at: '2026-07-21T00:00:00Z',
  updated_at: '2026-07-21T00:00:00Z'
})

const makeResetCardUsage = (id: number) => ({
  id,
  grant_id: 51,
  subscription_id: 12,
  user_id: 2,
  user_email: 'member@example.com',
  group_id: 8,
  group_name: 'Codex Pro',
  mode: 'manual',
  previous_daily_usage_usd: 4,
  previous_weekly_usage_usd: 9,
  previous_monthly_usage_usd: 18,
  used_at: '2026-07-21T01:00:00Z'
})

function mountView(): VueWrapper {
  const wrapper = mount(SubscriptionsView, {
    global: {
      stubs: {
        AppLayout: { template: '<main><slot /></main>' },
        TablePageLayout: TablePageLayoutStub,
        DataTable: DataTableStub,
        BaseDialog: BaseDialogStub,
        ConfirmDialog: true,
        Pagination: true,
        EmptyState: true,
        Select: SelectStub,
        GroupBadge: true,
        GroupOptionItem: true,
        Icon: true,
        RouterLink: true,
        Teleport: true
      }
    }
  })
  wrappers.push(wrapper)
  return wrapper
}

describe('admin SubscriptionsView reset-card grant', () => {
  beforeEach(() => {
    localStorage.clear()
    listSubscriptions.mockReset().mockResolvedValue({
      items: [
        {
          id: 12,
          user_id: 2,
          group_id: 8,
          status: 'active',
          user: { id: 2, email: 'member@example.com' },
          group: { id: 8, name: 'Codex Pro', subscription_type: 'subscription' }
        }
      ],
      total: 1,
      pages: 1
    })
    getAllGroups.mockReset().mockResolvedValue([
      {
        id: 8,
        name: 'Codex Pro',
        description: null,
        platform: 'openai',
        subscription_type: 'subscription',
        rate_multiplier: 1,
        status: 'active'
      }
    ])
    searchUsers.mockReset().mockResolvedValue([])
    grantResetCards.mockReset().mockResolvedValue({ id: 51, issued_count: 3 })
    listResetCardGrants.mockReset().mockResolvedValue([makeResetCardGrant(51)])
    listResetCardUsages.mockReset().mockResolvedValue([makeResetCardUsage(61)])
    showError.mockReset()
    showSuccess.mockReset()
    vi.spyOn(globalThis.crypto, 'randomUUID').mockReturnValue(
      '22222222-2222-4222-8222-222222222222'
    )
  })

  afterEach(() => {
    vi.useRealTimers()
    for (const wrapper of wrappers.splice(0)) wrapper.unmount()
    vi.restoreAllMocks()
  })

  it('prefills user and group from a subscription row and grants a validated batch', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="grant-reset-cards-12"]').trigger('click')

    const form = wrapper.get('#grant-reset-cards-form')
    expect((form.get('[data-reset-card-user-search] input').element as HTMLInputElement).value).toBe(
      'member@example.com'
    )
    expect((form.get('select').element as HTMLSelectElement).value).toBe('8')

    await form.get('#reset-card-count').setValue(3)
    await form.get('#reset-card-notes').setValue('retention campaign')
    await form.trigger('submit')
    await flushPromises()

    expect(grantResetCards).toHaveBeenCalledWith(
      {
        user_id: 2,
        group_id: 8,
        count: 3,
        expires_at: undefined,
        notes: 'retention campaign'
      },
      'subscription-reset-card-grant-22222222-2222-4222-8222-222222222222'
    )
    expect(showSuccess).toHaveBeenCalledWith(
      expect.stringContaining('admin.subscriptions.resetCards.grantSuccess')
    )
    expect(listResetCardGrants).toHaveBeenCalledWith({ limit: 100, offset: 0 })
    expect(wrapper.find('#grant-reset-cards-form').exists()).toBe(false)
  })

  it('opens an empty grant form from the page toolbar', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="grant-reset-cards"]').trigger('click')

    const form = wrapper.get('#grant-reset-cards-form')
    const userSearch = form.get('[data-reset-card-user-search] input')
    expect((userSearch.element as HTMLInputElement).value).toBe('')
    expect(userSearch.attributes('role')).toBe('combobox')
    expect(userSearch.attributes('aria-controls')).toBe('reset-card-user-options')
    expect(userSearch.attributes('aria-expanded')).toBe('false')

    const groupSelect = form.get('select')
    expect((groupSelect.element as HTMLSelectElement).value).toBe('')
    expect(groupSelect.attributes('aria-label')).toBe('admin.subscriptions.form.group')
  })

  it('exposes the reset-card user search as an accessible combobox', async () => {
    searchUsers.mockResolvedValueOnce([{ id: 3, email: 'search-result@example.com' }])
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-testid="grant-reset-cards"]').trigger('click')

    vi.useFakeTimers()
    const userSearch = wrapper.get('#reset-card-user-search')
    await userSearch.trigger('focus')
    await userSearch.setValue('search-result')
    expect(userSearch.attributes('aria-expanded')).toBe('true')

    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()

    const listbox = wrapper.get('#reset-card-user-options')
    expect(listbox.attributes('role')).toBe('listbox')
    const option = listbox.get('[role="option"]')
    expect(option.text()).toContain('search-result@example.com')
    expect(option.attributes('aria-selected')).toBe('false')
  })

  it('does not grant to the previously selected user when the search text changes', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="grant-reset-cards-12"]').trigger('click')
    const form = wrapper.get('#grant-reset-cards-form')
    await form.get('[data-reset-card-user-search] input').setValue('other@example.com')
    await form.trigger('submit')

    expect(grantResetCards).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith('admin.subscriptions.pleaseSelectUser')
  })

  it('rejects a hidden row group that is not present in the active group options', async () => {
    getAllGroups.mockReset().mockResolvedValue([])
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="grant-reset-cards-12"]').trigger('click')
    const form = wrapper.get('#grant-reset-cards-form')
    await form.trigger('submit')

    expect(grantResetCards).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith('admin.subscriptions.pleaseSelectGroup')
  })

  it('loads and renders grant records and usage audit rows in separate tabs', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="reset-card-records"]').trigger('click')
    await flushPromises()

    expect(listResetCardGrants).toHaveBeenCalledWith({ limit: 100, offset: 0 })
    expect(listResetCardUsages).toHaveBeenCalledWith({ limit: 100, offset: 0 })
    const grantRow = wrapper.get('[data-testid="reset-card-grant-51"]')
    expect(grantRow.text()).toContain('member@example.com')
    expect(grantRow.text()).toContain('Codex Pro')
    expect(grantRow.text()).toContain('2 / 3')

    await wrapper.get('[data-testid="reset-card-records-tab-usages"]').trigger('click')
    const usageRow = wrapper.get('[data-testid="reset-card-usage-61"]')
    expect(usageRow.text()).toContain('member@example.com')
    expect(usageRow.text()).toContain('#12')
    expect(usageRow.text()).toContain('admin.subscriptions.resetCards.previousUsage')
  })

  it('shows a retryable records error and then an empty state', async () => {
    listResetCardGrants.mockReset().mockRejectedValueOnce(new Error('audit unavailable'))
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="reset-card-records"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="reset-card-grants-error"]').text()).toContain(
      'audit unavailable'
    )

    listResetCardGrants.mockResolvedValueOnce([])
    await wrapper.get('[data-testid="reset-card-grants-error"] button').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="reset-card-grants-empty"]').exists()).toBe(true)
  })

  it('loads subsequent grant and usage pages with offsets and appends the records', async () => {
    const firstGrantPage = Array.from({ length: 100 }, (_, index) =>
      makeResetCardGrant(1000 - index)
    )
    const firstUsagePage = Array.from({ length: 100 }, (_, index) =>
      makeResetCardUsage(2000 - index)
    )
    listResetCardGrants
      .mockReset()
      .mockResolvedValueOnce(firstGrantPage)
      .mockResolvedValueOnce([makeResetCardGrant(900)])
    listResetCardUsages
      .mockReset()
      .mockResolvedValueOnce(firstUsagePage)
      .mockResolvedValueOnce([makeResetCardUsage(1900)])

    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-testid="reset-card-records"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="reset-card-records-tab-grants"]').text()).toContain('100')
    await wrapper.get('[data-testid="load-more-reset-card-grants"]').trigger('click')
    await flushPromises()
    expect(listResetCardGrants).toHaveBeenLastCalledWith({ limit: 100, offset: 100 })
    expect(wrapper.find('[data-testid="reset-card-grant-1000"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="reset-card-grant-900"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="reset-card-records-tab-grants"]').text()).toContain('101')
    expect(wrapper.find('[data-testid="load-more-reset-card-grants"]').exists()).toBe(false)

    await wrapper.get('[data-testid="reset-card-records-tab-usages"]').trigger('click')
    expect(wrapper.get('[data-testid="reset-card-records-tab-usages"]').text()).toContain('100')
    await wrapper.get('[data-testid="load-more-reset-card-usages"]').trigger('click')
    await flushPromises()
    expect(listResetCardUsages).toHaveBeenLastCalledWith({ limit: 100, offset: 100 })
    expect(wrapper.find('[data-testid="reset-card-usage-2000"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="reset-card-usage-1900"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="reset-card-records-tab-usages"]').text()).toContain('101')
    expect(wrapper.find('[data-testid="load-more-reset-card-usages"]').exists()).toBe(false)
  })

  it('keeps loaded grant records visible when loading more fails and retries the same offset', async () => {
    const firstGrantPage = Array.from({ length: 100 }, (_, index) =>
      makeResetCardGrant(1000 - index)
    )
    listResetCardGrants
      .mockReset()
      .mockResolvedValueOnce(firstGrantPage)
      .mockRejectedValueOnce(new Error('next page unavailable'))
      .mockResolvedValueOnce([makeResetCardGrant(900)])

    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-testid="reset-card-records"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="load-more-reset-card-grants"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="reset-card-grant-1000"]').exists()).toBe(true)
    const retryState = wrapper.get('[data-testid="reset-card-grants-load-more-error"]')
    expect(retryState.text()).toContain('next page unavailable')

    await retryState.get('button').trigger('click')
    await flushPromises()
    expect(listResetCardGrants).toHaveBeenNthCalledWith(2, { limit: 100, offset: 100 })
    expect(listResetCardGrants).toHaveBeenNthCalledWith(3, { limit: 100, offset: 100 })
    expect(wrapper.find('[data-testid="reset-card-grant-1000"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="reset-card-grant-900"]').exists()).toBe(true)
  })

  it('reuses the grant idempotency key for the same failed payload and rotates it after edits', async () => {
    vi.mocked(globalThis.crypto.randomUUID)
      .mockReset()
      .mockReturnValueOnce('33333333-3333-4333-8333-333333333333')
      .mockReturnValueOnce('44444444-4444-4444-8444-444444444444')
    grantResetCards
      .mockReset()
      .mockRejectedValueOnce(new Error('timeout'))
      .mockRejectedValueOnce(new Error('timeout again'))
      .mockResolvedValueOnce({ id: 52, issued_count: 3 })

    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-testid="grant-reset-cards-12"]').trigger('click')
    const form = wrapper.get('#grant-reset-cards-form')
    await form.get('#reset-card-count').setValue(2)

    await form.trigger('submit')
    await flushPromises()
    await form.trigger('submit')
    await flushPromises()

    expect(grantResetCards.mock.calls[0][1]).toBe(
      'subscription-reset-card-grant-33333333-3333-4333-8333-333333333333'
    )
    expect(grantResetCards.mock.calls[1][1]).toBe(grantResetCards.mock.calls[0][1])

    await form.get('#reset-card-count').setValue(3)
    await form.trigger('submit')
    await flushPromises()

    expect(grantResetCards.mock.calls[2][1]).toBe(
      'subscription-reset-card-grant-44444444-4444-4444-8444-444444444444'
    )
  })
})
