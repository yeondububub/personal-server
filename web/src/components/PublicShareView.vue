<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Download, Lock, AlertCircle } from 'lucide-vue-next'
import { api } from '../api/client'
import type { PublicShareState } from '../types/api'
import { formatBytes, formatDateTime } from '../utils/format'
import DsmAppIcon from './desktop/DsmAppIcon.vue'
import DsmWallpaper from './desktop/DsmWallpaper.vue'

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
  <div class="relative flex min-h-full w-full items-center justify-center overflow-y-auto bg-[#0b3b6e] p-6">
    <DsmWallpaper theme="ocean" />

    <div
      v-if="view.kind === 'loading'"
      class="relative z-10 rounded-[8px] border border-slate-200/80 bg-white px-6 py-4 text-[13px] text-slate-600 shadow-2xl"
    >
      공유 항목을 확인하는 중...
    </div>

    <div
      v-else-if="view.kind === 'error'"
      class="relative z-10 flex w-full max-w-md flex-col items-center gap-2.5 rounded-[8px] border border-slate-200/80 bg-white p-8 text-center text-slate-800 shadow-2xl"
    >
      <AlertCircle class="h-9 w-9 text-[#d92d4b]" />
      <h2 class="text-[15px] font-semibold text-[#1e293b]">공유 링크에 접근할 수 없습니다</h2>
      <p class="text-[12px] text-[#64748b]">{{ view.message }}</p>
    </div>

    <div
      v-else-if="view.share.kind === 'locked'"
      class="relative z-10 w-full max-w-sm rounded-[8px] border border-slate-200/80 bg-white p-6 text-slate-800 shadow-2xl"
    >
      <div class="mb-5 flex items-center gap-3">
        <div class="relative shrink-0">
          <DsmAppIcon app-id="file-station" size="md" />
          <span
            class="absolute -right-1 -bottom-1 flex h-4.5 w-4.5 items-center justify-center rounded-full border border-white bg-[#1e293b] text-white"
          >
            <Lock class="h-2.5 w-2.5" />
          </span>
        </div>
        <div class="min-w-0">
          <h2 class="text-[14px] font-semibold text-[#1e293b]">비밀번호로 보호된 공유 링크</h2>
          <p class="mt-0.5 text-[11.5px] text-[#64748b]">만료: {{ formatDateTime(view.share.expiresAt) }}</p>
        </div>
      </div>

      <form class="space-y-3" @submit.prevent="handleUnlock">
        <input
          v-model="passwordInput"
          type="password"
          placeholder="공유 비밀번호 입력"
          class="dsm-input !h-9 w-full !text-[13px]"
        />
        <p v-if="unlockError" class="text-[12px] font-medium text-[#c81e3a]">{{ unlockError }}</p>
        <button
          type="submit"
          :disabled="unlocking"
          class="dsm-btn dsm-btn-primary !h-9 w-full !text-[13px]"
        >
          {{ unlocking ? '확인 중...' : '잠금 해제' }}
        </button>
      </form>
    </div>

    <div
      v-else
      class="relative z-10 w-full max-w-4xl rounded-[8px] border border-slate-200/80 bg-white p-6 text-slate-800 shadow-2xl"
    >
      <div class="mb-4 flex flex-wrap items-center justify-between gap-4 border-b border-[#dce2ea] pb-4">
        <div class="flex min-w-0 items-center gap-3">
          <DsmAppIcon :app-id="view.share.fileKind === 'video' ? 'video-player' : 'file-station'" size="md" />
          <div class="min-w-0">
            <h1 class="truncate text-[15px] font-semibold text-[#1e293b]">{{ view.share.name }}</h1>
            <p class="mt-0.5 text-[12px] text-[#64748b]">
              {{ view.share.isDir ? '폴더 (ZIP 압축 다운로드)' : formatBytes(view.share.size) }}
              · 만료 {{ formatDateTime(view.share.expiresAt) }}
            </p>
          </div>
        </div>

        <a
          v-if="view.share.allowDownload"
          :href="api.publicShareDownloadUrl(view.share.token)"
          class="dsm-btn dsm-btn-primary no-underline"
        >
          <Download class="h-3.5 w-3.5" />
          <span>다운로드</span>
        </a>
      </div>

      <div v-if="view.share.fileKind === 'video'" class="overflow-hidden rounded-[4px] border border-[#dce2ea] bg-[#000000]">
        <video
          controls
          playsinline
          preload="metadata"
          class="aspect-video max-h-[70vh] w-full bg-[#000000]"
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
      <div
        v-else
        class="rounded-[4px] border border-[#e2e8f0] bg-[#f8fafc] py-10 text-center text-[12.5px] text-[#64748b]"
      >
        상단의 다운로드 버튼을 눌러 파일을 저장하세요.
      </div>
    </div>

    <div class="pointer-events-none absolute bottom-4 left-6 z-10 select-none text-[11.5px] text-white/75">
      File Station 공유 · Personal NAS
    </div>
  </div>
</template>
