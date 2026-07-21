import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, put } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: { get, post, put }
}))

import subscriptionResetCardsAPI from '@/api/subscriptionResetCards'
import adminSubscriptionResetCardsAPI from '@/api/admin/subscriptionResetCards'

describe('subscription reset-card APIs', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    put.mockReset()
  })

  it('loads the current user inventory and bounded usage history', async () => {
    get.mockResolvedValueOnce({ data: [{ group_id: 8, remaining_count: 2 }] })
    await expect(subscriptionResetCardsAPI.getInventory()).resolves.toEqual([
      { group_id: 8, remaining_count: 2 }
    ])
    expect(get).toHaveBeenNthCalledWith(1, '/subscription-reset-cards')

    get.mockResolvedValueOnce({ data: [{ id: 11, mode: 'manual' }] })
    await expect(subscriptionResetCardsAPI.getUsages(12)).resolves.toEqual([
      { id: 11, mode: 'manual' }
    ])
    expect(get).toHaveBeenNthCalledWith(2, '/subscription-reset-cards/usages', {
      params: { limit: 12 }
    })
  })

  it('sends the caller-provided idempotency key when a user consumes a card', async () => {
    post.mockResolvedValue({ data: { subscription: { id: 31 }, usage: { id: 41 } } })

    await subscriptionResetCardsAPI.useCard(31, 'reset-card-operation-1')

    expect(post).toHaveBeenCalledWith('/subscriptions/31/reset-card/use', undefined, {
      headers: { 'Idempotency-Key': 'reset-card-operation-1' }
    })
  })

  it('updates the automatic-use preference for a subscription group', async () => {
    put.mockResolvedValue({ data: { group_id: 8, auto_use_enabled: true } })

    await subscriptionResetCardsAPI.updateAutoUsePreference(8, true)

    expect(put).toHaveBeenCalledWith('/subscription-reset-cards/preferences/8', {
      auto_use_enabled: true
    })
  })

  it('sends idempotency and exposes admin grant and audit-list endpoints', async () => {
    post.mockResolvedValueOnce({ data: { id: 5, issued_count: 3 } })
    await adminSubscriptionResetCardsAPI.grant(
      { user_id: 2, group_id: 8, count: 3, notes: 'retention' },
      'grant-operation-1'
    )
    expect(post).toHaveBeenCalledWith(
      '/admin/subscription-reset-cards/grants',
      { user_id: 2, group_id: 8, count: 3, notes: 'retention' },
      { headers: { 'Idempotency-Key': 'grant-operation-1' } }
    )

    get.mockResolvedValueOnce({ data: [{ id: 5 }] })
    await adminSubscriptionResetCardsAPI.listGrants({ user_id: 2, limit: 10, offset: 20 })
    expect(get).toHaveBeenNthCalledWith(1, '/admin/subscription-reset-cards/grants', {
      params: { user_id: 2, limit: 10, offset: 20 }
    })

    get.mockResolvedValueOnce({ data: [{ id: 6 }] })
    await adminSubscriptionResetCardsAPI.listUsages({ group_id: 8, limit: 20, offset: 40 })
    expect(get).toHaveBeenNthCalledWith(2, '/admin/subscription-reset-cards/usages', {
      params: { group_id: 8, limit: 20, offset: 40 }
    })
  })
})
