import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import BaseDialog from '../BaseDialog.vue'
import ImagePreview from '../ImagePreview.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)
afterEach(() => { vi.restoreAllMocks(); document.body.innerHTML = '' })

const tab = (shiftKey = false) => {
  const event = new KeyboardEvent('keydown', { key: 'Tab', shiftKey, cancelable: true })
  document.dispatchEvent(event)
  return event
}

describe('dialog keyboard containment', () => {
  it('wraps both directions and restores a parent trigger after conditional unmount', async () => {
    vi.spyOn(HTMLElement.prototype, 'getClientRects').mockReturnValue([{}] as unknown as DOMRectList)
    const parent = mount(BaseDialog, {
      attachTo: document.body,
      props: { show: true, title: 'Parent', showCloseButton: false },
      slots: { default: '<input value="unsaved"/><button id="nested-trigger">Verify</button>' }
    })
    await nextTick()
    const trigger = document.querySelector<HTMLButtonElement>('#nested-trigger')!
    trigger.focus()
    const child = mount(BaseDialog, {
      attachTo: document.body,
      props: { show: true, title: 'Verification', showCloseButton: false },
      slots: { default: '<button id="first">First</button><button id="last">Last</button>' }
    })
    await nextTick()
    const first = document.querySelector<HTMLButtonElement>('#first')!
    const last = document.querySelector<HTMLButtonElement>('#last')!
    expect(document.activeElement).toBe(first)
    expect(tab(true).defaultPrevented).toBe(true)
    expect(document.activeElement).toBe(last)
    expect(tab().defaultPrevented).toBe(true)
    expect(document.activeElement).toBe(first)
    child.unmount()
    expect(document.activeElement).toBe(trigger)
    expect(document.querySelector('input')?.value).toBe('unsaved')
    expect(document.body.classList.contains('modal-open')).toBe(true)
    parent.unmount()
    expect(document.body.classList.contains('modal-open')).toBe(false)
  })

  it('does not steal keyboard navigation from a portalled listbox', async () => {
    mount(BaseDialog, { attachTo: document.body, props: { show: true, title: 'Editor' } })
    await nextTick()
    const popup = document.createElement('div')
    popup.setAttribute('role', 'listbox')
    const search = document.createElement('input')
    popup.append(search); document.body.append(popup); search.focus()
    expect(tab().defaultPrevented).toBe(false)
    expect(document.activeElement).toBe(search)
  })

  it('renders the original image in the shared preview and dismisses it', async () => {
    const wrapper = mount(ImagePreview, { props: { src: 'data:image/png;base64,AAAA', title: 'Preview' } })
    await nextTick()
    expect(document.querySelector('img')?.getAttribute('src')).toBe('data:image/png;base64,AAAA')
    document.querySelector<HTMLButtonElement>('.console-dialog-close')!.click()
    expect(wrapper.emitted('close')).toHaveLength(1)
  })
})
