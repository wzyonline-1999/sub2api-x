import { describe, expect, it } from 'vitest'
import { platformLabel, worstCapacityLevel } from '../capacityLevels'

describe('worstCapacityLevel', () => {
  it.each([
    [[], 'healthy'],
    [['healthy', null], 'healthy'],
    [['healthy', 'warning'], 'warning'],
    [['critical', 'healthy', 'warning'], 'critical'],
  ] as const)('returns the worst available level for %j', (levels, expected) => {
    expect(worstCapacityLevel(...levels)).toBe(expected)
  })
})

describe('platformLabel', () => {
  it('formats composite groups consistently with the other supported platforms', () => {
    expect(platformLabel('composite')).toBe('Composite')
  })
})
