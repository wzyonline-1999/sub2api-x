import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import CapacityGroupCard from '../CapacityGroupCard.vue'
import type { VisibleCapacityGroup } from '@/api/capacity'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

function capacityGroup(overrides: Partial<VisibleCapacityGroup> = {}): VisibleCapacityGroup {
  return {
    group_id: 1,
    name: 'Claude stable pool',
    platform: 'anthropic',
    concurrency: {
      current: 18,
      max: 40,
      remaining: 22,
      load_percentage: 45,
      waiting: 0,
    },
    account_concurrency: {
      current: 9,
      max: 16,
      load_percentage: 56,
      configured_accounts: 8,
    },
    load_capacity: {
      available: 11,
      total: 12,
      percentage: 92,
    },
    ...overrides,
  }
}

describe('CapacityGroupCard', () => {
  it.each([
    [69, 0, 'healthy'],
    [70, 0, 'warning'],
    [89, 0, 'warning'],
    [90, 0, 'critical'],
    [10, 1, 'critical'],
  ])('maps group load %s with queue %s to %s', (load, waiting, expected) => {
    const group = capacityGroup({
      concurrency: {
        current: load,
        max: 100,
        remaining: 100 - load,
        load_percentage: load,
        waiting,
      },
    })
    const wrapper = mount(CapacityGroupCard, { props: { group } })

    expect(wrapper.get('[data-testid="capacity-group-card"]').attributes('data-pressure-level')).toBe(expected)
    expect(wrapper.get('[data-testid="group-concurrency"] [role="progressbar"]').attributes('data-level')).toBe(expected)
  })

  it('hides account concurrency when the aggregate is null', () => {
    const wrapper = mount(CapacityGroupCard, {
      props: { group: capacityGroup({ account_concurrency: null }) },
    })

    expect(wrapper.find('[data-testid="account-concurrency"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="concurrency-grid"]').attributes('data-columns')).toBe('1')
  })

  it('renders group and account concurrency as symmetric peer panels', () => {
    const wrapper = mount(CapacityGroupCard, {
      props: { group: capacityGroup() },
    })

    expect(wrapper.get('[data-testid="concurrency-grid"]').attributes('data-columns')).toBe('2')
    expect(wrapper.get('[data-testid="group-concurrency-value"]').classes()).toContain('text-xl')
    expect(wrapper.get('[data-testid="account-concurrency-value"]').classes()).toContain('text-xl')
    expect(wrapper.get('[data-testid="group-concurrency"] [role="progressbar"]').classes()).toContain('h-2')
    expect(wrapper.get('[data-testid="account-concurrency"] [role="progressbar"]').classes()).toContain('h-2')
    expect(wrapper.get('[data-testid="account-concurrency"]').text()).toContain('capacity.remaining')
  })

  it.each([
    [90, 'healthy'],
    [60, 'warning'],
    [59, 'critical'],
  ])('maps load capability %s to %s', (percentage, expected) => {
    const wrapper = mount(CapacityGroupCard, {
      props: {
        group: capacityGroup({
          load_capacity: { available: percentage, total: 100, percentage },
        }),
      },
    })

    expect(wrapper.get('[data-testid="load-capability"]').attributes('data-level')).toBe(expected)
  })

  it('uses the account load threshold independently from a queued group', () => {
    const wrapper = mount(CapacityGroupCard, {
      props: {
        group: capacityGroup({
          concurrency: {
            current: 9,
            max: 10,
            remaining: 1,
            load_percentage: 90,
            waiting: 4,
          },
          account_concurrency: {
            current: 8,
            max: 10,
            load_percentage: 80,
            configured_accounts: 4,
          },
        }),
      },
    })

    expect(wrapper.get('[data-testid="capacity-group-card"]').attributes('data-pressure-level')).toBe('critical')
    expect(wrapper.get('[data-testid="account-concurrency"]').attributes('data-level')).toBe('warning')
  })

  it.each([
    [69, 'healthy'],
    [70, 'warning'],
    [89, 'warning'],
    [90, 'critical'],
  ])('maps the 5-hour quota load %s to %s', (percentage, expected) => {
    const wrapper = mount(CapacityGroupCard, {
      props: {
        group: capacityGroup({
          account_concurrency: null,
          quota_load: {
            five_hour: {
              load_percentage: percentage,
              accounts_with_data: 4,
              total_accounts: 5,
            },
            seven_day: null,
          },
        }),
      },
    })

    expect(wrapper.get('[data-testid="quota-five-hour"]').attributes('data-level')).toBe(expected)
    expect(wrapper.get('[data-testid="quota-five-hour"] [role="progressbar"]').classes()).toContain('h-2')
  })

  it('renders 5-hour and 7-day quota loads as peers with account coverage', () => {
    const wrapper = mount(CapacityGroupCard, {
      props: {
        group: capacityGroup({
          quota_load: {
            five_hour: {
              load_percentage: 42,
              accounts_with_data: 7,
              total_accounts: 8,
            },
            seven_day: {
              load_percentage: 76,
              accounts_with_data: 6,
              total_accounts: 8,
            },
          },
        }),
      },
    })

    expect(wrapper.get('[data-testid="quota-load"]').text()).toContain('capacity.quotaCoverage')
    expect(wrapper.get('[data-testid="quota-five-hour-value"]').classes()).toContain('text-xl')
    expect(wrapper.get('[data-testid="quota-seven-day-value"]').classes()).toContain('text-xl')
    expect(wrapper.get('[data-testid="quota-seven-day"]').attributes('data-level')).toBe('warning')
    expect(wrapper.get('[data-testid="capacity-group-card"]').attributes('data-pressure-level')).toBe('warning')
  })

  it.each([
    [undefined],
    [null],
    [{ five_hour: null, seven_day: null }],
  ])('hides quota load when no window has data', (quotaLoad) => {
    const wrapper = mount(CapacityGroupCard, {
      props: {
        group: capacityGroup({ quota_load: quotaLoad }),
      },
    })

    expect(wrapper.find('[data-testid="quota-load"]').exists()).toBe(false)
  })

  it('uses the worst level from concurrency, quota load, and schedulable capacity for the badge', () => {
    const wrapper = mount(CapacityGroupCard, {
      props: {
        group: capacityGroup({
          concurrency: {
            current: 20,
            max: 100,
            remaining: 80,
            load_percentage: 20,
            waiting: 0,
          },
          account_concurrency: {
            current: 8,
            max: 10,
            load_percentage: 80,
            configured_accounts: 4,
          },
          quota_load: {
            five_hour: {
              load_percentage: 95,
              accounts_with_data: 4,
              total_accounts: 4,
            },
            seven_day: null,
          },
          load_capacity: { available: 10, total: 10, percentage: 100 },
        }),
      },
    })

    expect(wrapper.get('[data-testid="group-concurrency"]').attributes('data-level')).toBe('healthy')
    expect(wrapper.get('[data-testid="account-concurrency"]').attributes('data-level')).toBe('warning')
    expect(wrapper.get('[data-testid="quota-five-hour"]').attributes('data-level')).toBe('critical')
    expect(wrapper.get('[data-testid="capacity-group-card"]').attributes('data-pressure-level')).toBe('critical')
  })
})
