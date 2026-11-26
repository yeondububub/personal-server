<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Download, Film, FolderArchive, Lock, AlertCircle } from 'lucide-vue-next'
import { api } from '../api/client'
import type { PublicShareState } from '../types/api'
import { formatBytes, formatDateTime } from '../utils/format'

const props = defineProps<{
  readonly token: string
}>()

type ViewState =
  | { readonly kind: 'loading' }
  | { readonly kind: 'error'; readonly message: string }
  | { readonly kind: 'ready'; readonly share: PublicShareState }

const view = ref<ViewState>({ kind: 'loading' })
const passwordInput = ref('')
const unlockError = ref('')
const unlocking = ref(false)

async function loadShare(): Promise<void> {
  view.value = { kind: 'loading' }
  try {
    const share = await api.fetchPublicShare(props.token)
    view.value = { kind: 'ready', share }
  } catch (err) {
    view.value = {
      kind: 'error',
      message: err instanceof Error ? err.message : '공유 링크를 불러올 수 없습니다.',
    }
  }
}

async function handleUnlock(): Promise<void> {
  if (!passwordInput.value) {
    unlockError.value = '비밀번호를 입력하세요.'
    return
  }
  unlocking.value = true
  unlockError.value = ''
  try {
    await api.unlockPublicShare({ token: props.token, password: passwordInput.value })
    passwordInput.value = ''
    await loadShare()
  } catch (err) {
    unlockError.value = err instanceof Error ? err.message : '비밀번호가 일치하지 않습니다.'
  } finally {
    unlocking.value = false
  }
}

onMounted(() => {
  void loadShare()
})
</script>

<template>
  <div class="flex min-h-full w-full items-center justify-center overflow-y-auto bg-slate-950 p-6 text-slate-100">
    <div v-if="view.kind === 'loading'" class="text-sm text-slate-400">공유 항목을 확인하는 중...</div>

    <div
      v-else-if="view.kind === 'error'"
      class="flex max-w-md flex-col items-center gap-3 rounded-2xl border border-rose-500/30 bg-slate-900 p-8 text-center"
    >
      <AlertCircle class="h-10 w-10 text-rose-400" />
      <h2 class="text-base font-semibold">공유 링크에 접근할 수 없습니다</h2>
      <p class="text-xs text-slate-400">{{ view.message }}</p>
    </div>

    <div
      v-else-if="view.share.kind === 'locked'"
      class="w-full max-w-sm rounded-2xl border border-white/10 bg-slate-900 p-6 shadow-xl"
    >
      <div class="mb-4 flex items-center gap-3">
        <div class="flex h-10 w-10 items-center justify-center rounded-xl bg-amber-500/20 text-amber-300">
          <Lock class="h-5 w-5" />
        </div>
        <div>
          <h2 class="text-sm font-semibold">비밀번호로 보호된 공유 링크</h2>
          <p class="text-xs text-slate-400">만료: {{ formatDateTime(view.share.expiresAt) }}</p>
        </div>
      </div>

      <form class="space-y-3" @submit.prevent="handleUnlock">
        <input
          v-model="passwordInput"
          type="password"
          placeholder="공유 비밀번호 입력"
          class="w-full rounded-xl border border-white/10 bg-slate-950 px-3.5 py-2.5 text-sm text-white focus:border-sky-400 focus:outline-none"
        />
        <p v-if="unlockError" class="text-xs text-rose-300">{{ unlockError }}</p>
        <button
          type="submit"
          :disabled="unlocking"
          class="w-full rounded-xl bg-sky-600 px-4 py-2.5 text-sm font-medium text-white hover:bg-sky-500 disabled:opacity-50"
        >
          {{ unlocking ? '확인 중...' : '잠금 해제' }}
        </button>
      </form>
    </div>

    <div
      v-else
      class="w-full max-w-4xl rounded-2xl border border-white/10 bg-slate-900 p-6 shadow-2xl"
    >
      <div class="mb-4 flex flex-wrap items-center justify-between gap-4 border-b border-white/10 pb-4">
        <div class="flex items-center gap-3">
          <div class="flex h-11 w-11 items-center justify-center rounded-xl bg-sky-500/20 text-sky-300">
            <Film v-if="view.share.fileKind === 'video'" class="h-6 w-6" />
            <FolderArchive v-else class="h-6 w-6" />
          </div>
          <div>
            <h1 class="text-base font-semibold text-white">{{ view.share.name }}</h1>
            <p class="text-xs text-slate-400">
              {{ view.share.isDir ? '폴더 (ZIP 압축 다운로드)' : formatBytes(view.share.size) }}
              · 만료 {{ formatDateTime(view.share.expiresAt) }}
            </p>
          </div>
        </div>

        <a
          v-if="view.share.allowDownload"
          :href="api.publicShareDownloadUrl(view.share.token)"
          class="inline-flex items-center gap-2 rounded-xl bg-sky-600 px-4 py-2 text-xs font-semibold text-white hover:bg-sky-500"
        >
          <Download class="h-4 w-4" />
          {{ view.share.isDir ? 'ZIP 다운로드' : '파일 다운로드' }}
        </a>
      </div>

      <div v-if="view.share.fileKind === 'video'" class="overflow-hidden rounded-xl bg-black">
        <video
          controls
          playsinline
          preload="metadata"
          class="aspect-video max-h-[70vh] w-full bg-black"
          :src="api.publicShareStreamUrl(view.share.token)"
          crossorigin="use-credentials"
        >
          <track
            v-for="(sub, idx) in view.share.subtitles"
            :key="sub.relPath"
            kind="subtitles"
            :label="sub.name"
            :srclang="'ko'"
            :src="api.publicShareSubtitleUrl({ token: view.share.token, relPath: sub.relPath })"
            :default="idx === 0"
          />
        </video>
      </div>
      <div v-else class="py-10 text-center text-sm text-slate-400">
        상단의 다운로드 버튼을 눌러 파일을 저장하세요.
      </div>
    </div>
  </div>
</template>
