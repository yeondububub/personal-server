<script setup lang="ts">
import { ref } from 'vue'
import { Lock, Server, ShieldCheck, Globe } from 'lucide-vue-next'
import { useAuthStore } from '../stores/auth'

const authStore = useAuthStore()

const username = ref('admin')
const password = ref('')
const submitting = ref(false)
const errorMessage = ref('')

async function handleSubmit(): Promise<void> {
  if (!username.value.trim() || !password.value) {
    errorMessage.value = '아이디와 비밀번호를 입력하세요.'
    return
  }
  submitting.value = true
  errorMessage.value = ''
  try {
    await authStore.login({
      username: username.value.trim(),
      password: password.value,
    })
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '로그인에 실패했습니다.'
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div
    class="relative flex h-full w-full items-center justify-center overflow-hidden bg-gradient-to-br from-slate-950 via-slate-900 to-sky-950 p-4"
  >
    <!-- Ambient DSM background accents -->
    <div class="pointer-events-none absolute -top-32 -left-32 h-96 w-96 rounded-full bg-sky-500/20 blur-3xl" />
    <div class="pointer-events-none absolute -right-32 -bottom-32 h-96 w-96 rounded-full bg-indigo-500/20 blur-3xl" />

    <div
      class="relative z-10 w-full max-w-md rounded-2xl border border-white/15 bg-slate-900/75 p-8 shadow-2xl backdrop-blur-xl"
    >
      <div class="mb-6 flex items-center justify-between">
        <div class="flex items-center gap-3">
          <div class="flex h-11 w-11 items-center justify-center rounded-xl bg-gradient-to-br from-sky-500 to-blue-600 shadow-lg">
            <Server class="h-6 w-6 text-white" />
          </div>
          <div>
            <h1 class="text-lg font-semibold text-white">Personal DSM</h1>
            <p class="text-xs text-slate-400">Ubuntu Integrated Workspace</p>
          </div>
        </div>

        <span
          v-if="authStore.accessTier === 'private'"
          class="inline-flex items-center gap-1.5 rounded-full border border-emerald-400/30 bg-emerald-500/15 px-2.5 py-1 text-xs font-medium text-emerald-300"
        >
          <ShieldCheck class="h-3.5 w-3.5" />
          Tailscale 내부망
        </span>
        <span
          v-else
          class="inline-flex items-center gap-1.5 rounded-full border border-sky-400/30 bg-sky-500/15 px-2.5 py-1 text-xs font-medium text-sky-300"
        >
          <Globe class="h-3.5 w-3.5" />
          외부 공개망
        </span>
      </div>

      <form class="space-y-4" @submit.prevent="handleSubmit">
        <div>
          <label class="mb-1.5 block text-xs font-medium text-slate-300">관리자 계정</label>
          <input
            v-model="username"
            type="text"
            autocomplete="username"
            class="w-full rounded-xl border border-white/10 bg-slate-950/70 px-3.5 py-2.5 text-sm text-white placeholder-slate-500 focus:border-sky-400 focus:outline-none"
            placeholder="admin"
          />
        </div>

        <div>
          <label class="mb-1.5 block text-xs font-medium text-slate-300">비밀번호</label>
          <input
            v-model="password"
            type="password"
            autocomplete="current-password"
            class="w-full rounded-xl border border-white/10 bg-slate-950/70 px-3.5 py-2.5 text-sm text-white placeholder-slate-500 focus:border-sky-400 focus:outline-none"
            placeholder="비밀번호 입력"
          />
        </div>

        <div
          v-if="errorMessage"
          class="rounded-xl border border-rose-500/30 bg-rose-500/15 px-3.5 py-2.5 text-xs text-rose-200"
        >
          {{ errorMessage }}
        </div>

        <button
          type="submit"
          :disabled="submitting"
          class="flex w-full items-center justify-center gap-2 rounded-xl bg-gradient-to-r from-sky-500 to-blue-600 px-4 py-2.5 text-sm font-semibold text-white shadow-lg transition hover:from-sky-400 hover:to-blue-500 disabled:opacity-50"
        >
          <Lock class="h-4 w-4" />
          <span>{{ submitting ? '인증 중...' : '로그인' }}</span>
        </button>
      </form>
    </div>
  </div>
</template>
