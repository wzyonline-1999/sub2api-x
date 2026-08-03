import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'

import type { ApiKey } from '@/types'
import KeysView from '../KeysView.vue'

const {
  listKeys,
  getPublicSettings,
  getDashboardApiKeysUsage,
  getAvailableGroups,
  getUserGroupRates,
  showError,
  showSuccess,
  copyToClipboard,
  isCurrentStep,
  nextStep,
} = vi.hoisted(() => ({
  listKeys: vi.fn(),
  getPublicSettings: vi.fn(),
  getDashboardApiKeysUsage: vi.fn(),
  getAvailableGroups: vi.fn(),
  getUserGroupRates: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  copyToClipboard: vi.fn(),
  isCurrentStep: vi.fn(),
  nextStep: vi.fn(),
}))

const messages: Record<string, string> = {
  'common.actions': 'Actions',
  'common.name': 'Name',
  'common.refresh': 'Refresh',
  'common.retry': 'Retry',
  'common.status': 'Status',
  'keys.apiKey': 'API Key',
  'keys.allGroups': 'All Groups',
  'keys.allStatus': 'All Status',
  'keys.columnSettings': 'Column Settings',
  'keys.createKey': 'Create API Key',
  'keys.created': 'Created',
  'keys.expiresAt': 'Expires',
  'keys.group': 'Group',
  'keys.id': 'ID',
  'keys.currentConcurrency': 'Current Concurrency',
  'keys.lastUsedAt': 'Last Used',
  'keys.lastUsedIP': 'Last Used IP',
  'keys.rateLimitColumn': 'Rate Limit',
  'keys.searchPlaceholder': 'Search name or key...',
  'keys.status.active': 'Active',
  'keys.status.expired': 'Expired',
  'keys.status.inactive': 'Inactive',
  'keys.status.quota_exhausted': 'Quota exhausted',
  'keys.usage': 'Usage',
  'usage.failedToLoad': 'Failed to load usage',
}

vi.mock('@/api', () => ({
  keysAPI: {
    list: listKeys,
    create: vi.fn(),
    update: vi.fn(),
    delete: vi.fn(),
    toggleStatus: vi.fn(),
  },
  authAPI: {
    getPublicSettings,
  },
  usageAPI: {
    getDashboardApiKeysUsage,
  },
  userGroupsAPI: {
    getAvailable: getAvailableGroups,
    getUserGroupRates,
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
  }),
}))

vi.mock('@/stores/onboarding', () => ({
  useOnboardingStore: () => ({
    isCurrentStep,
    nextStep,
  }),
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard,
  }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key,
    }),
  }
})

const createApiKey = (): ApiKey => ({
  id: 1,
  user_id: 1,
  key: 'sk-test-key',
  name: 'test-key',
  group_id: null,
  status: 'active',
  ip_whitelist: [],
  ip_blacklist: [],
  last_used_at: null,
  last_used_ip: null,
  quota: 0,
  quota_used: 0,
  expires_at: null,
  created_at: '2026-06-27T00:00:00Z',
  updated_at: '2026-06-27T00:00:00Z',
  current_concurrency: 3,
  rate_limit_5h: 0,
  rate_limit_1d: 0,
  rate_limit_7d: 0,
  usage_5h: 0,
  usage_1d: 0,
  usage_7d: 0,
  window_5h_start: null,
  window_1d_start: null,
  window_7d_start: null,
  reset_5h_at: null,
  reset_1d_at: null,
  reset_7d_at: null,
})

const AppLayoutStub = {
  template: '<div><slot /></div>',
}

const TablePageLayoutStub = {
  template: `
    <div>
      <slot name="filters" />
      <slot name="actions" />
      <slot name="table" />
      <slot name="pagination" />
    </div>
  `,
}

const DataTableStub = {
  name: 'DataTable',
  props: ['columns', 'data', 'loading'],
  emits: ['sort'],
  template: `
    <div>
      <div data-test="table-loading">{{ String(loading) }}</div>
      <div data-test="columns">{{ columns.map((col) => col.key).join(',') }}</div>
      <div data-test="columns-meta">{{ JSON.stringify(columns.map((col) => ({ key: col.key, sortable: !!col.sortable }))) }}</div>
      <button data-test="sort-current-concurrency" @click="$emit('sort', 'current_concurrency', 'asc')">
        Sort Current Concurrency
      </button>
      <div v-for="row in data" :key="row.id">
        <div
          v-if="columns.some((col) => col.key === 'id')"
          data-test="key-id"
        >
          <slot name="cell-id" :value="row.id" :row="row" />
        </div>
        <slot name="cell-name" :value="row.name" :row="row" />
        <div data-test="current-concurrency">
          <slot name="cell-current_concurrency" :value="row.current_concurrency" :row="row" />
        </div>
        <div data-test="usage">
          <slot name="cell-usage" :row="row" />
        </div>
        <div
          v-if="columns.some((col) => col.key === 'last_used_ip')"
          data-test="last-used-ip"
        >
          <slot name="cell-last_used_ip" :value="row.last_used_ip" :row="row" />
        </div>
      </div>
      <slot name="empty" />
    </div>
  `,
}

const SelectStub = {
  name: 'Select',
  props: ['modelValue', 'options'],
  emits: ['update:modelValue'],
  template: '<select :value="modelValue" @change="$emit(\'update:modelValue\', $event.target.value)"></select>',
}

const SearchInputStub = {
  name: 'SearchInput',
  props: ['modelValue'],
  emits: ['update:modelValue', 'search'],
  template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
}

const PaginationStub = {
  name: 'Pagination',
  props: ['page', 'total', 'pageSize'],
  emits: ['update:page', 'update:pageSize'],
  template: `
    <div>
      <button data-test="page-size-50" @click="$emit('update:pageSize', 50)">50</button>
    </div>
  `,
}

const IconStub = {
  props: ['name'],
  template: '<span data-test="icon">{{ name }}</span>',
}

const mountView = async () => {
  const wrapper = mount(KeysView, {
    global: {
      stubs: {
        AppLayout: AppLayoutStub,
        TablePageLayout: TablePageLayoutStub,
        DataTable: DataTableStub,
        Pagination: PaginationStub,
        BaseDialog: true,
        ConfirmDialog: true,
        EmptyState: true,
        Select: SelectStub,
        SearchInput: SearchInputStub,
        Icon: IconStub,
        UseKeyModal: true,
        EndpointPopover: true,
        GroupBadge: true,
        GroupOptionItem: true,
        Teleport: true,
      },
    },
  })
  await flushPromises()
  await nextTick()
  return wrapper
}

const visibleColumnKeys = (wrapper: VueWrapper) =>
  wrapper.get('[data-test="columns"]').text().split(',').filter(Boolean)

const visibleColumnMeta = (wrapper: VueWrapper): Array<{ key: string; sortable: boolean }> =>
  JSON.parse(wrapper.get('[data-test="columns-meta"]').text())

const getButtonByText = (wrapper: VueWrapper, text: string) => {
  const button = wrapper.findAll('button').find((item) => item.text().includes(text))
  if (!button) {
    throw new Error(`Button not found: ${text}`)
  }
  return button
}

const deferred = <T,>() => {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

describe('user KeysView column settings', () => {
  beforeEach(() => {
    localStorage.clear()

    listKeys.mockReset()
    getPublicSettings.mockReset()
    getDashboardApiKeysUsage.mockReset()
    getAvailableGroups.mockReset()
    getUserGroupRates.mockReset()
    showError.mockReset()
    showSuccess.mockReset()
    copyToClipboard.mockReset()
    isCurrentStep.mockReset()
    nextStep.mockReset()

    listKeys.mockResolvedValue({
      items: [createApiKey()],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })
    getPublicSettings.mockResolvedValue({})
    getDashboardApiKeysUsage.mockResolvedValue({ stats: {} })
    getAvailableGroups.mockResolvedValue([])
    getUserGroupRates.mockResolvedValue({})
    isCurrentStep.mockReturnValue(false)
  })

  it('uses the default API key columns with low-frequency columns hidden', async () => {
    const wrapper = await mountView()

    expect(visibleColumnKeys(wrapper)).toEqual([
      'name',
      'key',
      'group',
      'current_concurrency',
      'usage',
      'expires_at',
      'status',
      'created_at',
      'actions',
    ])
    expect(visibleColumnKeys(wrapper)).not.toContain('rate_limit')
    expect(visibleColumnKeys(wrapper)).not.toContain('last_used_at')
    expect(visibleColumnKeys(wrapper)).not.toContain('last_used_ip')
    expect(visibleColumnKeys(wrapper)).not.toContain('id')
  })

  it('shows a hidden column when toggled and persists the preference', async () => {
    const wrapper = await mountView()

    await wrapper.get('button[title="Column Settings"]').trigger('click')
    await getButtonByText(wrapper, 'Rate Limit').trigger('click')
    await nextTick()

    expect(visibleColumnKeys(wrapper)).toContain('rate_limit')
    expect(localStorage.getItem('api-key-hidden-columns')).toBe(
      JSON.stringify(['id', 'last_used_at', 'last_used_ip'])
    )
    expect(localStorage.getItem('api-key-column-settings-version')).toBe('3')
  })

  it('shows the API key ID column when toggled', async () => {
    const wrapper = await mountView()

    await wrapper.get('button[title="Column Settings"]').trigger('click')
    await getButtonByText(wrapper, 'ID').trigger('click')
    await nextTick()

    expect(visibleColumnKeys(wrapper)).toContain('id')
    expect(wrapper.get('[data-test="key-id"]').text()).toBe('#1')
    expect(visibleColumnMeta(wrapper).find((column) => column.key === 'id')?.sortable).toBe(true)
  })

  it('shows the last used IP column when toggled', async () => {
    listKeys.mockResolvedValue({
      items: [{ ...createApiKey(), last_used_ip: '203.0.113.10' }],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })
    const wrapper = await mountView()

    await wrapper.get('button[title="Column Settings"]').trigger('click')
    await getButtonByText(wrapper, 'Last Used IP').trigger('click')
    await flushPromises()
    await nextTick()

    expect(visibleColumnKeys(wrapper)).toContain('last_used_ip')
    expect(wrapper.get('[data-test="last-used-ip"]').text()).toBe('203.0.113.10')
    expect(listKeys).toHaveBeenCalledTimes(2)
    expect(listKeys.mock.calls[0][2]).toMatchObject({ include_last_used_ip: false })
    expect(listKeys.mock.calls[1][2]).toMatchObject({ include_last_used_ip: true })
    expect(getDashboardApiKeysUsage).toHaveBeenCalledTimes(1)
  })

  it('only loads usage after the usage column becomes visible', async () => {
    localStorage.setItem('api-key-hidden-columns', JSON.stringify(['usage', 'last_used_ip']))
    localStorage.setItem('api-key-column-settings-version', '3')

    const wrapper = await mountView()

    expect(visibleColumnKeys(wrapper)).not.toContain('usage')
    expect(getDashboardApiKeysUsage).not.toHaveBeenCalled()

    await wrapper.get('button[title="Column Settings"]').trigger('click')
    await getButtonByText(wrapper, 'Usage').trigger('click')
    await flushPromises()
    await nextTick()

    expect(visibleColumnKeys(wrapper)).toContain('usage')
    expect(getDashboardApiKeysUsage).toHaveBeenCalledTimes(1)
    expect(getDashboardApiKeysUsage.mock.calls[0][0]).toEqual([1])
  })

  it('shows a usage error and retries without reloading the key list', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    getDashboardApiKeysUsage
      .mockRejectedValueOnce(new Error('usage query timed out'))
      .mockResolvedValueOnce({
        stats: {
          '1': {
            api_key_id: 1,
            today_actual_cost: 2,
            total_actual_cost: 20,
          },
        },
      })

    const wrapper = await mountView()

    expect(wrapper.get('[data-test="usage-error"]').text()).toContain('Failed to load usage')
    expect(showError).toHaveBeenCalledWith('Failed to load usage')
    expect(listKeys).toHaveBeenCalledTimes(1)

    await wrapper.get('[data-test="usage-retry"]').trigger('click')
    await flushPromises()
    await nextTick()

    expect(getDashboardApiKeysUsage).toHaveBeenCalledTimes(2)
    expect(listKeys).toHaveBeenCalledTimes(1)
    expect(wrapper.find('[data-test="usage-error"]').exists()).toBe(false)
    expect(wrapper.get('[data-test="usage"]').text()).toContain('$2.0000')
    expect(wrapper.get('[data-test="usage"]').text()).toContain('$20.0000')
    consoleError.mockRestore()
  })

  it('restores column preferences from localStorage on mount', async () => {
    localStorage.setItem('api-key-hidden-columns', JSON.stringify(['group', 'created_at']))
    localStorage.setItem('api-key-column-settings-version', '1')

    const wrapper = await mountView()

    expect(visibleColumnKeys(wrapper)).toEqual([
      'name',
      'key',
      'current_concurrency',
      'usage',
      'rate_limit',
      'expires_at',
      'status',
      'last_used_at',
      'actions',
    ])
    expect(localStorage.getItem('api-key-hidden-columns')).toBe(
      JSON.stringify(['group', 'created_at', 'last_used_ip', 'id'])
    )
    expect(localStorage.getItem('api-key-column-settings-version')).toBe('3')
  })

  it('does not include always-visible columns in the toggleable menu', async () => {
    const wrapper = await mountView()

    await wrapper.get('button[title="Column Settings"]').trigger('click')
    await nextTick()

    const columnMenuText = wrapper.text()
    expect(columnMenuText).toContain('API Key')
    expect(columnMenuText).toContain('ID')
    expect(columnMenuText).toContain('Current Concurrency')
    expect(columnMenuText).toContain('Rate Limit')
    expect(columnMenuText).toContain('Last Used IP')
    expect(columnMenuText).not.toContain('Name')
    expect(columnMenuText).not.toContain('Actions')
  })

  it('renders the current concurrency value', async () => {
    const wrapper = await mountView()

    expect(wrapper.get('[data-test="current-concurrency"]').text()).toBe('3')
  })

  it('shows the API key list before the batch usage request completes', async () => {
    const usageRequest = deferred<{
      stats: Record<string, { today_actual_cost: number; total_actual_cost: number }>
    }>()
    getDashboardApiKeysUsage.mockReturnValueOnce(usageRequest.promise)

    const wrapper = await mountView()

    expect(wrapper.get('[data-test="table-loading"]').text()).toBe('false')
    expect(wrapper.get('[data-test="current-concurrency"]').text()).toBe('3')
    expect(wrapper.get('[data-test="usage-loading"]').exists()).toBe(true)

    usageRequest.resolve({
      stats: {
        '1': {
          today_actual_cost: 1.25,
          total_actual_cost: 9.5,
        },
      },
    })
    await flushPromises()
    await nextTick()

    expect(wrapper.find('[data-test="usage-loading"]').exists()).toBe(false)
    expect(wrapper.get('[data-test="usage"]').text()).toContain('$1.2500')
    expect(wrapper.get('[data-test="usage"]').text()).toContain('$9.5000')
  })

  it('splits large pages into backend-safe usage batches', async () => {
    const items = Array.from({ length: 101 }, (_, index) => ({
      ...createApiKey(),
      id: index + 1,
      key: `sk-test-key-${index + 1}`,
      name: `test-key-${index + 1}`,
    }))
    listKeys.mockResolvedValueOnce({
      items,
      total: items.length,
      page: 1,
      page_size: items.length,
      pages: 1,
    })
    getDashboardApiKeysUsage
      .mockResolvedValueOnce({ stats: {} })
      .mockResolvedValueOnce({ stats: {} })

    await mountView()

    expect(getDashboardApiKeysUsage).toHaveBeenCalledTimes(2)
    expect(getDashboardApiKeysUsage.mock.calls[0][0]).toHaveLength(100)
    expect(getDashboardApiKeysUsage.mock.calls[1][0]).toEqual([101])
  })

  it('cancels stale usage requests and ignores responses that arrive after a reload', async () => {
    const firstUsageRequest = deferred<{
      stats: Record<string, { today_actual_cost: number; total_actual_cost: number }>
    }>()
    const secondUsageRequest = deferred<{
      stats: Record<string, { today_actual_cost: number; total_actual_cost: number }>
    }>()
    getDashboardApiKeysUsage
      .mockReturnValueOnce(firstUsageRequest.promise)
      .mockReturnValueOnce(secondUsageRequest.promise)

    const wrapper = await mountView()
    const firstSignal = getDashboardApiKeysUsage.mock.calls[0][1].signal as AbortSignal

    await wrapper.get('[data-test="page-size-50"]').trigger('click')
    await flushPromises()
    await nextTick()

    expect(firstSignal.aborted).toBe(true)
    expect(getDashboardApiKeysUsage).toHaveBeenCalledTimes(2)

    secondUsageRequest.resolve({
      stats: {
        '1': {
          today_actual_cost: 2,
          total_actual_cost: 20,
        },
      },
    })
    await flushPromises()
    await nextTick()

    expect(wrapper.get('[data-test="usage"]').text()).toContain('$2.0000')
    expect(wrapper.get('[data-test="usage"]').text()).toContain('$20.0000')

    firstUsageRequest.resolve({
      stats: {
        '1': {
          today_actual_cost: 99,
          total_actual_cost: 999,
        },
      },
    })
    await flushPromises()
    await nextTick()

    expect(wrapper.get('[data-test="usage"]').text()).toContain('$2.0000')
    expect(wrapper.get('[data-test="usage"]').text()).not.toContain('$99.0000')
    expect(wrapper.get('[data-test="usage"]').text()).not.toContain('$999.0000')
  })

  it('keeps an in-flight usage request when loading the last used IP column', async () => {
    const usageRequest = deferred<{
      stats: Record<string, { today_actual_cost: number; total_actual_cost: number }>
    }>()
    getDashboardApiKeysUsage.mockReturnValueOnce(usageRequest.promise)
    listKeys.mockResolvedValue({
      items: [{ ...createApiKey(), last_used_ip: '203.0.113.10' }],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })

    const wrapper = await mountView()
    const usageSignal = getDashboardApiKeysUsage.mock.calls[0][1].signal as AbortSignal

    await wrapper.get('button[title="Column Settings"]').trigger('click')
    await getButtonByText(wrapper, 'Last Used IP').trigger('click')
    await flushPromises()
    await nextTick()

    expect(listKeys).toHaveBeenCalledTimes(2)
    expect(getDashboardApiKeysUsage).toHaveBeenCalledTimes(1)
    expect(usageSignal.aborted).toBe(false)

    usageRequest.resolve({
      stats: {
        '1': {
          today_actual_cost: 3,
          total_actual_cost: 30,
        },
      },
    })
    await flushPromises()
    await nextTick()

    expect(wrapper.get('[data-test="usage"]').text()).toContain('$3.0000')
    expect(wrapper.get('[data-test="usage"]').text()).toContain('$30.0000')
  })

  it('inherits usage loading when the last used IP reload replaces a pending list request', async () => {
    const initialListRequest = deferred<{
      items: ApiKey[]
      total: number
      page: number
      page_size: number
      pages: number
    }>()
    listKeys
      .mockReturnValueOnce(initialListRequest.promise)
      .mockResolvedValueOnce({
        items: [{ ...createApiKey(), id: 2, last_used_ip: '203.0.113.20' }],
        total: 1,
        page: 1,
        page_size: 20,
        pages: 1,
      })

    const wrapper = await mountView()

    await wrapper.get('button[title="Column Settings"]').trigger('click')
    await getButtonByText(wrapper, 'Last Used IP').trigger('click')
    await flushPromises()
    await nextTick()

    expect(listKeys).toHaveBeenCalledTimes(2)
    expect(getDashboardApiKeysUsage).toHaveBeenCalledTimes(1)
    expect(getDashboardApiKeysUsage.mock.calls[0][0]).toEqual([2])

    initialListRequest.resolve({
      items: [createApiKey()],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })
    await flushPromises()
  })

  it('cancels usage for stale keys when a pending list returns new keys', async () => {
    localStorage.setItem('api-key-hidden-columns', JSON.stringify(['usage', 'last_used_ip']))
    localStorage.setItem('api-key-column-settings-version', '3')

    const nextListRequest = deferred<{
      items: ApiKey[]
      total: number
      page: number
      page_size: number
      pages: number
    }>()
    const staleUsageRequest = deferred<{
      stats: Record<string, { today_actual_cost: number; total_actual_cost: number }>
    }>()
    listKeys
      .mockResolvedValueOnce({
        items: [createApiKey()],
        total: 1,
        page: 1,
        page_size: 20,
        pages: 1,
      })
      .mockReturnValueOnce(nextListRequest.promise)
    getDashboardApiKeysUsage
      .mockReturnValueOnce(staleUsageRequest.promise)
      .mockResolvedValueOnce({
        stats: {
          '2': {
            today_actual_cost: 4,
            total_actual_cost: 40,
          },
        },
      })

    const wrapper = await mountView()
    await wrapper.get('[data-test="page-size-50"]').trigger('click')
    await nextTick()

    await wrapper.get('button[title="Column Settings"]').trigger('click')
    await getButtonByText(wrapper, 'Usage').trigger('click')
    await nextTick()

    expect(getDashboardApiKeysUsage).toHaveBeenCalledTimes(1)
    expect(getDashboardApiKeysUsage.mock.calls[0][0]).toEqual([1])
    const staleSignal = getDashboardApiKeysUsage.mock.calls[0][1].signal as AbortSignal

    nextListRequest.resolve({
      items: [{ ...createApiKey(), id: 2 }],
      total: 1,
      page: 1,
      page_size: 50,
      pages: 1,
    })
    await flushPromises()
    await nextTick()

    expect(staleSignal.aborted).toBe(true)
    expect(getDashboardApiKeysUsage).toHaveBeenCalledTimes(2)
    expect(getDashboardApiKeysUsage.mock.calls[1][0]).toEqual([2])
    expect(wrapper.get('[data-test="usage"]').text()).toContain('$4.0000')
    expect(wrapper.get('[data-test="usage"]').text()).toContain('$40.0000')

    staleUsageRequest.resolve({
      stats: {
        '1': {
          today_actual_cost: 99,
          total_actual_cost: 999,
        },
      },
    })
    await flushPromises()
    await nextTick()

    expect(wrapper.get('[data-test="usage"]').text()).not.toContain('$99.0000')
  })

  it('marks current concurrency as sortable', async () => {
    const wrapper = await mountView()

    const currentConcurrencyColumn = visibleColumnMeta(wrapper).find(
      (column) => column.key === 'current_concurrency'
    )
    expect(currentConcurrencyColumn?.sortable).toBe(true)
  })

  it('keeps filters and selected page size when sorting by current concurrency', async () => {
    getAvailableGroups.mockResolvedValue([{ id: 42, name: 'OpenAI' }])
    const wrapper = await mountView()

    await wrapper.get('[data-test="page-size-50"]').trigger('click')
    await flushPromises()

    await wrapper.findComponent({ name: 'SearchInput' }).vm.$emit('update:modelValue', 'target')
    await wrapper.findComponent({ name: 'SearchInput' }).vm.$emit('search')
    await flushPromises()

    const selects = wrapper.findAllComponents({ name: 'Select' })
    await selects[0].vm.$emit('update:modelValue', 42)
    await flushPromises()
    await selects[1].vm.$emit('update:modelValue', 'active')
    await flushPromises()

    listKeys.mockClear()

    await wrapper.get('[data-test="sort-current-concurrency"]').trigger('click')
    await flushPromises()

    expect(listKeys).toHaveBeenLastCalledWith(
      1,
      50,
      {
        search: 'target',
        status: 'active',
        group_id: 42,
        sort_by: 'current_concurrency',
        sort_order: 'asc',
        include_last_used_ip: false,
      },
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )
  })
})
