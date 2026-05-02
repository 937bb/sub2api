<template>
  <div ref="containerRef" class="relative inline-block w-full">
    <button
      type="button"
      class="input flex w-full items-center justify-between gap-1 text-left"
      @click="open = !open"
    >
      <span class="truncate text-sm">
        <template v-if="modelValue.length === 0">
          <span class="text-gray-400">{{ placeholder }}</span>
        </template>
        <template v-else>
          {{ selectedLabel }}
        </template>
      </span>
      <svg class="h-4 w-4 shrink-0 text-gray-400 transition-transform" :class="{ 'rotate-180': open }" viewBox="0 0 20 20" fill="currentColor">
        <path fill-rule="evenodd" d="M5.23 7.21a.75.75 0 011.06.02L10 11.168l3.71-3.938a.75.75 0 111.08 1.04l-4.25 4.5a.75.75 0 01-1.08 0l-4.25-4.5a.75.75 0 01.02-1.06z" clip-rule="evenodd" />
      </svg>
    </button>

    <Teleport to="body">
      <div
        v-if="open"
        ref="dropdownRef"
        class="fixed z-[9999] min-w-[180px] rounded-lg border border-gray-200 bg-white py-1 shadow-lg dark:border-dark-600 dark:bg-dark-800"
        :style="dropdownStyle"
      >
        <div class="max-h-48 overflow-y-auto">
          <label
            v-for="option in options"
            :key="option.value"
            class="flex cursor-pointer items-center gap-2 px-3 py-1.5 text-sm transition-colors hover:bg-gray-50 dark:hover:bg-dark-700"
          >
            <input
              type="checkbox"
              :checked="modelValue.includes(option.value)"
              @change="toggle(option.value)"
              class="h-3.5 w-3.5 shrink-0 rounded border-gray-300 text-primary-500 focus:ring-primary-500 dark:border-dark-500"
            />
            <span class="truncate text-gray-700 dark:text-gray-200">{{ option.label }}</span>
          </label>
          <div v-if="options.length === 0" class="px-3 py-2 text-center text-xs text-gray-400">
            {{ emptyText }}
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick, onBeforeUnmount } from 'vue'

interface Option {
  value: number
  label: string
}

interface Props {
  modelValue: number[]
  options: Option[]
  placeholder?: string
  emptyText?: string
}

const props = withDefaults(defineProps<Props>(), {
  placeholder: 'All',
  emptyText: 'No options',
})
const emit = defineEmits<{
  'update:modelValue': [value: number[]]
}>()

const open = ref(false)
const containerRef = ref<HTMLElement | null>(null)
const dropdownRef = ref<HTMLElement | null>(null)
const dropdownStyle = ref<Record<string, string>>({})

const selectedLabel = computed(() => {
  const names = props.options
    .filter((o) => props.modelValue.includes(o.value))
    .map((o) => o.label)
  if (names.length <= 2) return names.join(', ')
  return `${names[0]}, +${names.length - 1}`
})

function toggle(value: number) {
  const newValue = props.modelValue.includes(value)
    ? props.modelValue.filter((v) => v !== value)
    : [...props.modelValue, value]
  emit('update:modelValue', newValue)
}

function updatePosition() {
  if (!containerRef.value) return
  const rect = containerRef.value.getBoundingClientRect()
  const spaceBelow = window.innerHeight - rect.bottom
  const above = spaceBelow < 220

  dropdownStyle.value = {
    left: `${rect.left}px`,
    width: `${Math.max(rect.width, 180)}px`,
    ...(above
      ? { bottom: `${window.innerHeight - rect.top + 4}px` }
      : { top: `${rect.bottom + 4}px` }),
  }
}

watch(open, async (val) => {
  if (val) {
    await nextTick()
    updatePosition()
    document.addEventListener('click', onClickOutside, true)
    window.addEventListener('scroll', updatePosition, true)
  } else {
    document.removeEventListener('click', onClickOutside, true)
    window.removeEventListener('scroll', updatePosition, true)
  }
})

function onClickOutside(e: MouseEvent) {
  const target = e.target as Node
  if (containerRef.value?.contains(target) || dropdownRef.value?.contains(target)) return
  open.value = false
}

onBeforeUnmount(() => {
  document.removeEventListener('click', onClickOutside, true)
  window.removeEventListener('scroll', updatePosition, true)
})
</script>
