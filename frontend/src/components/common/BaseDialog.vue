<template>
  <Teleport to="body">
    <Transition name="modal">
      <div
        v-if="show"
        class="modal-overlay console-overlay"
        :class="{ 'console-sheet-overlay': presentation === 'sheet' }"
        :style="zIndexStyle"
        :aria-labelledby="dialogId"
        role="dialog"
        aria-modal="true"
        @click.self="handleClose"
      >
        <!-- Modal panel -->
        <div ref="dialogRef" :class="['modal-content console-dialog', widthClasses]" :data-width="width" tabindex="-1" @click.stop>
          <!-- Header -->
          <div class="modal-header">
            <div class="console-dialog-heading">
              <h3 :id="dialogId" class="modal-title">{{ title }}</h3>
              <p v-if="description" class="console-dialog-description">{{ description }}</p>
            </div>
            <button
              v-if="showCloseButton"
              @click="emit('close')"
              type="button"
              class="console-dialog-close"
              :aria-label="closeLabel"
            >
              <Icon name="x" size="md" />
            </button>
          </div>

          <!-- Body -->
          <div v-if="$slots.navigation" class="console-dialog-navigation"><slot name="navigation"></slot></div>
          <div ref="modalBodyRef" class="modal-body" :class="bodyClass">
            <slot></slot>
          </div>

          <!-- Footer -->
          <div v-if="$slots.footer" class="modal-footer">
            <slot name="footer"></slot>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script lang="ts">
let dialogIdCounter = 0
const openDialogs = new Set<string>()
</script>

<script setup lang="ts">
import { computed, watch, onMounted, onUnmounted, ref, nextTick, getCurrentInstance } from 'vue'
import Icon from '@/components/icons/Icon.vue'

// Keep the primitive usable without an i18n plugin in isolated consumers.
const translate = getCurrentInstance()?.appContext.config.globalProperties.$t
const closeLabel = computed(() => translate?.('common.close') ?? 'Close modal')

// Each mounted dialog keeps its own label and place in the modal stack.
const dialogId = `modal-title-${++dialogIdCounter}`

// Restore the trigger when a dialog closes.
const dialogRef = ref<HTMLElement | null>(null)
const modalBodyRef = ref<HTMLElement | null>(null)
let previousActiveElement: HTMLElement | null = null

type DialogWidth = 'narrow' | 'normal' | 'wide' | 'extra-wide' | 'full'

interface Props {
  show: boolean
  title: string
  description?: string
  bodyClass?: string
  width?: DialogWidth
  closeOnEscape?: boolean
  closeOnClickOutside?: boolean
  showCloseButton?: boolean
  zIndex?: number
  presentation?: 'dialog' | 'sheet'
}

interface Emits {
  (e: 'close'): void
}

const props = withDefaults(defineProps<Props>(), {
  width: 'normal',
  closeOnEscape: true,
  closeOnClickOutside: false,
  showCloseButton: true,
  zIndex: 50,
  presentation: 'dialog'
})

const emit = defineEmits<Emits>()

// Custom z-index style (overrides the default z-50 from CSS)
const zIndexStyle = computed(() => {
  return props.zIndex !== 50 ? { zIndex: props.zIndex } : undefined
})

const widthClasses = computed(() => {
  // Width guidance: narrow=confirm/short prompts, normal=standard forms,
  // wide=multi-section forms or rich content, extra-wide=analytics/tables,
  // full=full-screen or very dense layouts.
  const widths: Record<DialogWidth, string> = {
    narrow: 'max-w-md',
    normal: 'max-w-lg',
    wide: 'w-full sm:max-w-2xl md:max-w-3xl lg:max-w-4xl',
    'extra-wide': 'w-full sm:max-w-3xl md:max-w-4xl lg:max-w-5xl xl:max-w-6xl',
    full: 'w-full sm:max-w-4xl md:max-w-5xl lg:max-w-6xl xl:max-w-7xl'
  }
  return widths[props.width]
})

const handleClose = () => {
  if (props.closeOnClickOutside) {
    emit('close')
  }
}

const handleEscape = (event: KeyboardEvent) => {
  if (!props.show || [...openDialogs].pop() !== dialogId || event.defaultPrevented) return
  if (props.closeOnEscape && event.key === 'Escape') {
    emit('close')
  }
  if (event.key !== 'Tab' || !dialogRef.value) return
  // Select menus are portalled siblings and manage their own keyboard focus.
  if (document.activeElement?.closest('[role="listbox"]')) return
  const items = Array.from(dialogRef.value.querySelectorAll<HTMLElement>(
    'button:not(:disabled), a[href], input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [tabindex]:not([tabindex="-1"])'
  )).filter((element) => element.getClientRects().length > 0 && !element.closest('[inert], [aria-hidden="true"]'))
  const target = event.shiftKey ? items.at(-1) : items[0]
  if (!items.length || !dialogRef.value.contains(document.activeElement) ||
    (event.shiftKey && document.activeElement === items[0]) ||
    (!event.shiftKey && document.activeElement === items.at(-1))) {
    event.preventDefault()
    ;(target || dialogRef.value).focus()
  }
}

const updateScrollLock = (isOpen: boolean) => {
  if (isOpen) openDialogs.add(dialogId)
  else openDialogs.delete(dialogId)
  document.body.classList.toggle('modal-open', openDialogs.size > 0)
}

// Prevent body scroll when modal is open and manage focus
watch(
  () => props.show,
  async (isOpen) => {
    if (isOpen) {
      // Save the trigger before locking document scroll.
      previousActiveElement = document.activeElement as HTMLElement
      updateScrollLock(true)

      // Wait for the panel to mount before moving focus.
      await nextTick()
      if (modalBodyRef.value) {
        modalBodyRef.value.scrollTop = 0
      }
      if (dialogRef.value && !dialogRef.value.contains(document.activeElement)) {
        const firstFocusable = dialogRef.value.querySelector<HTMLElement>(
          'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
        )
        firstFocusable?.focus()
      }
    } else {
      updateScrollLock(false)
      // Nested dialogs restore focus to their parent.
      if (previousActiveElement && typeof previousActiveElement.focus === 'function') {
        previousActiveElement.focus()
      }
      previousActiveElement = null
    }
  },
  { immediate: true }
)

onMounted(() => {
  document.addEventListener('keydown', handleEscape)
})

onUnmounted(() => {
  document.removeEventListener('keydown', handleEscape)
  // Release only this dialog's scroll lock.
  updateScrollLock(false)
  // Conditional dialogs unmount without first receiving show=false.
  if (previousActiveElement?.isConnected) previousActiveElement.focus()
})
</script>
