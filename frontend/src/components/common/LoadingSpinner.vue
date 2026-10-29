<template>
  <div
    :class="['loading-mark', sizeClasses, colorClass]"
    role="status"
    :aria-label="t('common.loading')"
  >
    <span></span>
    <span></span>
    <span></span>
    <span class="sr-only">{{ t('common.loading') }}</span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

type SpinnerSize = 'sm' | 'md' | 'lg' | 'xl'
type SpinnerColor = 'primary' | 'secondary' | 'white' | 'gray' | 'current'

interface Props {
  size?: SpinnerSize
  color?: SpinnerColor
}

const props = withDefaults(defineProps<Props>(), {
  size: 'md',
  color: 'primary'
})

const sizeClasses = computed(() => {
  const sizes: Record<SpinnerSize, string> = {
    sm: 'loading-mark-sm',
    md: 'loading-mark-md',
    lg: 'loading-mark-lg',
    xl: 'loading-mark-xl'
  }
  return sizes[props.size]
})

const colorClass = computed(() => {
  const colors: Record<SpinnerColor, string> = {
	current: '',
    primary: 'text-primary-500',
    secondary: 'text-gray-500 dark:text-dark-400',
    white: 'text-white',
    gray: 'text-gray-400 dark:text-dark-500'
  }
  return colors[props.color]
})
</script>

<style scoped>
.loading-mark {
  display: inline-grid;
  grid-template-columns: repeat(3, 1fr);
  align-items: end;
  gap: 2px;
  flex: none;
}
.loading-mark-sm { width: 14px; height: 14px; }
.loading-mark-md { width: 24px; height: 24px; gap: 3px; }
.loading-mark-lg { width: 34px; height: 34px; gap: 4px; }
.loading-mark-xl { width: 46px; height: 46px; gap: 5px; }
.loading-mark > span:not(.sr-only) {
  width: 100%;
  height: 38%;
  border-radius: 1px;
  background: currentColor;
  animation: loading-level 1.05s ease-in-out infinite;
}
.loading-mark > span:nth-child(2) { animation-delay: -0.72s; }
.loading-mark > span:nth-child(3) { animation-delay: -0.36s; }
@keyframes loading-level { 0%, 100% { height: 28%; opacity: .42; } 50% { height: 100%; opacity: 1; } }
@media (prefers-reduced-motion: reduce) { .loading-mark > span:not(.sr-only) { animation: none; height: 62%; } }
</style>
