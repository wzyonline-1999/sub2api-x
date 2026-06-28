import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ModelWhitelistSelector from '../ModelWhitelistSelector.vue'
import { accountsAPI } from '@/api/admin/accounts'

const showError = vi.hoisted(() => vi.fn())

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showInfo: vi.fn(),
    showSuccess: vi.fn()
  })
}))

vi.mock('@/api/admin/accounts', () => ({
  accountsAPI: {
    syncUpstreamModels: vi.fn(),
    syncUpstreamModelsPreview: vi.fn()
  }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        if (key === 'admin.accounts.syncUpstreamModels') return '同步上游支持的模型'
        if (key === 'admin.accounts.syncUpstreamModelsFailed') return '同步上游模型失败'
        if (key === 'admin.accounts.syncUpstreamModelsError') return `同步上游模型失败：${params?.message ?? ''}`
        return key
      }
    })
  }
})

function findSyncButton(wrapper: ReturnType<typeof mount>) {
  return wrapper.findAll('button').find(button => button.text() === '同步上游支持的模型')
}

describe('ModelWhitelistSelector upstream sync', () => {
  beforeEach(() => {
    showError.mockReset()
    vi.mocked(accountsAPI.syncUpstreamModels).mockReset()
    vi.mocked(accountsAPI.syncUpstreamModelsPreview).mockReset()
  })

  it('hides live upstream sync for existing OpenAI OAuth accounts', () => {
    const wrapper = mount(ModelWhitelistSelector, {
      props: {
        modelValue: [],
        platform: 'openai',
        accountType: 'oauth',
        accountId: 1
      }
    })

    expect(findSyncButton(wrapper)).toBeUndefined()
  })

  it('keeps live upstream sync for existing OpenAI API key accounts', () => {
    const wrapper = mount(ModelWhitelistSelector, {
      props: {
        modelValue: [],
        platform: 'openai',
        accountType: 'apikey',
        accountId: 1
      }
    })

    expect(findSyncButton(wrapper)).toBeTruthy()
  })

  it('hides live upstream sync for Grok accounts until backend model listing is supported', () => {
    const wrapper = mount(ModelWhitelistSelector, {
      props: {
        modelValue: [],
        platform: 'grok',
        accountType: 'oauth',
        accountId: 1
      }
    })

    expect(findSyncButton(wrapper)).toBeUndefined()
  })

  it('uses API plain-object message once when sync fails', async () => {
    vi.mocked(accountsAPI.syncUpstreamModels).mockRejectedValue({
      status: 400,
      message: 'OpenAI 认证登录账号暂不支持同步上游模型'
    })

    const wrapper = mount(ModelWhitelistSelector, {
      props: {
        modelValue: [],
        platform: 'openai',
        accountType: 'apikey',
        accountId: 1
      }
    })

    await findSyncButton(wrapper)!.trigger('click')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('同步上游模型失败：OpenAI 认证登录账号暂不支持同步上游模型')
  })

  it('does not duplicate the generic sync failure message', async () => {
    vi.mocked(accountsAPI.syncUpstreamModels).mockRejectedValue({
      status: 400,
      message: '同步上游模型失败'
    })

    const wrapper = mount(ModelWhitelistSelector, {
      props: {
        modelValue: [],
        platform: 'openai',
        accountType: 'apikey',
        accountId: 1
      }
    })

    await findSyncButton(wrapper)!.trigger('click')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('同步上游模型失败')
  })
})
