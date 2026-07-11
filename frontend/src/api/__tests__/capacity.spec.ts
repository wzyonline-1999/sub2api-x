import { beforeEach, describe, expect, it, vi } from 'vitest'

const get = vi.hoisted(() => vi.fn())

vi.mock('@/api/client', () => ({
  apiClient: { get },
}))

describe('capacity API', () => {
  beforeEach(() => {
    get.mockReset()
  })

  it('requests the authenticated visible-capacity endpoint with cancellation support', async () => {
    const snapshot = {
      collected_at: '2026-07-10T12:00:00Z',
      groups: [{
        group_id: 1,
        name: 'Primary',
        platform: 'openai',
        concurrency: {
          current: 2,
          max: 10,
          remaining: 8,
          load_percentage: 20,
          waiting: 0,
        },
        account_concurrency: null,
        quota_load: {
          five_hour: {
            load_percentage: 36,
            accounts_with_data: 3,
            total_accounts: 4,
          },
          seven_day: null,
        },
        load_capacity: { available: 4, total: 4, percentage: 100 },
      }],
    }
    get.mockResolvedValue({ data: snapshot })
    const controller = new AbortController()

    const { getVisible } = await import('@/api/capacity')
    await expect(getVisible({ signal: controller.signal })).resolves.toEqual(snapshot)

    expect(get).toHaveBeenCalledWith('/capacity/visible', {
      signal: controller.signal,
    })
  })
})
