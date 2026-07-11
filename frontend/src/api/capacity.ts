/**
 * User-facing resource capacity snapshot.
 *
 * The response intentionally contains group-level aggregates only. Keep account
 * identity, credentials, proxy details and administrative diagnostics out of
 * this contract.
 */
import { apiClient } from './client'

export interface CapacityConcurrency {
  current: number
  max: number
  remaining: number
  load_percentage: number
  waiting: number
}

export interface CapacityAccountConcurrency {
  current: number
  max: number
  load_percentage: number
  configured_accounts: number
}

export interface CapacityLoadCapability {
  available: number
  total: number
  percentage: number
}

export interface CapacityQuotaWindowLoad {
  load_percentage: number
  accounts_with_data: number
  total_accounts: number
}

export interface CapacityQuotaLoad {
  five_hour: CapacityQuotaWindowLoad | null
  seven_day: CapacityQuotaWindowLoad | null
}

export interface VisibleCapacityGroup {
  group_id: number
  name: string
  platform: string
  concurrency: CapacityConcurrency
  account_concurrency: CapacityAccountConcurrency | null
  quota_load?: CapacityQuotaLoad | null
  load_capacity: CapacityLoadCapability
}

export interface VisibleCapacitySnapshot {
  collected_at: string
  groups: VisibleCapacityGroup[]
}

export async function getVisible(options?: { signal?: AbortSignal }): Promise<VisibleCapacitySnapshot> {
  const { data } = await apiClient.get<VisibleCapacitySnapshot>('/capacity/visible', {
    signal: options?.signal,
  })
  return data
}

export const capacityAPI = {
  getVisible,
}

export default capacityAPI
