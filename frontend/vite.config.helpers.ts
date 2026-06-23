export type DevProxyConfig = Record<string, { target: string; changeOrigin: true }>

export function normalizeBasePath(raw: string | undefined): string {
  const value = (raw || '/').trim()
  if (!value || value === '/') {
    return '/'
  }
  const withLeadingSlash = value.startsWith('/') ? value : `/${value}`
  return withLeadingSlash.endsWith('/') ? withLeadingSlash : `${withLeadingSlash}/`
}

function basePathPrefix(appBasePath: string): string {
  return appBasePath === '/' ? '' : appBasePath.replace(/\/$/, '')
}

export function resolveDevBackendBaseUrl(backendUrl: string, appBasePath: string): string {
  const trimmedBackend = backendUrl.replace(/\/+$/, '')
  const prefix = basePathPrefix(normalizeBasePath(appBasePath))
  if (!prefix) {
    return trimmedBackend
  }

  try {
    const parsed = new URL(trimmedBackend)
    const existingPath = parsed.pathname.replace(/\/+$/, '')
    if (existingPath === prefix || existingPath.endsWith(prefix)) {
      return trimmedBackend
    }
    parsed.pathname = `${existingPath === '/' ? '' : existingPath}${prefix}`
    return parsed.toString().replace(/\/+$/, '')
  } catch {
    return `${trimmedBackend}${prefix}`
  }
}

export function buildDevProxy(backendUrl: string, appBasePath: string): DevProxyConfig {
  const proxyRoots = [
    '/api',
    '/v1',
    '/v1beta',
    '/openai',
    '/backend-api',
    '/antigravity',
    '/setup',
    '/chat',
    '/health',
    '/responses',
    '/images',
  ]
  const prefix = basePathPrefix(normalizeBasePath(appBasePath))
  const proxy: DevProxyConfig = {}

  for (const root of proxyRoots) {
    proxy[root] = { target: backendUrl, changeOrigin: true }
    if (prefix) {
      proxy[`${prefix}${root}`] = { target: backendUrl, changeOrigin: true }
    }
  }

  return proxy
}
