<script setup lang="ts">
import { ref } from 'vue'
import { useAuthStore } from '../stores/auth'
import DsmWallpaper from './desktop/DsmWallpaper.vue'

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
  <div class="relative flex h-full w-full items-center justify-center overflow-hidden bg-[#0b3b6e] p-4">
    <DsmWallpaper theme="ocean" />

    <div
      class="relative z-10 w-[360px] max-w-full rounded-[8px] border border-white/60 bg-white/95 p-8 text-slate-800 shadow-[0_20px_50px_rgba(0,0,0,0.35)]"
    >
      <div class="mb-6">
        <h1 class="text-[20px] leading-tight font-semibold tracking-tight text-[#1e293b]">Ubuntu-DSM</h1>
        <p class="mt-1 text-[12px] text-[#64748b]">계정에 로그인</p>
      </div>

      <form class="space-y-4" @submit.prevent="handleSubmit">
        <div>
          <label class="mb-1.5 block text-[12px] font-medium text-[#475569]">계정</label>
          <input
            v-model="username"
            type="text"
            autocomplete="username"
            class="dsm-input !h-9 w-full !text-[13px]"
            placeholder="사용자 아이디"
          />
        </div>

        <div>
          <label class="mb-1.5 block text-[12px] font-medium text-[#475569]">비밀번호</label>
          <input
            v-model="password"
            type="password"
            autocomplete="current-password"
            class="dsm-input !h-9 w-full !text-[13px]"
            placeholder="비밀번호"
          />
        </div>

        <div
          v-if="errorMessage"
          class="rounded-[4px] border border-[#f5c2c7] bg-[#fff5f5] px-3 py-2 text-[12px] font-medium text-[#c81e3a]"
        >
          {{ errorMessage }}
        </div>

        <button
          type="submit"
          :disabled="submitting"
          class="flex h-9 w-full cursor-pointer items-center justify-center gap-2 rounded-[4px] bg-[#0572ec] px-4 text-[13px] font-medium text-white transition-colors hover:bg-[#005ec4] disabled:cursor-not-allowed disabled:opacity-55"
        >
          <span>{{ submitting ? '로그인 중...' : '로그인' }}</span>
          <svg
            v-if="!submitting"
            viewBox="0 0 16 16"
            class="h-3.5 w-3.5 shrink-0"
            fill="none"
            stroke="currentColor"
            stroke-width="1.8"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path d="M3.5 8h9M8.5 4l4 4-4 4" />
          </svg>
        </button>
      </form>
    </div>

    <div class="pointer-events-none absolute bottom-5 left-6 z-10 select-none text-[12px] text-white/80">
      <div class="font-semibold tracking-tight text-white/90">Personal NAS · DSM 7 Style</div>
      <div class="mt-0.5 flex items-center gap-1.5 text-white/80">
        <span
          class="inline-block h-2 w-2 rounded-full"
          :class="authStore.accessTier === 'private' ? 'bg-[#10b981]' : 'bg-[#60a5fa]'"
        />
        <span>{{ authStore.accessTier === 'private' ? 'Tailscale 내부 관리망' : '외부 접속' }}</span>
      </div>
    </div>
  </div>
</template>
