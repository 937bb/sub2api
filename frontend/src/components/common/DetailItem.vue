<template>
  <div class="min-w-0">
    <dt class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ label }}</dt>
    <dd
      class="mt-1 break-words text-sm text-gray-900 dark:text-white"
      :class="mono ? 'font-mono text-xs' : ''"
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
}>(), {
  mono: false
})
</script>
