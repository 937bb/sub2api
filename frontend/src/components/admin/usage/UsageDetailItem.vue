<template>
  <div class="min-w-0">
    <dt class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ label }}</dt>
    <dd
      class="mt-1 break-words text-sm text-gray-900 dark:text-gray-100"
      :class="[mono ? 'font-mono text-xs' : '', strong ? 'font-semibold text-primary-600 dark:text-primary-400' : '']"
    >
      {{ value }}
    </dd>
  </div>
</template>

<script setup lang="ts">
withDefaults(defineProps<{
  label: string
  value: string
  mono?: boolean
  strong?: boolean
}>(), {
  mono: false,
  strong: false,
})
</script>
