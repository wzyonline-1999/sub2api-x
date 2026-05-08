import { describe, expect, it } from 'vitest'
import { appWebSocketUrl } from '@/utils/basePath'

describe('basePath utilities', () => {
  it('normalizes explicit https websocket bases and keeps their path prefix', () => {
    expect(appWebSocketUrl('/api/v1/admin/ops/ws/qps', 'https://example.com/sub2api/')).toBe(
      'wss://example.com/sub2api/api/v1/admin/ops/ws/qps'
    )
  })

  it('normalizes host-only websocket bases with the current page protocol', () => {
    expect(appWebSocketUrl('/api/v1/admin/ops/ws/qps', 'example.com')).toBe(
      'ws://example.com/api/v1/admin/ops/ws/qps'
    )
  })
})
