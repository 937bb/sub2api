<template>
  <section class="console-form-section" data-dialog-section>
    <div class="console-section-heading">
      <h4>{{ title }}</h4>
      <p v-if="description">{{ description }}</p>
    </div>
    <div :class="columns === 2 ? 'console-field-grid' : 'console-section-fields'"><slot /></div>
  </section>
</template>

<script setup lang="ts">
withDefaults(defineProps<{ title: string; description?: string; columns?: 1 | 2 }>(), { columns: 1 })
</script>
