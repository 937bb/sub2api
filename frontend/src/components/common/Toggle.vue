<template>
  <button
    type="button"
    @click="toggle"
    class="console-switch"
    :disabled="disabled"
    role="switch"
    :aria-checked="modelValue"
  >
    <span />
  </button>
</template>

<script setup lang="ts">
const props = defineProps<{
  modelValue: boolean
  disabled?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
}>()

function toggle() {
  if (props.disabled) return
  emit('update:modelValue', !props.modelValue)
}
</script>
