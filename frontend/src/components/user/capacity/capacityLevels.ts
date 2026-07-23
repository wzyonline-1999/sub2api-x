export type CapacityLevel = 'healthy' | 'warning' | 'critical'

export function clampPercentage(value: number): number {
  if (!Number.isFinite(value)) return 0
  return Math.min(100, Math.max(0, value))
}

export function concurrencyLevel(loadPercentage: number, waiting = 0): CapacityLevel {
  if (waiting > 0 || loadPercentage >= 90) return 'critical'
  if (loadPercentage >= 70) return 'warning'
  return 'healthy'
}

export function loadCapabilityLevel(percentage: number): CapacityLevel {
  if (percentage >= 90) return 'healthy'
  if (percentage >= 60) return 'warning'
  return 'critical'
}

export function worstCapacityLevel(
  ...levels: Array<CapacityLevel | null | undefined>
): CapacityLevel {
  if (levels.includes('critical')) return 'critical'
  if (levels.includes('warning')) return 'warning'
  return 'healthy'
}

export function platformLabel(platform: string): string {
  const normalized = platform.trim().toLowerCase()
  const known: Record<string, string> = {
    anthropic: 'Anthropic',
    openai: 'OpenAI',
    gemini: 'Gemini',
    grok: 'Grok',
    xai: 'Grok',
    antigravity: 'Antigravity',
    composite: 'Composite',
  }
  return known[normalized] ?? (platform.trim() || 'Unknown')
}
