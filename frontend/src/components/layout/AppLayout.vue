<template>
  <div class="console-shell min-h-screen">
    <a href="#console-main" class="console-skip-link">{{ t('console.skipToContent') }}</a>

    <AppHeader />

    <!-- Sidebar -->
    <AppSidebar />

    <!-- Main Content Area -->
    <div
      class="console-workspace relative min-h-screen"
      :class="[sidebarCollapsed ? 'console-workspace-wide' : 'console-workspace-context']"
    >
      <!-- Main Content -->
      <main id="console-main" tabindex="-1" class="console-main">
        <slot />
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import '@/styles/onboarding.css'
import '@/styles/console.css'
import '@/styles/workbench.css'
import { useI18n } from 'vue-i18n'
import { computed, onMounted } from 'vue'
import { useAppStore } from '@/stores'
import { useAuthStore } from '@/stores/auth'
import { useOnboardingTour } from '@/composables/useOnboardingTour'
import { useOnboardingStore } from '@/stores/onboarding'
import AppSidebar from './AppSidebar.vue'
import AppHeader from './AppHeader.vue'

const appStore = useAppStore()
const { t } = useI18n()
const authStore = useAuthStore()
const sidebarCollapsed = computed(() => appStore.sidebarCollapsed)
const isAdmin = computed(() => authStore.user?.role === 'admin')

const { replayTour } = useOnboardingTour({
  storageKey: isAdmin.value ? 'admin_guide' : 'user_guide',
  autoStart: true
})

const onboardingStore = useOnboardingStore()

onMounted(() => {
  onboardingStore.setReplayCallback(replayTour)
})

defineExpose({ replayTour })
</script>
