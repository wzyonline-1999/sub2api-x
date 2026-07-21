import { apiClient } from '../client'
import type {
  GrantSubscriptionResetCardsRequest,
  SubscriptionResetCardGrant,
  SubscriptionResetCardUsage
} from '@/types'

export async function grant(
  request: GrantSubscriptionResetCardsRequest,
  idempotencyKey: string
): Promise<SubscriptionResetCardGrant> {
  const { data } = await apiClient.post<SubscriptionResetCardGrant>(
    '/admin/subscription-reset-cards/grants',
    request,
    { headers: { 'Idempotency-Key': idempotencyKey } }
  )
  return data
}

export async function listGrants(
  filters?: { user_id?: number; group_id?: number; limit?: number; offset?: number }
): Promise<SubscriptionResetCardGrant[]> {
  const { data } = await apiClient.get<SubscriptionResetCardGrant[]>(
    '/admin/subscription-reset-cards/grants',
    { params: filters }
  )
  return data
}

export async function listUsages(
  filters?: { user_id?: number; group_id?: number; limit?: number; offset?: number }
): Promise<SubscriptionResetCardUsage[]> {
  const { data } = await apiClient.get<SubscriptionResetCardUsage[]>(
    '/admin/subscription-reset-cards/usages',
    { params: filters }
  )
  return data
}

export const subscriptionResetCardsAPI = {
  grant,
  listGrants,
  listUsages
}

export default subscriptionResetCardsAPI
