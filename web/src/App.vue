<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useAuthStore } from './stores/auth'
import { useSettingsStore } from './stores/settings'
import LoginScreen from './components/LoginScreen.vue'
import PublicShareView from './components/PublicShareView.vue'
import DsmDesktop from './components/desktop/DsmDesktop.vue'

const authStore = useAuthStore()
const settingsStore = useSettingsStore()

const shareToken = computed<string | null>(() => {
  const path = window.location.pathname
  if (!path.startsWith('/s/')) return null
  const token = path.slice(3).split('/')[0] ?? ''
  return token.trim() !== '' ? decodeURIComponent(token) : null
})

onMounted(() => {
  void settingsStore.loadPublic()
  if (!shareToken.value) {
    void authStore.refreshSession()
  }
})
</script>

<template>
  <PublicShareView v-if="shareToken" :token="shareToken" />
  <div
    v-else-if="authStore.state.kind === 'loading'"
    class="flex h-full w-full items-center justify-center bg-slate-950 text-slate-300"
  >
    <div class="flex flex-col items-center gap-3">
      <div class="h-8 w-8 animate-spin rounded-full border-2 border-sky-400 border-t-transparent" />
      <span class="text-sm text-slate-400">DSM 환경을 불러오는 중...</span>
    </div>
  </div>
  <LoginScreen v-else-if="authStore.state.kind === 'unauthenticated'" />
  <DsmDesktop v-else />
</template>
