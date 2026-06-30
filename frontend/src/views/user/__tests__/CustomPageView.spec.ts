import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import CustomPageView from '../CustomPageView.vue'

const routeState = vi.hoisted(() => ({
  params: { id: 'docs' } as Record<string, string>,
}))

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRoute: () => routeState,
  }
})

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
      locale: { value: 'zh-CN' },
    }),
  }
})

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    cachedPublicSettings: {
      custom_menu_items: [
        {
          id: 'docs',
          title: 'Docs',
          url: 'md:guide',
          page_slug: 'guide',
        },
      ],
    },
    publicSettingsLoaded: true,
    fetchPublicSettings: vi.fn(),
  }),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    isAdmin: false,
    token: 'viewer-token',
    user: { id: 7 },
  }),
}))

vi.mock('@/stores/adminSettings', () => ({
  useAdminSettingsStore: () => ({
    customMenuItems: [],
  }),
}))

vi.mock('@/api/client', () => ({
  buildApiUrl: (path: string) => `/sub2api/api/v1${path.startsWith('/') ? path : `/${path}`}`,
}))

describe('CustomPageView', () => {
  beforeEach(() => {
    vi.stubGlobal('MutationObserver', class {
      observe() {}
      disconnect() {}
    })
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      text: async () => '# Guide\n\n![cat](images/cat.png?size=small)',
    }))
  })

  it('loads markdown pages and relative images through the mounted API base path', async () => {
    const wrapper = mount(CustomPageView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Icon: true,
        },
      },
    })

    await flushPromises()
    await flushPromises()

    expect(fetch).toHaveBeenCalledWith(
      '/sub2api/api/v1/pages/guide',
      expect.objectContaining({
        headers: { Authorization: 'Bearer viewer-token' },
      }),
    )
    expect(wrapper.html()).toContain('/sub2api/api/v1/pages/guide/images/images/cat.png?size=small')

    wrapper.unmount()
  })
})
