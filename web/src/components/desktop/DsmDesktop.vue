<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue'
import { FolderOpen, Cpu, Terminal, Activity, Lock, HardDrive } from 'lucide-vue-next'
import { useAuthStore } from '../../stores/auth'
import { DESKTOP_APPS, useDesktopStore, type DesktopAppDefinition } from '../../stores/desktop'
import { useMountsStore } from '../../stores/mounts'
import { api } from '../../api/client'
import type { SystemSummary } from '../../types/api'
import { formatBytes, formatDuration } from '../../utils/format'
import DsmTaskbar from './DsmTaskbar.vue'
import DsmWindow from './DsmWindow.vue'
import FileStationApp from '../../apps/file-station/FileStationApp.vue'
import VideoPlayerApp from '../../apps/video-player/VideoPlayerApp.vue'
import CodeEditorApp from '../../apps/code-editor/CodeEditorApp.vue'
import MountManagerApp from '../../apps/mount-manager/MountManagerApp.vue'
import MonitorApp from '../../apps/monitor/MonitorApp.vue'
import WargameRunnerApp from '../../apps/wargame/WargameRunnerApp.vue'
import PhasePreviewApp from '../../apps/placeholders/PhasePreviewApp.vue'

const authStore = useAuthStore()
const desktopStore = useDesktopStore()
const mountsStore = useMountsStore()

const summary = ref<SystemSummary | null>(null)
const toastMessage = ref('')
let summaryTimer: ReturnType<typeof setInterval> | null = null
let toastTimer: ReturnType<typeof setTimeout> | null = null

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

  if (desktopStore.windows.length === 0) {
    desktopStore.openWindow({
      title: 'File Station',
      payload: { kind: 'file-station' },
    })
  }
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
    showToast(`${app.title}는 Tailscale 내부망 리스너(Private 포트)로 접속했을 때만 열 수 있습니다.`)
    return
  }
  switch (app.id) {
    case 'file-station':
      desktopStore.openWindow({ title: 'File Station', payload: { kind: 'file-station' } })
      return
    case 'mounts':
      desktopStore.openWindow({ title: '마운트 관리', payload: { kind: 'mounts' } })
      return
    case 'wargame':
      desktopStore.openWindow({ title: 'Wargame Runner', payload: { kind: 'wargame' } })
      return
    case 'terminal':
      desktopStore.openWindow({ title: 'Web Terminal', payload: { kind: 'terminal' } })
      return
    case 'monitor':
      desktopStore.openWindow({ title: 'Resource Monitor', payload: { kind: 'monitor' } })
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

function desktopIconComponent(id: DesktopAppDefinition['id']) {
  switch (id) {
    case 'file-station':
      return FolderOpen
    case 'mounts':
      return HardDrive
    case 'wargame':
      return Cpu
    case 'terminal':
      return Terminal
    case 'monitor':
      return Activity
    default:
      return FolderOpen
  }
}

function diskUsagePercent(total: number, free: number): number {
  if (total <= 0) return 0
  return Math.min(100, Math.max(0, Math.round(((total - free) / total) * 100)))
}
</script>

<template>
  <div
    class="relative flex h-full w-full flex-col overflow-hidden bg-gradient-to-br from-slate-950 via-sky-950/80 to-indigo-950"
  >
    <!-- Ambient DSM Wallpaper Shapes -->
    <div class="pointer-events-none absolute -top-24 left-1/4 h-[28rem] w-[28rem] rounded-full bg-sky-500/15 blur-3xl" />
    <div class="pointer-events-none absolute right-12 bottom-8 h-[24rem] w-[24rem] rounded-full bg-blue-600/15 blur-3xl" />

    <!-- Top Taskbar -->
    <DsmTaskbar @launch-app="handleLaunchApp" />

    <!-- Desktop Surface -->
    <main class="relative flex-1 overflow-hidden p-5">
      <!-- Left Desktop Icons -->
      <div class="grid w-24 grid-cols-1 gap-4">
        <button
          v-for="app in DESKTOP_APPS.filter((a) => a.showOnDesktop)"
          :key="app.id"
          type="button"
          class="group flex flex-col items-center gap-1.5 rounded-xl p-2 text-center transition hover:bg-white/10"
          @click="handleLaunchApp(app)"
        >
          <div
            class="relative flex h-13 w-13 items-center justify-center rounded-2xl bg-gradient-to-br text-white shadow-lg transition group-hover:scale-105"
            :class="app.iconBg"
          >
            <component :is="desktopIconComponent(app.id)" class="h-6 w-6" />
            <div
              v-if="app.requiresPrivate && authStore.accessTier === 'public'"
              class="absolute -right-1 -bottom-1 flex h-5 w-5 items-center justify-center rounded-full border border-white/20 bg-slate-900 text-amber-300"
              title="Tailscale 내부망 전용"
            >
              <Lock class="h-3 w-3" />
            </div>
          </div>
          <span class="line-clamp-2 text-xs font-medium text-white drop-shadow">{{ app.title }}</span>
        </button>
      </div>

      <!-- Right DSM System Status Widget -->
      <aside
        v-if="summary"
        class="pointer-events-auto absolute top-5 right-5 hidden w-64 rounded-2xl border border-white/10 bg-slate-950/60 p-4 backdrop-blur-xl lg:block"
      >
        <div class="mb-2.5 flex items-center justify-between">
          <span class="text-xs font-semibold text-slate-200">시스템 상태</span>
          <span class="text-[11px] text-slate-400">가동 {{ formatDuration(summary.uptimeSeconds) }}</span>
        </div>

        <div class="mb-3 grid grid-cols-2 gap-2 text-[11px]">
          <div class="rounded-lg bg-white/5 p-2">
            <div class="text-slate-400">플랫폼</div>
            <div class="mt-0.5 font-medium text-slate-200">{{ summary.goOS }} / {{ summary.cpuCores }}코어</div>
          </div>
          <div class="rounded-lg bg-white/5 p-2">
            <div class="text-slate-400">서버 메모리</div>
            <div class="mt-0.5 font-medium text-slate-200">{{ formatBytes(summary.appMemoryBytes) }}</div>
          </div>
        </div>

        <div class="space-y-2.5">
          <div v-for="disk in summary.disks" :key="disk.id" class="text-[11px]">
            <div class="mb-1 flex items-center justify-between text-slate-300">
              <span class="flex items-center gap-1 truncate">
                <HardDrive class="h-3 w-3 text-sky-400" />
                {{ disk.name }}
              </span>
              <span class="text-slate-400">{{ diskUsagePercent(disk.totalBytes, disk.freeBytes) }}%</span>
            </div>
            <div class="h-1.5 w-full overflow-hidden rounded-full bg-white/10">
              <div
                class="h-full rounded-full bg-sky-400"
                :style="{ width: `${diskUsagePercent(disk.totalBytes, disk.freeBytes)}%` }"
              />
            </div>
          </div>
        </div>
      </aside>

      <!-- Floating Windows -->
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

      <!-- Notification Toast -->
      <div
        v-if="toastMessage"
        class="fixed bottom-5 left-1/2 z-50 -translate-x-1/2 rounded-xl border border-amber-400/30 bg-slate-900/95 px-4 py-2.5 text-xs font-medium text-amber-200 shadow-2xl backdrop-blur"
      >
        {{ toastMessage }}
      </div>
    </main>
  </div>
</template>
