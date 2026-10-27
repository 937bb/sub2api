<template>
  <div>
    <button type="button" class="relative btn btn-ghost btn-icon" :aria-label="t('announcements.title')" @click="openModal">
      <Icon name="bell" size="md" />
      <span v-if="unreadCount > 0" class="absolute right-1 top-1 h-1.5 w-1.5 rounded-full bg-red-500"></span>
    </button>
    <BaseDialog :show="isModalOpen" :title="t('announcements.title')" width="normal" :z-index="100" @close="closeModal">
      <div class="console-announcement-meta">
        <span>{{ unreadCount }} {{ t('announcements.unread') }}</span>
        <button v-if="unreadCount > 0" type="button" class="btn btn-secondary btn-sm" :disabled="loading" @click="markAllAsRead">{{ t('announcements.markAllRead') }}</button>
      </div>
      <div v-if="loading" class="flex justify-center py-10" role="status"><Icon name="refresh" size="md" class="animate-spin" /><span class="sr-only">{{ t('common.loading') }}</span></div>
      <div v-else-if="announcements.length" class="console-announcement-list">
        <button v-for="item in announcements" :key="item.id" type="button" class="console-announcement-row" @click="openDetail(item)">
          <Icon :name="item.read_at ? 'check' : 'bell'" size="sm" />
          <span><strong>{{ item.title }}</strong><time>{{ formatRelativeTime(item.created_at) }}</time></span>
          <span v-if="!item.read_at" class="badge badge-primary">{{ t('announcements.unread') }}</span>
          <Icon name="chevronRight" size="sm" />
        </button>
      </div>
      <EmptyState v-else :title="t('common.noData')" />
    </BaseDialog>
    <BaseDialog v-if="selectedAnnouncement" :show="detailModalOpen" :title="selectedAnnouncement.title" width="wide" :z-index="110" @close="closeDetail">
      <div class="console-announcement-meta"><time>{{ formatRelativeWithDateTime(selectedAnnouncement.created_at) }}</time><span>{{ selectedAnnouncement.read_at ? t('announcements.read') : t('announcements.unread') }}</span></div>
      <article class="console-announcement-content markdown-body prose prose-sm max-w-none dark:prose-invert" v-html="renderMarkdown(selectedAnnouncement.content)"></article>
      <template #footer>
        <span class="mr-auto text-xs text-gray-500">{{ selectedAnnouncement.read_at ? t('announcements.readStatus') : t('announcements.markReadHint') }}</span>
        <button type="button" class="btn btn-secondary" @click="closeDetail">{{ t('common.close') }}</button>
        <button v-if="!selectedAnnouncement.read_at" type="button" class="btn btn-primary" @click="markAsReadAndClose(selectedAnnouncement.id)">{{ t('announcements.markRead') }}</button>
      </template>
    </BaseDialog>
  </div>
</template>

<script setup lang="ts">
import BaseDialog from '@/components/common/BaseDialog.vue'
import { ref, computed } from 'vue'
import EmptyState from './EmptyState.vue'
import { useI18n } from 'vue-i18n'
import { storeToRefs } from 'pinia'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import { useAppStore } from '@/stores/app'
import { useAnnouncementStore } from '@/stores/announcements'
import { formatRelativeTime, formatRelativeWithDateTime } from '@/utils/format'
import type { UserAnnouncement } from '@/types'
import Icon from '@/components/icons/Icon.vue'
import '@/styles/announcement-markdown.css'

const { t } = useI18n()
const appStore = useAppStore()
const announcementStore = useAnnouncementStore()

// Configure marked
marked.setOptions({
  breaks: true,
  gfm: true,
})

// Use store state (storeToRefs for reactivity)
const { announcements, loading } = storeToRefs(announcementStore)
const unreadCount = computed(() => announcementStore.unreadCount)

// Local modal state
const isModalOpen = ref(false)
const detailModalOpen = ref(false)
const selectedAnnouncement = ref<UserAnnouncement | null>(null)

// Methods
function renderMarkdown(content: string): string {
  if (!content) return ''
  const html = marked.parse(content) as string
  return DOMPurify.sanitize(html)
}

function openModal() {
  isModalOpen.value = true
}

function closeModal() {
  isModalOpen.value = false
}

function openDetail(announcement: UserAnnouncement) {
  selectedAnnouncement.value = announcement
  detailModalOpen.value = true
  if (!announcement.read_at) {
    markAsRead(announcement.id)
  }
}

function closeDetail() {
  detailModalOpen.value = false
  selectedAnnouncement.value = null
}

async function markAsRead(id: number) {
  try {
    const marked = await announcementStore.markAsRead(id)
    if (!marked) appStore.showError(t('common.unknownError'))
    return marked
  } catch (err: any) {
    appStore.showError(err?.message || t('common.unknownError'))
    return false
  }
}

async function markAsReadAndClose(id: number) {
  if (!await markAsRead(id)) return
  appStore.showSuccess(t('announcements.markedAsRead'))
  closeDetail()
}

async function markAllAsRead() {
  try {
    await announcementStore.markAllAsRead()
    appStore.showSuccess(t('announcements.allMarkedAsRead'))
  } catch (err: any) {
    appStore.showError(err?.message || t('common.unknownError'))
  }
}

</script>

<style scoped>
/* Modal Animations */
.modal-fade-enter-active {
  transition: all 0.3s cubic-bezier(0.16, 1, 0.3, 1);
}

.modal-fade-leave-active {
  transition: all 0.2s cubic-bezier(0.4, 0, 1, 1);
}

.modal-fade-enter-from,
.modal-fade-leave-to {
  opacity: 0;
}

.modal-fade-enter-from > div {
  transform: scale(0.94) translateY(-12px);
  opacity: 0;
}

.modal-fade-leave-to > div {
  transform: scale(0.96) translateY(-8px);
  opacity: 0;
}

/* Scrollbar Styling */
.overflow-y-auto::-webkit-scrollbar {
  width: 8px;
}

.overflow-y-auto::-webkit-scrollbar-track {
  background: transparent;
}

.overflow-y-auto::-webkit-scrollbar-thumb {
  background: linear-gradient(to bottom, #cbd5e1, #94a3b8);
  border-radius: 4px;
}

.dark .overflow-y-auto::-webkit-scrollbar-thumb {
  background: linear-gradient(to bottom, #4b5563, #374151);
}

.overflow-y-auto::-webkit-scrollbar-thumb:hover {
  background: linear-gradient(to bottom, #94a3b8, #64748b);
}

.dark .overflow-y-auto::-webkit-scrollbar-thumb:hover {
  background: linear-gradient(to bottom, #6b7280, #4b5563);
}
</style>
