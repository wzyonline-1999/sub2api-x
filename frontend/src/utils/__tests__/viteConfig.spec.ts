import { describe, expect, it } from 'vitest'
import { buildDevProxy, normalizeBasePath, resolveDevBackendBaseUrl } from '../../../vite.config.helpers'

describe('vite sub-path development config', () => {
  it('normalizes the app base path with leading and trailing slashes', () => {
    expect(normalizeBasePath('sub2api')).toBe('/sub2api/')
    expect(normalizeBasePath('/sub2api/')).toBe('/sub2api/')
    expect(normalizeBasePath('/')).toBe('/')
  })

  it('adds sub-path proxy entries while keeping root development entries', () => {
    const proxy = buildDevProxy('http://localhost:8080', 'sub2api')

    expect(proxy['/api']?.target).toBe('http://localhost:8080')
    expect(proxy['/sub2api/api']?.target).toBe('http://localhost:8080')
    expect(proxy['/sub2api/v1']?.target).toBe('http://localhost:8080')
    expect(proxy['/sub2api/health']?.target).toBe('http://localhost:8080')
  })

  it('uses the mounted backend base URL for injected public settings without duplicating it', () => {
    expect(resolveDevBackendBaseUrl('http://localhost:8080', 'sub2api')).toBe('http://localhost:8080/sub2api')
    expect(resolveDevBackendBaseUrl('http://localhost:8080/sub2api', '/sub2api/')).toBe('http://localhost:8080/sub2api')
  })
})
