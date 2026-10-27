<template>
  <BaseDialog :show="Boolean(src)" :title="title || t('console.imagePreview')" width="full" :z-index="100" :close-on-click-outside="true" body-class="console-image-preview" @close="$emit('close')">
    <img :src="src" :alt="title || t('console.imagePreview')" />
  </BaseDialog>
</template>
<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import BaseDialog from './BaseDialog.vue'
defineProps<{ src: string; title?: string }>()
defineEmits<{ close: [] }>()
const { t } = useI18n()
</script>
