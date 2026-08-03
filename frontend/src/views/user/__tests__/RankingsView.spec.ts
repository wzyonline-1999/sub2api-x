import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../RankingsView.vue')
const componentSource = readFileSync(componentPath, 'utf8')

const { getRankings, showError, activeLocale } = vi.hoisted(() => ({
  getRankings: vi.fn(),
  showError: vi.fn(),
  activeLocale: { value: 'zh' },
}))

vi.mock('@/api/usage', () => ({
  getRankings,
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  const [zhDashboard, enDashboard] = await Promise.all([
    vi.importActual<typeof import('@/i18n/locales/zh/dashboard')>('@/i18n/locales/zh/dashboard'),
    vi.importActual<typeof import('@/i18n/locales/en/dashboard')>('@/i18n/locales/en/dashboard'),
  ])
  const catalogs: Record<string, unknown> = {
    zh: zhDashboard.default,
    en: enDashboard.default,
  }

  function translate(key: string, params: Record<string, unknown> = {}): string {
    let message: unknown = catalogs[activeLocale.value]
    for (const segment of key.split('.')) {
      if (!message || typeof message !== 'object') return key
      message = (message as Record<string, unknown>)[segment]
    }
    if (typeof message !== 'string') return key
    return message.replace(/\{(\w+)\}/g, (placeholder, name: string) => {
      return Object.hasOwn(params, name) ? String(params[name]) : placeholder
    })
  }

  return {
    ...actual,
    useI18n: () => ({
      locale: activeLocale,
      t: translate,
    }),
  }
})

const AppLayoutStub = { template: '<div><slot /></div>' }

function rankingItem(rank: number, totalTokens: number, actualCost = totalTokens / 100) {
  return {
    rank,
    user_id: rank,
    display_name: `user-${rank}`,
    avatar_url: `https://cdn.example.com/users/${rank}.png`,
    total_tokens: totalTokens,
    actual_cost: actualCost,
    requests: rank * 10,
    previous_requests: rank * 8,
    previous_total_tokens: Math.round(totalTokens * 0.8),
    previous_actual_cost: actualCost * 0.8,
  }
}

const rankingResponse = {
  metric: 'tokens',
  period: 'month',
  generated_at: '2026-06-28T12:34:56Z',
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
      avatar_url: 'https://cdn.example.com/users/zeyu.png',
      total_tokens: 128_600_000,
      actual_cost: 1842.62,
      requests: 12800,
    },
    {
      rank: 2,
      user_id: 2,
      display_name: 'codex',
      avatar_url: 'https://cdn.example.com/users/codex.png',
      total_tokens: 99_400_000,
      actual_cost: 1104.71,
      requests: 9900,
    },
    {
      rank: 3,
      user_id: 3,
      display_name: 'gpt-5.5',
      avatar_url: 'https://cdn.example.com/users/gpt.png',
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

function deferredRankingResponse() {
  let resolve!: (value: typeof rankingResponse) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<typeof rankingResponse>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

describe('RankingsView', () => {
  beforeEach(() => {
    getRankings.mockReset()
    showError.mockReset()
    activeLocale.value = 'zh'
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
    expect(text).not.toContain('金牌')
    expect(text).not.toContain('银牌')
    expect(text).not.toContain('铜牌')
    expect(wrapper.find('.medal').exists()).toBe(false)
    expect(text).toContain('我的排名')
    expect(text).toContain('全站实名账号')
    expect(text).toContain(new Date(rankingResponse.generated_at).toLocaleString('zh-CN'))

    const podiumFrames = wrapper.findAll('.podium-frame')
    expect(podiumFrames).toHaveLength(3)
    podiumFrames.forEach((frame) => {
      expect(frame.attributes('alt')).toBe('')
      expect(frame.attributes('aria-hidden')).toBe('true')
    })
    expect(podiumFrames[0].attributes('src')).toContain('gold-twin-dragon-frame-v2')
    expect(podiumFrames[1].attributes('src')).toContain('silver-twin-python-frame')
    expect(podiumFrames[2].attributes('src')).toContain('bronze-twin-snake-frame-v2')

    const podiumRanks = wrapper.findAll('.podium-rank')
    expect(podiumRanks.map((rank) => rank.text())).toEqual(['第 1 名', '第 2 名', '第 3 名'])
    podiumRanks.forEach((rank) => {
      expect(rank.classes()).toContain('baked-in-frame')
      expect(rank.attributes('aria-label')).toBeUndefined()
    })
    expect(wrapper.find('.podium-rank:not(.baked-in-frame)').exists()).toBe(false)
    expect(wrapper.find('.podium-rank-medallion').exists()).toBe(false)

    expect(text).not.toContain('榜单维度')
    expect(text).not.toContain('请求类型')
    expect(text).not.toContain('模型来源')
    expect(text).not.toContain('导出')
    expect(text).not.toContain('CSV')
    expect(text).not.toContain('截图')
    expect(text).not.toContain('全站匿名账号')
  })

  it('renders the complete rankings experience in English', async () => {
    activeLocale.value = 'en'
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
    expect(text).toContain('Usage')
    expect(text).toContain('Daily')
    expect(text).toContain('My rank')
    expect(text).toContain('Verified accounts site-wide')
    expect(text).not.toMatch(/[\u3400-\u9fff]/)
  })

  it('keeps dark-mode selectors scoped to ranking elements', () => {
    expect(componentSource).not.toContain(':global(.dark)')
    expect(componentSource).toContain('.dark .panel')
    expect(componentSource).toContain('.dark .podium-card.first')
    expect(componentSource).toContain('.dark .segment-button.active')
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

    expect(getRankings).toHaveBeenCalledWith(
      {
        metric: 'tokens',
        period: 'day',
        limit: 10,
      },
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )
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

  it('renders the detailed fourth-to-tenth ranking list', async () => {
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
      current_user: rankingItem(4, 700),
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

    const fourth = wrapper.get('[data-testid="ranking-row-4"]')
    expect(fourth.text()).toContain('user-4')
    expect(fourth.text()).toContain('40 次调用')
    expect(fourth.text()).toContain('我的账号')
    expect(fourth.text()).toContain('相对榜首')
    expect(fourth.text()).toContain('70%')
    expect(fourth.text()).toContain('较昨日同期')
    expect(fourth.text()).toContain('+25.0%')
    expect(fourth.text()).toContain('昨日 560 tokens')
    expect(fourth.text()).toContain('700 tokens')
    expect(fourth.text()).toContain('$7.00 实际花费')
    expect(fourth.get('[role="progressbar"]').attributes('aria-valuenow')).toBe('70')
    expect(fourth.get('.rank-number').text()).toContain('4')
    expect(fourth.get('.rank-number').attributes('aria-label')).toBe('第 4 名')
    expect(fourth.get('.rank-number strong').attributes('aria-hidden')).toBe('true')
    expect(fourth.get('.rank-number-sticker').attributes('aria-hidden')).toBe('true')
    const rankStickers = wrapper.findAll('.rank-number-sticker')
    expect(rankStickers).toHaveLength(7)
    rankStickers.forEach((sticker) => {
      expect(sticker.attributes('alt')).toBe('')
      expect(sticker.attributes('aria-hidden')).toBe('true')
      expect(sticker.attributes('draggable')).toBe('false')
      expect(sticker.attributes('src')).toBeTruthy()
    })
    expect(wrapper.text()).not.toContain('紧随领奖台的活跃用户')
    expect(fourth.get('[data-testid="ranking-avatar-4"] img').attributes('src')).toBe(
      'https://cdn.example.com/users/4.png'
    )
    expect(fourth.get('.rank-number').element.contains(fourth.get('.rank-avatar').element)).toBe(false)
  })

  it('falls back to the user initial only when the avatar is missing or fails to load', async () => {
    getRankings.mockResolvedValueOnce({
      ...rankingResponse,
      ranking: [
        rankingItem(1, 1000),
        rankingItem(2, 900),
        rankingItem(3, 800),
        { ...rankingItem(4, 700), avatar_url: '' },
        rankingItem(5, 600),
      ],
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

    const fourthAvatar = wrapper.get('[data-testid="ranking-avatar-4"]')
    expect(fourthAvatar.find('img').exists()).toBe(false)
    expect(fourthAvatar.text()).toBe('U')

    const fifthAvatar = wrapper.get('[data-testid="ranking-avatar-5"]')
    await fifthAvatar.get('img').trigger('error')
    expect(fifthAvatar.find('img').exists()).toBe(false)
    expect(fifthAvatar.text()).toBe('U')
  })

  it('renders new, flat, and declining comparison states and follows the selected period', async () => {
    const trendResponse = {
      ...rankingResponse,
      ranking: [
        rankingItem(1, 1000),
        rankingItem(2, 900),
        rankingItem(3, 800),
        { ...rankingItem(4, 700), previous_total_tokens: 0 },
        { ...rankingItem(5, 600), previous_total_tokens: 600 },
        { ...rankingItem(6, 500), previous_total_tokens: 625 },
      ],
    }
    getRankings.mockResolvedValue(trendResponse)

    const RankingsView = (await import('../RankingsView.vue')).default
    const wrapper = mount(RankingsView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
        },
      },
    })

    await flushPromises()

    expect(wrapper.get('[data-testid="ranking-trend-4"]').text()).toContain('新增')
    expect(wrapper.get('[data-testid="ranking-trend-4"]').classes()).toContain('up')
    expect(wrapper.get('[data-testid="ranking-trend-5"]').text()).toContain('持平')
    expect(wrapper.get('[data-testid="ranking-trend-5"]').classes()).toContain('flat')
    expect(wrapper.get('[data-testid="ranking-trend-6"]').text()).toContain('-20.0%')
    expect(wrapper.get('[data-testid="ranking-trend-6"]').classes()).toContain('down')

    await wrapper.get('[data-testid="ranking-period-week"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="ranking-trend-4"]').text()).toContain('较上周同期')
    expect(wrapper.get('[data-testid="ranking-trend-4"]').text()).toContain('上周 0 tokens')

    await wrapper.get('[data-testid="ranking-period-month"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="ranking-trend-4"]').text()).toContain('较上月同期')
    expect(wrapper.get('[data-testid="ranking-trend-4"]').text()).toContain('上月 0 tokens')
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

    expect(getRankings).toHaveBeenLastCalledWith(
      {
        metric: 'cost',
        period: 'week',
        limit: 10,
      },
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )
  })

  it('ignores stale responses when filters change quickly', async () => {
    const RankingsView = (await import('../RankingsView.vue')).default
    const wrapper = mount(RankingsView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
        },
      },
    })
    await flushPromises()

    const stale = deferredRankingResponse()
    const latest = deferredRankingResponse()
    getRankings.mockImplementation(({ metric, period }) => {
      if (metric === 'cost' && period === 'day') return stale.promise
      if (metric === 'cost' && period === 'week') return latest.promise
      return Promise.resolve(rankingResponse)
    })

    await wrapper.get('[data-testid="ranking-metric-cost"]').trigger('click')
    const staleSignal = getRankings.mock.calls.at(-1)?.[1]?.signal as AbortSignal
    await wrapper.get('[data-testid="ranking-period-week"]').trigger('click')
    const latestSignal = getRankings.mock.calls.at(-1)?.[1]?.signal as AbortSignal

    expect(staleSignal.aborted).toBe(true)
    expect(latestSignal.aborted).toBe(false)

    stale.resolve({
      ...rankingResponse,
      ranking: [{ ...rankingItem(1, 100), display_name: 'stale-result' }],
    })
    await flushPromises()

    expect(wrapper.find('.loading-panel').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('stale-result')

    latest.resolve({
      ...rankingResponse,
      ranking: [{ ...rankingItem(1, 200), display_name: 'latest-result' }],
    })
    await flushPromises()

    expect(wrapper.find('.loading-panel').exists()).toBe(false)
    expect(wrapper.text()).toContain('latest-result')
    expect(wrapper.text()).not.toContain('stale-result')
    expect(showError).not.toHaveBeenCalled()
  })

  it('ignores a pending request after the view is unmounted', async () => {
    const pending = deferredRankingResponse()
    getRankings.mockReturnValueOnce(pending.promise)
    const RankingsView = (await import('../RankingsView.vue')).default
    const wrapper = mount(RankingsView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
        },
      },
    })

    const signal = getRankings.mock.calls[0][1].signal as AbortSignal
    wrapper.unmount()
    expect(signal.aborted).toBe(true)
    pending.reject(new Error('late ranking failure'))
    await flushPromises()

    expect(showError).not.toHaveBeenCalled()
  })

  it('uses singular English labels for one token and one call', async () => {
    activeLocale.value = 'en'
    getRankings.mockResolvedValue({
      ...rankingResponse,
      ranking: [
        rankingItem(1, 3),
        rankingItem(2, 2),
        rankingItem(3, 1),
        { ...rankingItem(4, 1), requests: 1 },
      ],
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

    const fourth = wrapper.get('[data-testid="ranking-row-4"]')
    expect(fourth.text()).toContain('1 token')
    expect(fourth.text()).toContain('1 call')
    expect(fourth.text()).not.toContain('1 tokens')
    expect(fourth.text()).not.toContain('1 calls')

    await wrapper.get('[data-testid="ranking-metric-cost"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="ranking-row-4"]').text()).toContain('1 token')
  })

  it('does not show the previous filter summary while the next ranking is loading', async () => {
    const RankingsView = (await import('../RankingsView.vue')).default
    const wrapper = mount(RankingsView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
        },
      },
    })
    await flushPromises()
    expect(wrapper.find('.stats-grid').exists()).toBe(true)

    const pending = deferredRankingResponse()
    getRankings.mockReturnValueOnce(pending.promise)
    await wrapper.get('[data-testid="ranking-period-week"]').trigger('click')

    expect(wrapper.find('.loading-panel').exists()).toBe(true)
    expect(wrapper.find('.stats-grid').exists()).toBe(false)

    pending.resolve({ ...rankingResponse, period: 'week' })
    await flushPromises()
    expect(wrapper.find('.stats-grid').exists()).toBe(true)
  })

  it('clears data when the latest filter request fails', async () => {
    const RankingsView = (await import('../RankingsView.vue')).default
    const wrapper = mount(RankingsView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
        },
      },
    })
    await flushPromises()
    expect(wrapper.text()).toContain('zeyu.wang.1999')

    const failed = deferredRankingResponse()
    getRankings.mockReturnValueOnce(failed.promise)
    await wrapper.get('[data-testid="ranking-metric-cost"]').trigger('click')
    failed.reject(new Error('ranking unavailable'))
    await flushPromises()

    expect(wrapper.find('.loading-panel').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('zeyu.wang.1999')
    expect(wrapper.text()).toContain('暂无排行数据')
    expect(showError).toHaveBeenCalledWith('ranking unavailable')
  })

  it('scales relative cost progress against a sub-dollar leader', async () => {
    getRankings.mockResolvedValue({
      ...rankingResponse,
      metric: 'cost',
      ranking: [
        { ...rankingItem(1, 100, 0.2), display_name: 'cost-leader' },
        { ...rankingItem(2, 90, 0.15), display_name: 'cost-runner-up' },
        { ...rankingItem(3, 80, 0.12), display_name: 'cost-third' },
        { ...rankingItem(4, 70, 0.1), display_name: 'cost-fourth' },
      ],
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
    await wrapper.get('[data-testid="ranking-metric-cost"]').trigger('click')
    await flushPromises()

    const fourthRow = wrapper.get('[data-testid="ranking-row-4"]')
    expect(fourthRow.get('[role="progressbar"]').attributes('aria-valuenow')).toBe('50')
    expect(fourthRow.text()).toContain('50%')
  })
})
