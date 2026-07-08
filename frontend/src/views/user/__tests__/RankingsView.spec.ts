import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const { getRankings, showError } = vi.hoisted(() => ({
  getRankings: vi.fn(),
  showError: vi.fn(),
}))

vi.mock('@/api/usage', () => ({
  getRankings,
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

const AppLayoutStub = { template: '<div><slot /></div>' }

function rankingItem(rank: number, totalTokens: number, actualCost = totalTokens / 100) {
  return {
    rank,
    user_id: rank,
    display_name: `user-${rank}`,
    total_tokens: totalTokens,
    actual_cost: actualCost,
    requests: rank * 10,
  }
}

const rankingResponse = {
  metric: 'tokens',
  period: 'month',
  start_date: '2026-06-01',
  end_date: '2026-06-28',
  summary: {
    total_tokens: 128_600_000,
    total_actual_cost: 1842.62,
    total_requests: 294_108,
    ranked_users: 128,
  },
  ranking: [
    {
      rank: 1,
      user_id: 1,
      display_name: 'zeyu.wang.1999',
      total_tokens: 128_600_000,
      actual_cost: 1842.62,
      requests: 12800,
    },
    {
      rank: 2,
      user_id: 2,
      display_name: 'codex',
      total_tokens: 99_400_000,
      actual_cost: 1104.71,
      requests: 9900,
    },
    {
      rank: 3,
      user_id: 3,
      display_name: 'gpt-5.5',
      total_tokens: 82_100_000,
      actual_cost: 928.4,
      requests: 8200,
    },
  ],
  current_user: {
    rank: 18,
    user_id: 18,
    display_name: 'me',
    total_tokens: 12_800_000,
    actual_cost: 128.6,
    requests: 1800,
  },
  current_user_target: {
    target_type: 'threshold',
    target_rank: 10,
    target_user_id: 10,
    target_display_name: 'user-10',
    gap_tokens: 1,
    gap_actual_cost: 0.01,
    progress_percent: 0,
  },
}

describe('RankingsView', () => {
  beforeEach(() => {
    getRankings.mockReset()
    showError.mockReset()
    getRankings.mockResolvedValue(rankingResponse)
  })

  it('renders the clean V3 controls and ranking cards', async () => {
    const RankingsView = (await import('../RankingsView.vue')).default
    const wrapper = mount(RankingsView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
        },
      },
    })

    await flushPromises()

    const text = wrapper.text()
    expect(text).toContain('使用量榜')
    expect(text).toContain('花费榜')
    expect(text).toContain('日榜')
    expect(text).toContain('周榜')
    expect(text).toContain('月榜')
    expect(text).toContain('zeyu.wang.1999')
    expect(text).toContain('金牌')
    expect(text).toContain('银牌')
    expect(text).toContain('铜牌')
    expect(text).toContain('我的排名')
    expect(text).toContain('全站实名账号')

    expect(text).not.toContain('榜单维度')
    expect(text).not.toContain('请求类型')
    expect(text).not.toContain('模型来源')
    expect(text).not.toContain('导出')
    expect(text).not.toContain('CSV')
    expect(text).not.toContain('截图')
    expect(text).not.toContain('全站匿名账号')
  })

  it('loads the day ranking by default', async () => {
    const RankingsView = (await import('../RankingsView.vue')).default
    mount(RankingsView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
        },
      },
    })

    await flushPromises()

    expect(getRankings).toHaveBeenCalledWith({
      metric: 'tokens',
      period: 'day',
      limit: 10,
    })
  })

  it('shows ranked state when the current user is in the top 10', async () => {
    getRankings.mockResolvedValueOnce({
      ...rankingResponse,
      ranking: [
        rankingItem(1, 1000),
        rankingItem(2, 900),
        rankingItem(3, 800),
      ],
      current_user: rankingItem(2, 900),
      current_user_target: {
        target_type: 'previous',
        target_rank: 1,
        target_user_id: 1,
        target_display_name: 'user-1',
        gap_tokens: 101,
        gap_actual_cost: 1.01,
        progress_percent: 90,
      },
    })
    const RankingsView = (await import('../RankingsView.vue')).default
    const wrapper = mount(RankingsView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
        },
      },
    })

    await flushPromises()

    const text = wrapper.text()
    expect(text).toContain('已上榜')
    expect(text).toContain('第 2 名')
    expect(text).toContain('900 tokens')
    expect(text).toContain('距离超过上一名还差 101 tokens')
  })

  it('shows summit state when the current user is first', async () => {
    getRankings.mockResolvedValueOnce({
      ...rankingResponse,
      ranking: [
        rankingItem(1, 1000),
        rankingItem(2, 900),
      ],
      current_user: rankingItem(1, 1000),
      current_user_target: {
        target_type: 'none',
        gap_tokens: 0,
        gap_actual_cost: 0,
        progress_percent: 100,
      },
    })
    const RankingsView = (await import('../RankingsView.vue')).default
    const wrapper = mount(RankingsView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
        },
      },
    })

    await flushPromises()

    const text = wrapper.text()
    expect(text).toContain('已登顶')
    expect(text).toContain('继续保持当前榜首')
  })

  it('shows the gap to enter the top 10 when the current user is unranked', async () => {
    getRankings.mockResolvedValueOnce({
      ...rankingResponse,
      ranking: [
        rankingItem(1, 1000),
        rankingItem(2, 900),
        rankingItem(3, 800),
        rankingItem(4, 700),
        rankingItem(5, 600),
        rankingItem(6, 500),
        rankingItem(7, 400),
        rankingItem(8, 350),
        rankingItem(9, 300),
        rankingItem(10, 250),
      ],
      current_user: rankingItem(18, 100),
      current_user_target: {
        target_type: 'threshold',
        target_rank: 10,
        target_user_id: 10,
        target_display_name: 'user-10',
        gap_tokens: 151,
        gap_actual_cost: 1.51,
        progress_percent: 40,
      },
    })
    const RankingsView = (await import('../RankingsView.vue')).default
    const wrapper = mount(RankingsView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
        },
      },
    })

    await flushPromises()

    const text = wrapper.text()
    expect(text).toContain('未上榜')
    expect(text).toContain('距离上榜还差 151 tokens')
  })

  it('reloads rankings when switching metric and period', async () => {
    const RankingsView = (await import('../RankingsView.vue')).default
    const wrapper = mount(RankingsView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
        },
      },
    })

    await flushPromises()
    await wrapper.get('[data-testid="ranking-metric-cost"]').trigger('click')
    await wrapper.get('[data-testid="ranking-period-week"]').trigger('click')

    expect(getRankings).toHaveBeenLastCalledWith({
      metric: 'cost',
      period: 'week',
      limit: 10,
    })
  })
})
