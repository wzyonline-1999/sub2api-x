import { describe, expect, it, vi } from 'vitest'
import { defineComponent, nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import OpsAlertEventsCard from '../OpsAlertEventsCard.vue'

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    listAlertEvents: vi.fn().mockResolvedValue([]),
    getAlertEvent: vi.fn(),
    createAlertSilence: vi.fn(),
    updateAlertEventStatus: vi.fn(),
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
  }),
}))

vi.mock('@/utils/basePath', () => ({
  appPath: (path: string) => `/sub2api${path.startsWith('/') ? path : `/${path}`}`,
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

const SelectStub = defineComponent({
  name: 'SelectStub',
  template: '<div class="select-stub" />',
})

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: {
    show: { type: Boolean, default: false },
  },
  template: '<div v-if="show" class="base-dialog-stub"><slot /></div>',
})

const IconStub = defineComponent({
  name: 'Icon',
  template: '<span class="icon-stub" />',
})

describe('OpsAlertEventsCard', () => {
  it('keeps alert detail links under the configured app base path', async () => {
    const wrapper = mount(OpsAlertEventsCard, {
      global: {
        stubs: {
          Select: SelectStub,
          BaseDialog: BaseDialogStub,
          Icon: IconStub,
        },
      },
    })

    ;(wrapper.vm as any).selected = {
      id: 10,
      rule_id: 88,
      severity: 'P1',
      status: 'firing',
      title: 'High 5xx',
      description: '',
      dimensions: { platform: 'openai', group_id: 7 },
      fired_at: '2026-05-31T00:00:00Z',
      created_at: '2026-05-31T00:00:00Z',
    }
    ;(wrapper.vm as any).showDetail = true
    await nextTick()

    const hrefs = wrapper.findAll('a').map((a) => a.attributes('href'))
    expect(hrefs).toContain('/sub2api/admin/ops?open_alert_rules=1&alert_rule_id=88')
    expect(hrefs).toContain('/sub2api/admin/ops?platform=openai&group_id=7&error_type=request&open_error_details=1')
  })
})
