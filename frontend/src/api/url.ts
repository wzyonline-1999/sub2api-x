const DEFAULT_API_BASE_PATH = '/api/v1'
const API_BASE_URL = normalizeAPIBaseURL(import.meta.env.VITE_API_BASE_URL)

function normalizePath(path: string): string {
  return path.startsWith('/') ? path : `/${path}`
}

function normalizeAppBasePath(raw: unknown): string {
  const value = String(raw || '/').trim()
  if (!value || value === '/') {
    return '/'
  }

  const withLeadingSlash = normalizePath(value)
  return withLeadingSlash.endsWith('/') ? withLeadingSlash : `${withLeadingSlash}/`
}

function appPath(path: string): string {
  const suffix = normalizePath(path)
  const appBasePath = normalizeAppBasePath(import.meta.env.BASE_URL)
  if (appBasePath === '/') {
    return suffix
  }

  return `${appBasePath.replace(/\/$/, '')}${suffix}`
}

function defaultAPIBaseURL(): string {
  return appPath(DEFAULT_API_BASE_PATH)
}

function normalizeAPIBaseURL(value: unknown): string {
  const raw = String(value || defaultAPIBaseURL()).trim() || defaultAPIBaseURL()
  const withoutTrailingSlash = raw.replace(/\/+$/, '')
  if (/^[a-z][a-z\d+.-]*:\/\//i.test(withoutTrailingSlash) || withoutTrailingSlash.startsWith('//')) {
    return withoutTrailingSlash
  }
  return normalizePath(withoutTrailingSlash)
}

export function getAPIBaseURL(): string {
  return API_BASE_URL
}

export function buildApiUrl(path: string): string {
  const base = getAPIBaseURL().replace(/\/+$/, '')
  let suffix = normalizePath(path)
  if (suffix === DEFAULT_API_BASE_PATH) {
    suffix = ''
  } else if (suffix.startsWith(`${DEFAULT_API_BASE_PATH}/`)) {
    suffix = suffix.slice(DEFAULT_API_BASE_PATH.length)
  }
  return `${base}${suffix}`
}

function gatewayBaseURL(): string {
  const apiBase = getAPIBaseURL().replace(/\/+$/, '')
  const apiBasePathPattern = /\/api\/v1$/i
  const withoutApiRoot = apiBasePathPattern.test(apiBase)
    ? apiBase.replace(apiBasePathPattern, '')
    : apiBase

  if (/^[a-z][a-z\d+.-]*:\/\//i.test(withoutApiRoot) || withoutApiRoot.startsWith('//')) {
    return withoutApiRoot.replace(/\/+$/, '')
  }

  if (typeof window === 'undefined') {
    return withoutApiRoot.replace(/\/+$/, '')
  }

  return new URL(withoutApiRoot || '/', window.location.origin).toString().replace(/\/+$/, '')
}

export function buildGatewayUrl(path: string): string {
  const suffix = normalizePath(path)
  const base = gatewayBaseURL()
  if (!base) {
    return suffix
  }

  return `${base}${suffix}`
}
