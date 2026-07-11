import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import CapacityView from '../CapacityView.vue'
import type { VisibleCapacityGroup, VisibleCapacitySnapshot } from '@/api/capacity'

const getVisible = vi.hoisted(() => vi.fn())

vi.mock('@/api/capacity', () => ({
  default: { getVisible },
  capacityAPI: { getVisible },
  getVisible,
}))

vi.mock('@/utils/apiError', () => ({
  extractApiErrorMessage: (error: unknown, fallback: string) => error instanceof Error ? error.message : fallback,
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => params ? `${key}:${JSON.stringify(params)}` : key,
    }),
  }
})

const AppLayoutStub = { template: '<main><slot /></main>' }
const wrappers: VueWrapper[] = []

function group(id: number, overrides: Partial<VisibleCapacityGroup> = {}): VisibleCapacityGroup {
  return {
    group_id: id,
    name: `Group ${id}`,
    platform: id % 2 === 0 ? 'openai' : 'anthropic',
    concurrency: {
      current: 10,
      max: 40,
      remaining: 30,
      load_percentage: 25,
      waiting: 0,
    },
    account_concurrency: null,
    load_capacity: {
      available: 9,
      total: 10,
      percentage: 90,
    },
    ...overrides,
  }
}

function snapshot(groups: VisibleCapacityGroup[]): VisibleCapacitySnapshot {
  return {
    collected_at: new Date().toISOString(),
    groups,
  }
}

function mountView(): VueWrapper {
  const wrapper = mount(CapacityView, {
    global: {
      stubs: {
        AppLayout: AppLayoutStub,
      },
    },
  })
  wrappers.push(wrapper)
  return wrapper
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise
  })
  return { promise, resolve }
}

describe('CapacityView', () => {
  beforeEach(() => {
    vi.useRealTimers()
    window.localStorage.clear()
    getVisible.mockReset()
  })

  afterEach(() => {
    for (const wrapper of wrappers.splice(0)) wrapper.unmount()
    vi.useRealTimers()
  })

  it.each([
    [1, ['grid-cols-1'], ['md:grid-cols-2', 'xl:grid-cols-3']],
    [2, ['grid-cols-1', 'md:grid-cols-2'], ['xl:grid-cols-3']],
    [3, ['grid-cols-1', 'md:grid-cols-2', 'xl:grid-cols-3'], []],
    [4, ['grid-cols-1', 'md:grid-cols-2'], ['xl:grid-cols-3']],
  ])('derives the responsive grid from %s filtered groups', async (count, expected, forbidden) => {
    getVisible.mockResolvedValue(snapshot(Array.from({ length: count }, (_, index) => group(index + 1))))
    const wrapper = mountView()
    await flushPromises()

    const classes = wrapper.get('[data-testid="capacity-grid"]').classes()
    expect(wrapper.findAll('[data-testid="capacity-group-card"]')).toHaveLength(count)
    for (const className of expected) expect(classes).toContain(className)
    for (const className of forbidden) expect(classes).not.toContain(className)
  })

  it('recomputes layout after platform and search filters', async () => {
    getVisible.mockResolvedValue(snapshot([
      group(1, { name: 'Claude stable', platform: 'anthropic' }),
      group(2, { name: 'Codex primary', platform: 'openai' }),
      group(3, { name: 'Claude overflow', platform: 'anthropic' }),
      group(4, { name: 'Codex overflow', platform: 'openai' }),
    ]))
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="capacity-platform-openai"]').trigger('click')
    expect(wrapper.findAll('[data-testid="capacity-group-card"]')).toHaveLength(2)
    expect(wrapper.get('[data-testid="capacity-grid"]').classes()).toContain('md:grid-cols-2')

    await wrapper.get('[data-testid="capacity-search"]').setValue('primary')
    expect(wrapper.findAll('[data-testid="capacity-group-card"]')).toHaveLength(1)
    expect(wrapper.get('[data-testid="capacity-grid"]').classes()).toEqual(expect.arrayContaining(['grid-cols-1']))
    expect(wrapper.get('[data-testid="capacity-grid"]').classes()).not.toContain('md:grid-cols-2')
  })

  it('sorts pressure first and can restore backend order', async () => {
    getVisible.mockResolvedValue(snapshot([
      group(1, { name: 'Healthy' }),
      group(2, {
        name: 'Queued',
        concurrency: {
          current: 20,
          max: 100,
          remaining: 80,
          load_percentage: 20,
          waiting: 1,
        },
      }),
    ]))
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.findAll('[data-testid="capacity-group-card"]')[0].attributes('data-group-id')).toBe('2')

    await wrapper.get('[data-testid="capacity-sort"]').setValue('default')
    expect(wrapper.findAll('[data-testid="capacity-group-card"]')[0].attributes('data-group-id')).toBe('1')
  })

  it('includes quota load and schedulable capacity in pressure sorting and stressed counts', async () => {
    getVisible.mockResolvedValue(snapshot([
      group(1, { name: 'Healthy' }),
      group(2, {
        name: 'Quota warning',
        quota_load: {
          five_hour: {
            load_percentage: 75,
            accounts_with_data: 4,
            total_accounts: 4,
          },
          seven_day: null,
        },
      }),
      group(3, {
        name: 'Capacity critical',
        load_capacity: { available: 5, total: 10, percentage: 50 },
      }),
    ]))
    const wrapper = mountView()
    await flushPromises()

    const cards = wrapper.findAll('[data-testid="capacity-group-card"]')
    expect(cards.map((card) => card.attributes('data-group-id'))).toEqual(['3', '2', '1'])
    expect(cards.map((card) => card.attributes('data-pressure-level'))).toEqual([
      'critical',
      'warning',
      'healthy',
    ])

    const stressedSummary = wrapper.findAll('.capacity-summary-card')[3]
    expect(stressedSummary.text()).toContain('2 / 3')
    expect(stressedSummary.text()).toContain('"warning":1')
    expect(stressedSummary.text()).toContain('"critical":1')
  })

  it('shows first-load skeleton, then the empty state', async () => {
    const request = deferred<VisibleCapacitySnapshot>()
    getVisible.mockReturnValue(request.promise)
    const wrapper = mountView()
    await wrapper.vm.$nextTick()

    expect(wrapper.find('[data-testid="capacity-skeleton"]').exists()).toBe(true)

    request.resolve(snapshot([]))
    await flushPromises()
    expect(wrapper.find('[data-testid="capacity-skeleton"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="capacity-empty"]').exists()).toBe(true)
  })

  it('renders an inline retry state and recovers', async () => {
    getVisible
      .mockRejectedValueOnce(new Error('capacity unavailable'))
      .mockResolvedValueOnce(snapshot([group(1)]))
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-testid="capacity-error"]').text()).toContain('capacity unavailable')

    await wrapper.get('[data-testid="capacity-error"] button').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="capacity-error"]').exists()).toBe(false)
    expect(wrapper.findAll('[data-testid="capacity-group-card"]')).toHaveLength(1)
  })

  it('supports manual refresh and refreshes automatically every 30 seconds until unmount', async () => {
    vi.useFakeTimers()
    getVisible.mockResolvedValue(snapshot([group(1)]))
    const wrapper = mountView()
    await flushPromises()
    expect(getVisible).toHaveBeenCalledTimes(1)

    await wrapper.get('[data-testid="capacity-refresh"]').trigger('click')
    await flushPromises()
    expect(getVisible).toHaveBeenCalledTimes(2)

    await vi.advanceTimersByTimeAsync(30_000)
    await flushPromises()
    expect(getVisible).toHaveBeenCalledTimes(3)

    wrapper.unmount()
    await vi.advanceTimersByTimeAsync(60_000)
    expect(getVisible).toHaveBeenCalledTimes(3)
  })
})
