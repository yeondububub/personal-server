<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { Check, KeyRound, Lock, LogOut, X } from 'lucide-vue-next'
import { useAuthStore } from '../../stores/auth'
import { DESKTOP_APPS, useDesktopStore, type DesktopAppDefinition } from '../../stores/desktop'
import { api } from '../../api/client'
import DsmAppIcon from './DsmAppIcon.vue'
import type { DsmWallpaperTheme } from './DsmWallpaper.vue'

const props = defineProps<{
  readonly wallpaperTheme: DsmWallpaperTheme
  readonly widgetOpen: boolean
}>()

const emit = defineEmits<{
  launchApp: [app: DesktopAppDefinition]
  toggleWidget: []
  changeWallpaper: [theme: DsmWallpaperTheme]
}>()

const WALLPAPER_OPTIONS: readonly { readonly id: DsmWallpaperTheme; readonly label: string }[] = [
  { id: 'ocean', label: '오션 블루' },
  { id: 'mist', label: '노르딕 미스트' },
  { id: 'slate', label: '딥 슬레이트' },
]

const authStore = useAuthStore()
const desktopStore = useDesktopStore()

const launcherOpen = ref(false)
const searchQuery = ref('')
const userMenuOpen = ref(false)
const pwModalOpen = ref(false)
const currentPassword = ref('')
const newPassword = ref('')
const pwStatus = ref('')
const clockText = ref('')

let clockTimer: ReturnType<typeof setInterval> | null = null

const filteredApps = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  const base = DESKTOP_APPS.filter((a) => a.showOnDesktop)
  if (!q) return base
  return base.filter(
    (app) =>
      app.title.toLowerCase().includes(q) ||
      app.subtitle.toLowerCase().includes(q),
  )
})

function updateClock(): void {
  clockText.value = new Date().toLocaleTimeString('ko-KR', {
    hour: '2-digit',
    minute: '2-digit',
  })
}

function closeLauncher(): void {
  launcherOpen.value = false
  searchQuery.value = ''
}

function toggleLauncher(): void {
  userMenuOpen.value = false
  if (launcherOpen.value) {
    closeLauncher()
  } else {
    launcherOpen.value = true
  }
}

function handleKeydown(ev: KeyboardEvent): void {
  if (ev.key !== 'Escape') return
  if (pwModalOpen.value) {
    pwModalOpen.value = false
    return
  }
  if (launcherOpen.value) {
    closeLauncher()
    return
  }
  if (userMenuOpen.value) {
    userMenuOpen.value = false
  }
}

onMounted(() => {
  updateClock()
  clockTimer = setInterval(updateClock, 15000)
  window.addEventListener('keydown', handleKeydown)
})

onUnmounted(() => {
  if (clockTimer) clearInterval(clockTimer)
  window.removeEventListener('keydown', handleKeydown)
})

function handleSelectApp(app: DesktopAppDefinition): void {
  closeLauncher()
  emit('launchApp', app)
}

function handleSelectWallpaper(theme: DsmWallpaperTheme): void {
  emit('changeWallpaper', theme)
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
</script>

<template>
  <div class="relative z-[100] shrink-0">
    <header
      class="relative z-50 flex h-9 w-full select-none items-center justify-between border-b border-white/12 bg-[#121c29]/68 px-2.5 text-white backdrop-blur-md"
    >
      <div class="flex min-w-0 flex-1 items-center gap-2">
        <button
          type="button"
          class="flex h-7 w-7 shrink-0 cursor-pointer items-center justify-center rounded-[4px] text-white/90 transition-colors hover:bg-white/15"
          :class="{ 'bg-white/22 text-white': launcherOpen }"
          title="메인 메뉴"
          @click="toggleLauncher"
        >
          <svg viewBox="0 0 16 16" class="h-4 w-4" fill="currentColor">
            <rect x="2" y="2" width="5" height="5" rx="1" />
            <rect x="9" y="2" width="5" height="5" rx="1" />
            <rect x="2" y="9" width="5" height="5" rx="1" />
            <rect x="9" y="9" width="5" height="5" rx="1" />
          </svg>
        </button>

        <div class="h-4 w-px shrink-0 bg-white/15" />

        <div class="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto pr-2">
          <button
            v-for="win in desktopStore.windows"
            :key="win.id"
            type="button"
            class="relative flex h-7 max-w-[176px] shrink-0 cursor-pointer items-center gap-1.5 rounded-[4px] px-2 text-[12px] transition-colors"
            :class="
              desktopStore.activeWindowId === win.id && !win.minimized
                ? 'bg-white/22 font-medium text-white shadow-xs'
                : 'text-white/80 hover:bg-white/12'
            "
            @click="closeLauncher(); userMenuOpen = false; desktopStore.toggleMinimize(win.id)"
          >
            <DsmAppIcon :app-id="win.appId" size="xs" />
            <span class="truncate">{{ win.title }}</span>
            <span
              v-if="desktopStore.activeWindowId === win.id && !win.minimized"
              class="pointer-events-none absolute right-1.5 bottom-0 left-1.5 h-[2px] rounded-full bg-[#38bdf8]"
            />
          </button>
        </div>
      </div>

      <div class="flex shrink-0 items-center gap-1.5">
        <span class="hidden items-center gap-1.5 px-1.5 text-[11px] text-white/85 sm:inline-flex">
          <span
            class="h-2 w-2 rounded-full"
            :class="authStore.accessTier === 'private' ? 'bg-[#10b981]' : 'bg-[#60a5fa]'"
          />
          <span>{{ authStore.accessTier === 'private' ? 'Tailscale' : '외부 접속' }}</span>
        </span>

        <button
          type="button"
          class="flex h-7 w-7 cursor-pointer items-center justify-center rounded-[4px] transition-colors"
          :class="props.widgetOpen ? 'bg-white/22 text-white' : 'text-white/80 hover:bg-white/15'"
          title="시스템 위젯 표시 전환"
          @click="emit('toggleWidget')"
        >
          <svg
            viewBox="0 0 16 16"
            class="h-4 w-4"
            fill="none"
            stroke="currentColor"
            stroke-width="1.5"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <rect x="2" y="2.5" width="12" height="11" rx="1.8" />
            <path d="M10 2.5v11" />
          </svg>
        </button>

        <button
          type="button"
          class="flex h-7 cursor-pointer items-center gap-1.5 rounded-[4px] px-2 text-[12px] text-white/90 transition-colors hover:bg-white/15"
          :class="{ 'bg-white/20 text-white': userMenuOpen }"
          @click="userMenuOpen = !userMenuOpen; closeLauncher()"
        >
          <svg
            viewBox="0 0 16 16"
            class="h-3.5 w-3.5 shrink-0"
            fill="none"
            stroke="currentColor"
            stroke-width="1.6"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <circle cx="8" cy="5.5" r="2.5" />
            <path d="M3.2 13.5c.6-2.2 2.4-3.5 4.8-3.5s4.2 1.3 4.8 3.5" />
          </svg>
          <span>{{ authStore.username }}</span>
        </button>

        <span class="pl-1 text-[12px] tabular-nums text-white/90">{{ clockText }}</span>
      </div>
    </header>

    <div
      v-if="userMenuOpen"
      class="fixed inset-0 z-30"
      @click="userMenuOpen = false"
    />

    <div
      v-if="userMenuOpen"
      class="absolute top-10 right-2.5 z-40 w-52 rounded-[6px] border border-slate-200 bg-white py-1 text-[12px] text-slate-800 shadow-xl"
    >
      <div class="px-3 py-1.5 text-[11px] font-semibold text-[#64748b]">배경화면</div>
      <button
        v-for="opt in WALLPAPER_OPTIONS"
        :key="opt.id"
        type="button"
        class="flex w-full cursor-pointer items-center justify-between px-3 py-1.5 text-left text-[#1e293b] hover:bg-[#f1f5f9]"
        @click="handleSelectWallpaper(opt.id)"
      >
        <span>{{ opt.label }}</span>
        <Check v-if="props.wallpaperTheme === opt.id" class="h-3.5 w-3.5 text-[#0572ec]" />
      </button>

      <div class="my-1 border-t border-slate-200" />

      <button
        type="button"
        class="flex w-full cursor-pointer items-center gap-2 px-3 py-1.5 text-left text-[#1e293b] hover:bg-[#f1f5f9]"
        @click="pwModalOpen = true; userMenuOpen = false; pwStatus = ''"
      >
        <KeyRound class="h-3.5 w-3.5 text-[#64748b]" />
        <span>비밀번호 변경...</span>
      </button>

      <div class="my-1 border-t border-slate-200" />

      <button
        type="button"
        class="flex w-full cursor-pointer items-center gap-2 px-3 py-1.5 text-left text-[#c81e3a] hover:bg-[#fff1f3]"
        @click="handleLogout"
      >
        <LogOut class="h-3.5 w-3.5" />
        <span>로그아웃</span>
      </button>
    </div>

    <div
      v-if="launcherOpen"
      class="fixed inset-0 top-9 z-40 flex flex-col items-center bg-[#0d1826]/78 px-6 pt-10 backdrop-blur-xl"
      @click.self="closeLauncher"
    >
      <input
        v-model="searchQuery"
        type="text"
        autofocus
        placeholder="검색"
        class="h-9 w-full max-w-md rounded-[6px] border border-white/20 bg-white/14 px-3 text-[13px] text-white placeholder-white/60 focus:border-white/45 focus:bg-white/18 focus:outline-none"
      />

      <div
        class="mt-10 grid w-full max-w-3xl grid-cols-3 gap-6 sm:grid-cols-5"
        @click.self="closeLauncher"
      >
        <button
          v-for="app in filteredApps"
          :key="app.id"
          type="button"
          class="flex cursor-pointer flex-col items-center gap-2 rounded-[8px] px-2 py-3 text-center transition-colors hover:bg-white/12 active:bg-white/20"
          @click="handleSelectApp(app)"
        >
          <div class="relative">
            <DsmAppIcon :app-id="app.id" size="lg" />
            <span
              v-if="app.requiresPrivate && authStore.accessTier === 'public'"
              class="absolute -right-1 -bottom-1 flex h-4.5 w-4.5 items-center justify-center rounded-full border border-white/25 bg-[#1e293b] text-amber-300 shadow-sm"
              title="Tailscale 내부망 전용"
            >
              <Lock class="h-2.5 w-2.5" />
            </span>
          </div>
          <div class="min-w-0">
            <div class="break-keep text-[12px] leading-snug font-medium text-white">{{ app.title }}</div>
            <div class="mt-0.5 break-keep line-clamp-1 text-[11px] text-white/65">{{ app.subtitle }}</div>
          </div>
        </button>
      </div>
    </div>

    <div
      v-if="pwModalOpen"
      class="fixed inset-0 z-[60] flex items-center justify-center bg-black/45 p-4"
      @click.self="pwModalOpen = false"
    >
      <form
        class="w-full max-w-sm overflow-hidden rounded-[6px] border border-[#cbd5e1] bg-white text-[#1e293b] shadow-xl"
        @submit.prevent="handlePasswordSave"
      >
        <div class="flex items-center justify-between border-b border-[#dce2ea] bg-[#f4f6f9] px-4 py-2.5">
          <h3 class="text-[13px] font-semibold text-[#1e293b]">비밀번호 변경</h3>
          <button
            type="button"
            class="flex h-6 w-6 cursor-pointer items-center justify-center rounded-[4px] text-[#64748b] hover:bg-black/6 hover:text-[#1e293b]"
            @click="pwModalOpen = false"
          >
            <X class="h-3.5 w-3.5" />
          </button>
        </div>

        <div class="space-y-3 p-4">
          <div>
            <label class="mb-1 block text-[12px] font-medium text-[#475569]">현재 비밀번호</label>
            <input
              v-model="currentPassword"
              type="password"
              placeholder="현재 비밀번호 입력"
              class="dsm-input w-full"
            />
          </div>
          <div>
            <label class="mb-1 block text-[12px] font-medium text-[#475569]">새 비밀번호</label>
            <input
              v-model="newPassword"
              type="password"
              placeholder="새 비밀번호 (8자 이상)"
              class="dsm-input w-full"
            />
          </div>
          <p v-if="pwStatus" class="text-[12px] font-medium text-[#c81e3a]">{{ pwStatus }}</p>
        </div>

        <div class="flex items-center justify-end gap-2 border-t border-[#dce2ea] bg-[#f8fafc] px-4 py-2.5">
          <button
            type="button"
            class="dsm-btn"
            @click="pwModalOpen = false"
          >
            취소
          </button>
          <button
            type="submit"
            class="dsm-btn dsm-btn-primary"
          >
            확인
          </button>
        </div>
      </form>
    </div>
  </div>
</template>
