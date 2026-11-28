<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue'
import { Lock } from 'lucide-vue-next'
import { useAuthStore } from '../../stores/auth'
import { DESKTOP_APPS, useDesktopStore, type DesktopAppDefinition } from '../../stores/desktop'
import { useMountsStore } from '../../stores/mounts'
import { useSettingsStore } from '../../stores/settings'
import { api } from '../../api/client'
import type { MonitorSnapshot } from '../../types/api'
import { formatBytes, formatDuration } from '../../utils/format'
import DsmAppIcon from './DsmAppIcon.vue'
import DsmTaskbar from './DsmTaskbar.vue'
import DsmWallpaper from './DsmWallpaper.vue'
import DsmWindow from './DsmWindow.vue'
import FileStationApp from '../../apps/file-station/FileStationApp.vue'
import VideoPlayerApp from '../../apps/video-player/VideoPlayerApp.vue'
import CodeEditorApp from '../../apps/code-editor/CodeEditorApp.vue'
import MountManagerApp from '../../apps/mount-manager/MountManagerApp.vue'
import DockerManagerApp from '../../apps/docker-manager/DockerManagerApp.vue'
import MonitorApp from '../../apps/monitor/MonitorApp.vue'
import WargameRunnerApp from '../../apps/wargame/WargameRunnerApp.vue'
import WebTerminalApp from '../../apps/terminal/WebTerminalApp.vue'
import SettingsApp from '../../apps/settings/SettingsApp.vue'

const authStore = useAuthStore()
const desktopStore = useDesktopStore()
const mountsStore = useMountsStore()
const settingsStore = useSettingsStore()

const widgetOpen = ref(true)
const snapshot = ref<MonitorSnapshot | null>(null)
const toastMessage = ref('')
let snapshotTimer: ReturnType<typeof setInterval> | null = null
let toastTimer: ReturnType<typeof setTimeout> | null = null

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

async function refreshSnapshot(): Promise<void> {
  try {
    snapshot.value = await api.fetchMonitorSnapshot()
  } catch {
    return
  }
}

onMounted(() => {
  void refreshSnapshot()
  snapshotTimer = setInterval(() => {
    void refreshSnapshot()
  }, 4000)
})

watch(
  () => mountsStore.version,
  () => {
    void refreshSnapshot()
  },
)

onUnmounted(() => {
  if (snapshotTimer) clearInterval(snapshotTimer)
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
    case 'docker':
      desktopStore.openWindow({ title: app.title, payload: { kind: 'docker' } })
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
    case 'settings':
      desktopStore.openWindow({ title: app.title, payload: { kind: 'settings' } })
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

function clampPercent(value: number): number {
  if (!Number.isFinite(value)) return 0
  return Math.min(100, Math.max(0, value))
}

function metricBarClass(percent: number, defaultClass: string): string {
  if (percent >= 90) return 'bg-[#f43f5e]'
  if (percent >= 75) return 'bg-[#f59e0b]'
  return defaultClass
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
    <DsmWallpaper
      :theme="settingsStore.wallpaperPreset"
      :custom-url="settingsStore.wallpaperMode === 'custom' ? settingsStore.customWallpaperUrl : null"
    />

    <DsmTaskbar
      :widget-open="widgetOpen"
      @launch-app="handleLaunchApp"
      @toggle-widget="handleToggleWidget"
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
        v-if="snapshot && widgetOpen"
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
          <div class="mt-1.5 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-[11px] text-white/75">
            <span>가동 {{ formatDuration(snapshot.uptimeSeconds) }}</span>
            <span>·</span>
            <span>{{ snapshot.os }} {{ snapshot.cpuPerCore.length }}코어</span>
          </div>
        </div>

        <div class="space-y-2.5 border-b border-white/10 px-3.5 py-3">
          <div class="text-[12px] font-semibold text-white/95">리소스 모니터</div>

          <div>
            <div class="mb-1 flex items-center justify-between text-[11.5px]">
              <span class="font-medium text-white/90">CPU</span>
              <span class="tabular-nums text-white/80">
                {{ clampPercent(snapshot.cpuTotalPercent).toFixed(1) }}%
              </span>
            </div>
            <div class="h-1.5 w-full overflow-hidden rounded-full bg-white/15">
              <div
                class="h-full rounded-full transition-all duration-300"
                :class="metricBarClass(clampPercent(snapshot.cpuTotalPercent), 'bg-[#2b88ff]')"
                :style="{ width: `${clampPercent(snapshot.cpuTotalPercent)}%` }"
              />
            </div>
          </div>

          <div>
            <div class="mb-0.5 flex items-center justify-between text-[11.5px]">
              <span class="font-medium text-white/90">RAM</span>
              <span class="tabular-nums text-white/80">
                {{ clampPercent(snapshot.memUsedPercent).toFixed(1) }}%
              </span>
            </div>
            <div class="mb-1 text-[10.5px] tabular-nums text-white/65">
              {{ formatBytes(snapshot.memUsedBytes) }} / {{ formatBytes(snapshot.memTotalBytes) }}
            </div>
            <div class="h-1.5 w-full overflow-hidden rounded-full bg-white/15">
              <div
                class="h-full rounded-full transition-all duration-300"
                :class="metricBarClass(clampPercent(snapshot.memUsedPercent), 'bg-[#14b8a6]')"
                :style="{ width: `${clampPercent(snapshot.memUsedPercent)}%` }"
              />
            </div>
          </div>
        </div>

        <div class="px-3.5 py-3">
          <div class="mb-2.5 text-[12px] font-semibold text-white/95">스토리지</div>
          <div class="space-y-2.5">
            <div v-for="disk in snapshot.disks" :key="disk.id">
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
        <DockerManagerApp v-else-if="win.payload.kind === 'docker'" />
        <MonitorApp v-else-if="win.payload.kind === 'monitor'" />
        <WargameRunnerApp
          v-else-if="win.payload.kind === 'wargame'"
          :launch-source="win.payload.launchSource"
        />
        <WebTerminalApp
          v-else-if="win.payload.kind === 'terminal'"
          :initial-target="win.payload.initialTarget"
        />
        <SettingsApp v-else-if="win.payload.kind === 'settings'" />
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
