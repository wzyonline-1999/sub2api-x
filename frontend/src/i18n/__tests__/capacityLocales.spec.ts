import { describe, expect, it } from 'vitest'
import en from '../locales/en'
import zh from '../locales/zh'

describe('channel resources locale labels', () => {
  it('keeps the navigation and page titles aligned in both locales', () => {
    expect(zh.nav.resourceStatus).toBe('渠道资源')
    expect(zh.capacity.title).toBe('渠道资源')
    expect(en.nav.resourceStatus).toBe('Channel Resources')
    expect(en.capacity.title).toBe('Channel Resources')
  })
})
