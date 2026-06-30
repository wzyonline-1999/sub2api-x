import { describe, expect, it, vi, beforeEach } from 'vitest'

const get = vi.hoisted(() => vi.fn())

vi.mock('@/api/client', () => ({
  apiClient: { get },
}))

describe('usage rankings API', () => {
  beforeEach(() => {
    get.mockReset()
  })

  it('requests the shared rankings endpoint with metric and period', async () => {
    get.mockResolvedValue({
      data: {
        metric: 'tokens',
        period: 'month',
        ranking: [],
        summary: {
          total_tokens: 0,
          total_actual_cost: 0,
          total_requests: 0,
          ranked_users: 0,
        },
      },
    })

    const { getRankings } = await import('@/api/usage')
    await getRankings({ metric: 'tokens', period: 'month', limit: 10 })

    expect(get).toHaveBeenCalledWith('/usage/rankings', {
      params: { metric: 'tokens', period: 'month', limit: 10 },
    })
  })
})
