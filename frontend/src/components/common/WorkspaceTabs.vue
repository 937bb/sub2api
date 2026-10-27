<template>
  <div class="workbench-tabs" role="tablist" :aria-label="label">
    <button v-for="tab in tabs" :key="tab.value" type="button" role="tab"
      :aria-selected="modelValue === tab.value" :tabindex="modelValue === tab.value ? 0 : -1"
      @click="$emit('update:modelValue', tab.value)" @keydown="navigate($event, tab.value)">
      {{ tab.label }}
    </button>
  </div>
</template>
<script setup lang="ts">
const props = defineProps<{ modelValue: string; label: string; tabs: { value: string; label: string }[] }>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
function navigate(event: KeyboardEvent, value: string) {
  const index = props.tabs.findIndex(tab => tab.value === value)
  let next = index
  if (event.key === 'ArrowRight') next = (index + 1) % props.tabs.length
  else if (event.key === 'ArrowLeft') next = (index - 1 + props.tabs.length) % props.tabs.length
  else if (event.key === 'Home') next = 0
  else if (event.key === 'End') next = props.tabs.length - 1
  else return
  event.preventDefault()
  emit('update:modelValue', props.tabs[next].value)
  const buttons = (event.currentTarget as HTMLElement).parentElement?.querySelectorAll('button')
  buttons?.[next]?.focus()
}
</script>
