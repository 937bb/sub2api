<template>
  <div class="auth-shell min-h-screen w-full lg:grid lg:grid-cols-[1.05fr_0.95fr]">
    <!-- ============ 左:品牌展示区(深色高级分屏,lg 以上显示) ============ -->
    <aside class="brand-panel relative hidden overflow-hidden p-12 xl:p-16 lg:flex lg:flex-col lg:justify-between">
      <!-- 细网格 + 单点柔光 + 顶部高光 -->
      <div class="brand-grid pointer-events-none absolute inset-0"></div>
      <div class="brand-glow pointer-events-none absolute inset-0"></div>

      <!-- 顶部 wordmark -->
      <div class="relative z-10 flex items-center gap-3">
        <div class="h-10 w-10 overflow-hidden rounded-xl ring-1 ring-white/15">
          <img :src="siteLogo || '/logo.png'" alt="Logo" class="h-full w-full object-contain" />
        </div>
        <span class="text-sm font-medium tracking-wide text-white/70">{{ siteName }}</span>
      </div>

      <!-- 中部:大标题 + 副标题 + 能力标签 -->
      <div class="relative z-10 max-w-md">
        <h1 class="font-display text-5xl font-extrabold leading-[1.05] tracking-tight text-white xl:text-6xl">
          {{ siteName }}
        </h1>
        <p class="mt-5 text-lg leading-relaxed text-white/60">
          {{ siteSubtitle }}
        </p>
        <ul class="mt-9 space-y-3.5">
          <li v-for="cap in capabilities" :key="cap" class="flex items-center gap-3 text-[15px] text-white/75">
            <span class="cap-dot"></span>
            {{ cap }}
          </li>
        </ul>
      </div>

      <!-- 底部:mono 装饰行 + 版权 -->
      <div class="relative z-10 space-y-4">
        <div class="brand-code inline-flex items-center gap-2 rounded-lg px-3 py-1.5 font-mono text-xs text-white/55">
          <span class="text-emerald-400">$</span>
          <span>curl -X POST /v1/messages</span>
        </div>
        <p class="text-xs text-white/35">&copy; {{ currentYear }} {{ siteName }}. All rights reserved.</p>
      </div>
    </aside>

    <!-- ============ 右:表单区 ============ -->
    <main class="relative flex items-center justify-center px-5 py-10 sm:px-8">
      <div class="relative z-10 w-full max-w-[420px]">
        <!-- 移动端品牌头 -->
        <div class="mb-9 flex flex-col items-center text-center lg:hidden">
          <div class="mb-4 h-14 w-14 overflow-hidden rounded-2xl shadow-glass-sm ring-1 ring-black/5 dark:ring-white/10">
            <img :src="siteLogo || '/logo.png'" alt="Logo" class="h-full w-full object-contain" />
          </div>
          <h1 class="font-display text-3xl font-bold tracking-tight text-gray-900 dark:text-white">
            {{ siteName }}
          </h1>
          <p class="mt-1.5 text-sm text-gray-500 dark:text-dark-400">{{ siteSubtitle }}</p>
        </div>

        <!-- 表单玻璃卡 -->
        <div class="auth-card">
          <slot />
        </div>

        <!-- 页脚 -->
        <div class="mt-6 text-center text-sm">
          <slot name="footer" />
        </div>
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'

const { t } = useI18n()
const appStore = useAppStore()

const siteName = computed(() => appStore.siteName || 'Sub2API')
const siteLogo = computed(() =>
  sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true })
)
const siteSubtitle = computed(
  () => appStore.cachedPublicSettings?.site_subtitle || 'Subscription to API Conversion Platform'
)

const capabilities = computed(() => [
  t('home.tags.subscriptionToApi'),
  t('home.tags.stickySession'),
  t('home.tags.realtimeBilling')
])

const currentYear = computed(() => new Date().getFullYear())

onMounted(() => {
  appStore.fetchPublicSettings()
})
</script>

<style scoped>
/* 右侧表单区背景由全局 body mesh 提供,保持透明 */

/* 左侧品牌深色面板 —— 中性炭黑 + 单点电光蓝柔光,不铺彩色 */
.brand-panel {
  background:
    radial-gradient(120% 80% at 15% 0%, #1a1c22 0%, #0b0c10 55%),
    #0b0c10;
}

.brand-grid {
  background-image:
    linear-gradient(rgba(255, 255, 255, 0.035) 1px, transparent 1px),
    linear-gradient(90deg, rgba(255, 255, 255, 0.035) 1px, transparent 1px);
  background-size: 56px 56px;
  mask-image: radial-gradient(120% 100% at 30% 20%, #000 0%, transparent 75%);
  -webkit-mask-image: radial-gradient(120% 100% at 30% 20%, #000 0%, transparent 75%);
}

/* 单一柔和电光蓝辉光(克制,只此一处) */
.brand-glow {
  background: radial-gradient(50% 45% at 78% 78%, rgba(10, 132, 255, 0.22) 0%, transparent 70%);
}

.cap-dot {
  height: 6px;
  width: 6px;
  border-radius: 9999px;
  background: #0a84ff;
  box-shadow: 0 0 10px rgba(10, 132, 255, 0.8);
  flex: none;
}

.brand-code {
  background: rgba(255, 255, 255, 0.04);
  border: 1px solid rgba(255, 255, 255, 0.08);
  box-shadow: inset 0 0.5px 0 rgba(255, 255, 255, 0.12);
}

/* 表单卡:中性磨砂玻璃 + 镜面高光边 */
.auth-card {
  background-color: var(--glass-bg-content);
  backdrop-filter: var(--glass-filter);
  -webkit-backdrop-filter: var(--glass-filter);
  border: 1px solid var(--glass-border);
  box-shadow: var(--glass-highlight), 0 20px 50px -20px rgba(0, 0, 0, 0.28);
  border-radius: 1.5rem;
  padding: 2rem;
}
</style>
