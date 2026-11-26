<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import {
  LayoutGrid,
  ShieldCheck,
  Globe,
  LogOut,
  KeyRound,
  FolderOpen,
  Film,
  FileCode2,
  HardDrive,
  Cpu,
  Terminal,
  Activity,
  Lock,
} from 'lucide-vue-next'
import { useAuthStore } from '../../stores/auth'
import { DESKTOP_APPS, useDesktopStore, type AppId, type DesktopAppDefinition } from '../../stores/desktop'
import { api } from '../../api/client'

const emit = defineEmits<{
  launchApp: [app: DesktopAppDefinition]
}>()

const authStore = useAuthStore()
const desktopStore = useDesktopStore()

const launcherOpen = ref(false)
const userMenuOpen = ref(false)
const pwModalOpen = ref(false)
const currentPassword = ref('')
const newPassword = ref('')
const pwStatus = ref('')
const clockText = ref('')

let clockTimer: ReturnType<typeof setInterval> | null = null

function updateClock(): void {
  clockText.value = new Date().toLocaleTimeString('ko-KR', {
    hour: '2-digit',
    minute: '2-digit',
  })
}

onMounted(() => {
  updateClock()
  clockTimer = setInterval(updateClock, 15000)
})

onUnmounted(() => {
  if (clockTimer) clearInterval(clockTimer)
})

function handleSelectApp(app: DesktopAppDefinition): void {
  launcherOpen.value = false
  emit('launchApp', app)
}

async function handleLogout(): Promise<void> {
  userMenuOpen.value = false
  desktopStore.resetAll()
  await authStore.logout()
}

async function handlePasswordSave(): Promise<void> {
  pwStatus.value = ''
  try {
    await api.changePassword({
      currentPassword: currentPassword.value,
      newPassword: newPassword.value,
    })
    currentPassword.value = ''
    newPassword.value = ''
    pwModalOpen.value = false
  } catch (err) {
    pwStatus.value = err instanceof Error ? err.message : '비밀번호 변경 실패'
  }
}

function iconNameFor(appId: AppId): typeof FolderOpen {
  switch (appId) {
    case 'file-station':
      return FolderOpen
    case 'video-player':
      return Film
    case 'code-editor':
      return FileCode2
    case 'mounts':
      return HardDrive
    case 'wargame':
      return Cpu
    case 'terminal':
      return Terminal
    case 'monitor':
      return Activity
    default: {
      const _exhaustive: never = appId
      return _exhaustive
    }
  }
}
</script>

<template>
  <header
    class="relative z-50 flex h-11 w-full select-none items-center justify-between border-b border-white/10 bg-slate-950/75 px-3 backdrop-blur-xl"
  >
    <!-- Left: App Launcher + Open Windows -->
    <div class="flex min-w-0 flex-1 items-center gap-2">
      <button
        type="button"
        class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-slate-200 transition hover:bg-white/10"
        title="메인 메뉴"
        @click="launcherOpen = !launcherOpen; userMenuOpen = false"
      >
        <LayoutGrid class="h-4 w-4" />
      </button>

      <div class="flex min-w-0 flex-1 items-center gap-1.5 overflow-x-auto pr-2">
        <button
          v-for="win in desktopStore.windows"
          :key="win.id"
          type="button"
          class="flex max-w-[180px] shrink-0 items-center gap-2 rounded-lg border px-2.5 py-1 text-xs transition"
          :class="
            desktopStore.activeWindowId === win.id && !win.minimized
              ? 'border-sky-400/40 bg-sky-500/20 text-white'
              : 'border-white/5 bg-white/5 text-slate-300 hover:bg-white/10'
          "
          @click="desktopStore.toggleMinimize(win.id)"
        >
          <component :is="iconNameFor(win.appId)" class="h-3.5 w-3.5 shrink-0 text-sky-300" />
          <span class="truncate">{{ win.title }}</span>
        </button>
      </div>
    </div>

    <!-- Right: Tier Indicator + Clock + User Menu -->
    <div class="flex shrink-0 items-center gap-2.5">
      <span
        v-if="authStore.accessTier === 'private'"
        class="hidden items-center gap-1 rounded-full border border-emerald-400/30 bg-emerald-500/15 px-2.5 py-0.5 text-[11px] font-medium text-emerald-300 sm:inline-flex"
      >
        <ShieldCheck class="h-3 w-3" />
        Tailscale Admin
      </span>
      <span
        v-else
        class="hidden items-center gap-1 rounded-full border border-sky-400/30 bg-sky-500/15 px-2.5 py-0.5 text-[11px] font-medium text-sky-300 sm:inline-flex"
      >
        <Globe class="h-3 w-3" />
        Public
      </span>

      <span class="text-xs font-medium tabular-nums text-slate-300">{{ clockText }}</span>

      <button
        type="button"
        class="rounded-lg bg-white/10 px-2.5 py-1 text-xs font-medium text-white hover:bg-white/15"
        @click="userMenuOpen = !userMenuOpen; launcherOpen = false"
      >
        {{ authStore.username }}
      </button>
    </div>

    <!-- App Launcher Popover -->
    <div
      v-if="launcherOpen"
      class="absolute top-12 left-3 z-50 w-80 rounded-2xl border border-white/15 bg-slate-900/95 p-4 shadow-2xl backdrop-blur-xl"
    >
      <div class="mb-3 text-xs font-semibold tracking-wide text-slate-400">애플리케이션</div>
      <div class="grid grid-cols-2 gap-2">
        <button
          v-for="app in DESKTOP_APPS.filter((a) => a.showOnDesktop)"
          :key="app.id"
          type="button"
          class="flex items-center gap-2.5 rounded-xl border border-white/5 bg-white/5 p-2.5 text-left transition hover:border-white/15 hover:bg-white/10"
          @click="handleSelectApp(app)"
        >
          <div
            class="relative flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-gradient-to-br text-white shadow"
            :class="app.iconBg"
          >
            <component :is="iconNameFor(app.id)" class="h-4 w-4" />
            <Lock
              v-if="app.requiresPrivate && authStore.accessTier === 'public'"
              class="absolute -right-1 -bottom-1 h-3.5 w-3.5 rounded-full bg-slate-900 p-0.5 text-amber-300"
            />
          </div>
          <div class="min-w-0">
            <div class="truncate text-xs font-semibold text-white">{{ app.title }}</div>
            <div class="truncate text-[10px] text-slate-400">{{ app.subtitle }}</div>
          </div>
        </button>
      </div>
    </div>

    <!-- User Menu Popover -->
    <div
      v-if="userMenuOpen"
      class="absolute top-12 right-3 z-50 w-48 rounded-xl border border-white/15 bg-slate-900/95 p-1.5 shadow-2xl backdrop-blur-xl"
    >
      <button
        type="button"
        class="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-xs text-slate-200 hover:bg-white/10"
        @click="pwModalOpen = true; userMenuOpen = false"
      >
        <KeyRound class="h-3.5 w-3.5 text-sky-300" />
        비밀번호 변경
      </button>
      <button
        type="button"
        class="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-xs text-rose-300 hover:bg-rose-500/15"
        @click="handleLogout"
      >
        <LogOut class="h-3.5 w-3.5" />
        로그아웃
      </button>
    </div>

    <!-- Password Change Modal -->
    <div
      v-if="pwModalOpen"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
      @click.self="pwModalOpen = false"
    >
      <div class="w-full max-w-sm rounded-2xl border border-white/15 bg-slate-900 p-5 shadow-2xl">
        <h3 class="mb-3 text-sm font-semibold text-white">관리자 비밀번호 변경</h3>
        <form class="space-y-3" @submit.prevent="handlePasswordSave">
          <input
            v-model="currentPassword"
            type="password"
            placeholder="현재 비밀번호"
            class="w-full rounded-xl border border-white/10 bg-slate-950 px-3 py-2 text-xs text-white focus:border-sky-400 focus:outline-none"
          />
          <input
            v-model="newPassword"
            type="password"
            placeholder="새 비밀번호 (8자 이상)"
            class="w-full rounded-xl border border-white/10 bg-slate-950 px-3 py-2 text-xs text-white focus:border-sky-400 focus:outline-none"
          />
          <p v-if="pwStatus" class="text-xs text-rose-300">{{ pwStatus }}</p>
          <div class="flex justify-end gap-2 pt-1">
            <button
              type="button"
              class="rounded-lg px-3 py-1.5 text-xs text-slate-400 hover:bg-white/5"
              @click="pwModalOpen = false"
            >
              취소
            </button>
            <button
              type="submit"
              class="rounded-lg bg-sky-600 px-3 py-1.5 text-xs font-semibold text-white hover:bg-sky-500"
            >
              변경 저장
            </button>
          </div>
        </form>
      </div>
    </div>
  </header>
</template>
