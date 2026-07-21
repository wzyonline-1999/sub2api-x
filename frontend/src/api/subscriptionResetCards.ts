import { apiClient } from './client'
import type {
  SubscriptionResetCardInventoryItem,
  SubscriptionResetCardUsage,
  UserSubscription
} from '@/types'

export interface UseSubscriptionResetCardResponse {
  subscription: UserSubscription
  usage: SubscriptionResetCardUsage
}

export async function getInventory(): Promise<SubscriptionResetCardInventoryItem[]> {
  const { data } = await apiClient.get<SubscriptionResetCardInventoryItem[]>(
    '/subscription-reset-cards'
  )
  return data
}

export async function getUsages(limit: number = 20): Promise<SubscriptionResetCardUsage[]> {
  const { data } = await apiClient.get<SubscriptionResetCardUsage[]>(
    '/subscription-reset-cards/usages',
    { params: { limit } }
  )
  return data
}

export async function useCard(
  subscriptionId: number,
  idempotencyKey: string
): Promise<UseSubscriptionResetCardResponse> {
  const { data } = await apiClient.post<UseSubscriptionResetCardResponse>(
    `/subscriptions/${subscriptionId}/reset-card/use`,
    undefined,
    { headers: { 'Idempotency-Key': idempotencyKey } }
  )
  return data
}

export async function updateAutoUsePreference(
  groupId: number,
  autoUseEnabled: boolean
): Promise<SubscriptionResetCardInventoryItem> {
  const { data } = await apiClient.put<SubscriptionResetCardInventoryItem>(
    `/subscription-reset-cards/preferences/${groupId}`,
    { auto_use_enabled: autoUseEnabled }
  )
  return data
}

export const subscriptionResetCardsAPI = {
  getInventory,
  getUsages,
  useCard,
  updateAutoUsePreference
}

export default subscriptionResetCardsAPI
