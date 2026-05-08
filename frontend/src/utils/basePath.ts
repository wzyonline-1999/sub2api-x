function normalizePath(path: string): string {
  if (!path) {
    return '/'
  }
  return path.startsWith('/') ? path : `/${path}`
}

function normalizeBasePath(raw: string | undefined): string {
  const value = (raw || '/').trim()
  if (!value || value === '/') {
    return '/'
  }
  const withLeadingSlash = value.startsWith('/') ? value : `/${value}`
  return withLeadingSlash.endsWith('/') ? withLeadingSlash : `${withLeadingSlash}/`
}

export const APP_BASE_PATH = normalizeBasePath(import.meta.env.BASE_URL)

export function appPath(path: string = '/'): string {
  const normalizedPath = normalizePath(path)
  if (APP_BASE_PATH === '/') {
    return normalizedPath
  }
  return `${APP_BASE_PATH.replace(/\/$/, '')}${normalizedPath}`
}

export function appUrl(path: string = '/'): string {
  const resolvedPath = appPath(path)
  if (typeof window === 'undefined') {
    return resolvedPath
  }
  return new URL(resolvedPath, window.location.origin).toString()
}

export function appBaseUrl(): string {
  return appUrl('/').replace(/\/+$/, '')
}

export function stripAppBasePath(pathname: string): string {
  const normalizedPathname = normalizePath(pathname)
  if (APP_BASE_PATH === '/') {
    return normalizedPathname
  }

  const base = APP_BASE_PATH.replace(/\/$/, '')
  if (normalizedPathname === base) {
    return '/'
  }
  if (normalizedPathname.startsWith(`${base}/`)) {
    return normalizedPathname.slice(base.length) || '/'
  }
  return normalizedPathname
}

export function isCurrentAppPath(pathPrefix: string): boolean {
  if (typeof window === 'undefined') {
    return false
  }
  return stripAppBasePath(window.location.pathname).startsWith(normalizePath(pathPrefix))
}

export function defaultApiBaseUrl(): string {
  const configured = (import.meta.env.VITE_API_BASE_URL as string | undefined)?.trim()
  if (configured) {
    return configured.replace(/\/$/, '')
  }
  return appPath('/api/v1')
}

export function appWebSocketUrl(path: string, explicitBaseUrl?: string): string {
  if (typeof window === 'undefined') {
    return appPath(path)
  }

  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const configuredBase = explicitBaseUrl || (import.meta.env.VITE_WS_BASE_URL as string | undefined)?.trim()
  if (!configuredBase) {
    return new URL(appPath(path), `${protocol}//${window.location.host}`).toString()
  }

  const base = normalizeWebSocketBaseUrl(configuredBase, protocol)
  if (!base) {
    return new URL(appPath(path), `${protocol}//${window.location.host}`).toString()
  }

  const basePath = base.pathname.replace(/\/+$/, '')
  base.pathname = basePath && basePath !== '/' ? `${basePath}${normalizePath(path)}` : appPath(path)
  base.search = ''
  base.hash = ''
  return base.toString()
}

function normalizeWebSocketBaseUrl(raw: string, protocol: 'ws:' | 'wss:'): URL | null {
  const value = raw.trim().replace(/\/+$/, '')
  if (!value) {
    return null
  }

  let candidate = value
  if (candidate.startsWith('https://')) {
    candidate = `wss://${candidate.slice('https://'.length)}`
  } else if (candidate.startsWith('http://')) {
    candidate = `ws://${candidate.slice('http://'.length)}`
  } else if (candidate.startsWith('//')) {
    candidate = `${protocol}${candidate}`
  } else if (!candidate.startsWith('ws://') && !candidate.startsWith('wss://')) {
    candidate = `${protocol}//${candidate}`
  }

  try {
    return new URL(candidate)
  } catch {
    return null
  }
}
