import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import ConfirmDialog from '../ConfirmDialog.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

const BaseDialogStub = {
  props: ['show', 'title'],
  template: '<div><slot /><slot name="footer" /></div>'
}

function mountDialog(confirmDisabled: boolean) {
  return mount(ConfirmDialog, {
    props: {
      show: true,
      title: 'Confirm',
      message: 'Review details',
      confirmText: 'Restore',
      confirmDisabled
    },
    global: {
      stubs: { BaseDialog: BaseDialogStub }
    }
  })
}

describe('ConfirmDialog', () => {
  it('does not emit confirm while the action is disabled', async () => {
    const wrapper = mountDialog(true)
    const confirmButton = wrapper.findAll('button')[1]

    expect(confirmButton.attributes('disabled')).toBeDefined()
    await confirmButton.trigger('click')
    expect(wrapper.emitted('confirm')).toBeUndefined()
  })

  it('emits confirm when the action is enabled', async () => {
    const wrapper = mountDialog(false)
    const confirmButton = wrapper.findAll('button')[1]

    expect(confirmButton.attributes('disabled')).toBeUndefined()
    await confirmButton.trigger('click')
    expect(wrapper.emitted('confirm')).toHaveLength(1)
  })
})
