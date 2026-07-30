import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ModelWhitelistSelector from '../ModelWhitelistSelector.vue'
import { accountsAPI } from '@/api/admin/accounts'

const showError = vi.hoisted(() => vi.fn())
const copyToClipboard = vi.hoisted(() => vi.fn().mockResolvedValue(true))

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

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        if (key === 'common.copy') return '复制'
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

  it('keeps live upstream sync for existing Gemini OAuth accounts', () => {
    const wrapper = mount(ModelWhitelistSelector, {
      props: {
        modelValue: [],
        platform: 'gemini',
        accountType: 'oauth',
        accountId: 1
      }
    })

    expect(findSyncButton(wrapper)).toBeTruthy()
  })

  it('keeps live upstream sync for existing Grok accounts', () => {
    const wrapper = mount(ModelWhitelistSelector, {
      props: {
        modelValue: [],
        platform: 'grok',
        accountType: 'oauth',
        accountId: 1
      }
    })

    expect(findSyncButton(wrapper)).toBeTruthy()
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

function mountSelector() {
  return mount(ModelWhitelistSelector, {
    props: {
      modelValue: [],
      platform: 'openai'
    },
    global: {
      stubs: {
        ModelIcon: true
      }
    }
  })
}

function findModelRow(wrapper: ReturnType<typeof mountSelector>, modelId: string) {
  const row = wrapper
    .findAll('[data-testid="model-option"]')
    .find(candidate => candidate.text().includes(modelId))

  if (!row) {
    throw new Error(`Model row not found: ${modelId}`)
  }

  return row
}

describe('ModelWhitelistSelector', () => {
  beforeEach(() => {
    copyToClipboard.mockClear()
  })

  it('copies a model ID without selecting the model', async () => {
    const wrapper = mountSelector()
    await wrapper.get('div.cursor-pointer').trigger('click')

    const row = findModelRow(wrapper, 'gpt-5.6-sol')

    const copyButton = row.get('[data-testid="copy-model-id"]')
    expect(copyButton.attributes('aria-label')).toBe('复制 gpt-5.6-sol')

    await copyButton.trigger('click')
    await flushPromises()

    expect(copyToClipboard).toHaveBeenCalledWith('gpt-5.6-sol')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })

  it('keeps the existing model selection behavior', async () => {
    const wrapper = mountSelector()
    await wrapper.get('div.cursor-pointer').trigger('click')

    const row = findModelRow(wrapper, 'gpt-5.6-sol')
    await row.get('[data-testid="select-model"]').trigger('click')

    expect(wrapper.emitted('update:modelValue')).toEqual([[['gpt-5.6-sol']]])
    expect(copyToClipboard).not.toHaveBeenCalled()
  })
})
