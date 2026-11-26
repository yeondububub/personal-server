<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue'
import { Lock } from 'lucide-vue-next'
import { useAuthStore } from '../../stores/auth'
import { DESKTOP_APPS, useDesktopStore, type DesktopAppDefinition } from '../../stores/desktop'
import { useMountsStore } from '../../stores/mounts'
import { api } from '../../api/client'
import type { SystemSummary } from '../../types/api'
import { formatBytes, formatDuration } from '../../utils/format'
import DsmAppIcon from './DsmAppIcon.vue'
import DsmTaskbar from './DsmTaskbar.vue'
import DsmWallpaper, { type DsmWallpaperTheme } from './DsmWallpaper.vue'
import DsmWindow from './DsmWindow.vue'
import FileStationApp from '../../apps/file-station/FileStationApp.vue'
import VideoPlayerApp from '../../apps/video-player/VideoPlayerApp.vue'
import CodeEditorApp from '../../apps/code-editor/CodeEditorApp.vue'
import MountManagerApp from '../../apps/mount-manager/MountManagerApp.vue'
import MonitorApp from '../../apps/monitor/MonitorApp.vue'
import WargameRunnerApp from '../../apps/wargame/WargameRunnerApp.vue'
import PhasePreviewApp from '../../apps/placeholders/PhasePreviewApp.vue'

const WALLPAPER_STORAGE_KEY = 'ps_dsm_wallpaper'

function parseWallpaperTheme(raw: string | null): DsmWallpaperTheme {
  switch (raw) {
    case 'ocean':
    case 'mist':
    case 'slate':
      return raw
    default:
      return 'ocean'
  }
}

function loadInitialWallpaper(): DsmWallpaperTheme {
  try {
    return parseWallpaperTheme(localStorage.getItem(WALLPAPER_STORAGE_KEY))
  } catch {
    return 'ocean'
  }
}

const authStore = useAuthStore()
const desktopStore = useDesktopStore()
const mountsStore = useMountsStore()

const wallpaperTheme = ref<DsmWallpaperTheme>(loadInitialWallpaper())
const widgetOpen = ref(true)
const summary = ref<SystemSummary | null>(null)
const toastMessage = ref('')
let summaryTimer: ReturnType<typeof setInterval> | null = null
let toastTimer: ReturnType<typeof setTimeout> | null = null

function handleChangeWallpaper(theme: DsmWallpaperTheme): void {
  wallpaperTheme.value = theme
  try {
    localStorage.setItem(WALLPAPER_STORAGE_KEY, theme)
  } catch {
    // Ignore storage write errors in restricted browsing contexts
  }
}

function handleToggleWidget(): void {
  widgetOpen.value = !widgetOpen.value
}

function showToast(msg: string): void {
  toastMessage.value = msg
  if (toastTimer) clearTimeout(toastTimer)
  toastTimer = setTimeout(() => {
    toastMessage.value = ''
  }, 4000)
}

async function refreshSummary(): Promise<void> {
  try {
    summary.value = await api.fetchSystemSummary()
  } catch {
    // Ignore transient widget poll failures
  }
}

onMounted(() => {
  void refreshSummary()
  summaryTimer = setInterval(() => {
    void refreshSummary()
  }, 10000)
})

watch(
  () => mountsStore.version,
  () => {
    void refreshSummary()
  },
)

onUnmounted(() => {
  if (summaryTimer) clearInterval(summaryTimer)
  if (toastTimer) clearTimeout(toastTimer)
})

function handleLaunchApp(app: DesktopAppDefinition): void {
  if (app.requiresPrivate && authStore.accessTier === 'public') {
    showToast(`${app.title} 앱은 Tailscale 내부 관리망 접속 시에만 실행할 수 있습니다.`)
    return
  }
  switch (app.id) {
    case 'file-station':
      desktopStore.openWindow({ title: app.title, payload: { kind: 'file-station' } })
      return
    case 'mounts':
      desktopStore.openWindow({ title: app.title, payload: { kind: 'mounts' } })
      return
    case 'wargame':
      desktopStore.openWindow({ title: app.title, payload: { kind: 'wargame' } })
      return
    case 'terminal':
      desktopStore.openWindow({ title: app.title, payload: { kind: 'terminal' } })
      return
    case 'monitor':
      desktopStore.openWindow({ title: app.title, payload: { kind: 'monitor' } })
      return
    case 'video-player':
    case 'code-editor':
      return
    default: {
      const _exhaustive: never = app.id
      return _exhaustive
    }
  }
}

function diskUsagePercent(total: number, free: number): number {
  if (total <= 0) return 0
  return Math.min(100, Math.max(0, Math.round(((total - free) / total) * 100)))
}

function diskUsedBytes(total: number, free: number): number {
  return Math.max(0, total - free)
}
</script>

<template>
  <div class="relative flex h-full w-full flex-col overflow-hidden bg-[#0b3b6e]">
    <DsmWallpaper :theme="wallpaperTheme" />

    <DsmTaskbar
      :wallpaper-theme="wallpaperTheme"
      :widget-open="widgetOpen"
      @launch-app="handleLaunchApp"
      @toggle-widget="handleToggleWidget"
      @change-wallpaper="handleChangeWallpaper"
    />

    <main class="relative flex-1 overflow-hidden">
      <div class="grid w-[112px] grid-cols-1 gap-2 p-3">
        <button
          v-for="app in DESKTOP_APPS.filter((a) => a.showOnDesktop)"
          :key="app.id"
          type="button"
          class="flex cursor-pointer flex-col items-center gap-1.5 rounded-[6px] px-1 py-2 transition-colors hover:bg-white/14 active:bg-white/22"
          @click="handleLaunchApp(app)"
        >
          <div class="relative">
            <DsmAppIcon :app-id="app.id" size="lg" />
            <div
              v-if="app.requiresPrivate && authStore.accessTier === 'public'"
              class="absolute -right-1 -bottom-1 flex h-4.5 w-4.5 items-center justify-center rounded-full border border-white/25 bg-[#1e293b] text-amber-300 shadow-sm"
              title="Tailscale 내부망 전용"
            >
              <Lock class="h-2.5 w-2.5" />
            </div>
          </div>
          <span class="dsm-icon-label break-keep line-clamp-2 text-center text-[12px] leading-tight font-medium text-white">
            {{ app.title }}
          </span>
        </button>
      </div>

      <aside
        v-if="summary && widgetOpen"
        class="pointer-events-auto absolute top-3.5 right-3.5 hidden w-[272px] overflow-hidden rounded-[8px] border border-white/14 bg-[#141e2b]/72 text-white shadow-xl backdrop-blur-md md:block"
      >
        <div class="border-b border-white/10 px-3.5 py-3">
          <div class="text-[12px] font-semibold text-white/95">시스템 상태</div>
          <div class="mt-2 flex items-center gap-2">
            <svg
              viewBox="0 0 16 16"
              class="h-4 w-4 shrink-0 text-[#34d399]"
              fill="none"
              stroke="currentColor"
              stroke-width="1.8"
              stroke-linecap="round"
              stroke-linejoin="round"
            >
              <circle cx="8" cy="8" r="6" />
              <path d="M5.5 8.2l1.8 1.8 3.4-3.6" />
            </svg>
            <span class="text-[12.5px] font-semibold text-[#34d399]">정상 작동 중</span>
          </div>
          <p class="mt-1 text-[11px] text-white/75">시스템이 정상입니다.</p>
          <div class="mt-1.5 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-[11px] text-white/75">
            <span>가동 {{ formatDuration(summary.uptimeSeconds) }}</span>
            <span>·</span>
            <span>{{ summary.goOS }} {{ summary.cpuCores }}코어</span>
            <span>·</span>
            <span>메모리 {{ formatBytes(summary.appMemoryBytes) }}</span>
          </div>
        </div>

        <div class="px-3.5 py-3">
          <div class="mb-2.5 text-[12px] font-semibold text-white/95">스토리지</div>
          <div class="space-y-2.5">
            <div v-for="disk in summary.disks" :key="disk.id">
              <div class="flex items-center justify-between gap-2">
                <span class="truncate text-[11.5px] font-medium text-white/90">{{ disk.name }}</span>
                <span class="shrink-0 text-[11px] tabular-nums text-white/75">
                  {{ diskUsagePercent(disk.totalBytes, disk.freeBytes) }}%
                </span>
              </div>
              <div class="mt-0.5 mb-1 text-[10.5px] tabular-nums text-white/65">
                {{ formatBytes(diskUsedBytes(disk.totalBytes, disk.freeBytes)) }} / {{ formatBytes(disk.totalBytes) }}
              </div>
              <div class="h-1.5 w-full overflow-hidden rounded-full bg-white/15">
                <div
                  class="h-full rounded-full transition-all"
                  :class="
                    diskUsagePercent(disk.totalBytes, disk.freeBytes) > 85 ? 'bg-[#f59e0b]' : 'bg-[#2b88ff]'
                  "
                  :style="{ width: `${diskUsagePercent(disk.totalBytes, disk.freeBytes)}%` }"
                />
              </div>
            </div>
          </div>
        </div>
      </aside>

      <DsmWindow
        v-for="win in desktopStore.windows"
        :key="win.id"
        :win="win"
        :is-active="desktopStore.activeWindowId === win.id"
      >
        <FileStationApp
          v-if="win.payload.kind === 'file-station'"
          :window-id="win.id"
          :initial-root-id="win.payload.initialRootId"
          :initial-path="win.payload.initialPath"
        />
        <VideoPlayerApp
          v-else-if="win.payload.kind === 'video-player'"
          :window-id="win.id"
          :root-id="win.payload.rootId"
          :rel-path="win.payload.relPath"
          :file-name="win.payload.fileName"
        />
        <CodeEditorApp
          v-else-if="win.payload.kind === 'code-editor'"
          :window-id="win.id"
          :root-id="win.payload.rootId"
          :rel-path="win.payload.relPath"
          :file-name="win.payload.fileName"
        />
        <MountManagerApp v-else-if="win.payload.kind === 'mounts'" />
        <MonitorApp v-else-if="win.payload.kind === 'monitor'" />
        <WargameRunnerApp
          v-else-if="win.payload.kind === 'wargame'"
          :launch-source="win.payload.launchSource"
        />
        <PhasePreviewApp v-else-if="win.payload.kind === 'terminal'" />
      </DsmWindow>

      <div
        v-if="toastMessage"
        class="fixed bottom-5 left-1/2 z-50 -translate-x-1/2 rounded-[6px] border border-white/15 bg-[#1e293b] px-3.5 py-2 text-[12px] text-white shadow-lg"
      >
        {{ toastMessage }}
      </div>
    </main>
  </div>
</template>
