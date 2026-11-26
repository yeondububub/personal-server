<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { RotateCcw, Subtitles, Download } from 'lucide-vue-next'
import { api } from '../../api/client'
import type { MediaInfo } from '../../types/api'
import { formatBytes, formatDuration } from '../../utils/format'

const props = defineProps<{
  readonly windowId: string
  readonly rootId: string
  readonly relPath: string
  readonly fileName: string
}>()

const videoRef = ref<HTMLVideoElement | null>(null)
const mediaInfo = ref<MediaInfo | null>(null)
const resumeOfferSec = ref<number | null>(null)
const playbackRate = ref(1)
const errorMessage = ref('')

let saveTimer: ReturnType<typeof setInterval> | null = null

const streamUrl = computed(() =>
  api.mediaStreamUrl({ rootId: props.rootId, relPath: props.relPath }),
)

async function persistProgress(): Promise<void> {
  const el = videoRef.value
  if (!el || !Number.isFinite(el.currentTime) || el.currentTime < 1) return
  try {
    await api.saveMediaProgress({
      rootId: props.rootId,
      relPath: props.relPath,
      positionSec: el.currentTime,
      durationSec: Number.isFinite(el.duration) ? el.duration : 0,
    })
  } catch {
    // Ignore background progress save failure
  }
}

function applyResume(): void {
  const el = videoRef.value
  if (!el || resumeOfferSec.value === null) return
  el.currentTime = resumeOfferSec.value
  resumeOfferSec.value = null
  void el.play()
}

function updatePlaybackRate(): void {
  if (videoRef.value) {
    videoRef.value.playbackRate = playbackRate.value
  }
}

onMounted(async () => {
  try {
    const info = await api.fetchMediaInfo({ rootId: props.rootId, relPath: props.relPath })
    mediaInfo.value = info
    if (info.positionSec >= 10 && (info.durationSec === 0 || info.positionSec < info.durationSec - 15)) {
      resumeOfferSec.value = info.positionSec
    }
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '영상 메타데이터를 읽을 수 없습니다.'
  }

  saveTimer = setInterval(() => {
    if (videoRef.value && !videoRef.value.paused) {
      void persistProgress()
    }
  }, 10000)
})

onUnmounted(() => {
  if (saveTimer) clearInterval(saveTimer)
  void persistProgress()
})
</script>

<template>
  <div class="flex h-full w-full flex-col bg-black text-xs text-slate-200">
    <!-- Resume Prompt Banner -->
    <div
      v-if="resumeOfferSec !== null"
      class="flex items-center justify-between bg-sky-950/90 px-4 py-2 text-xs text-sky-200"
    >
      <span>이전에 시청하던 위치({{ formatDuration(resumeOfferSec) }})가 있습니다.</span>
      <div class="flex items-center gap-2">
        <button
          type="button"
          class="inline-flex items-center gap-1 rounded-lg bg-sky-500 px-2.5 py-1 font-semibold text-white hover:bg-sky-400"
          @click="applyResume"
        >
          <RotateCcw class="h-3.5 w-3.5" />
          이어보기
        </button>
        <button
          type="button"
          class="rounded-lg px-2 py-1 text-slate-300 hover:bg-white/10"
          @click="resumeOfferSec = null"
        >
          처음부터
        </button>
      </div>
    </div>

    <div v-if="errorMessage" class="bg-rose-500/20 px-4 py-2 text-rose-200">
      {{ errorMessage }}
    </div>

    <!-- Video Element -->
    <div class="relative flex min-h-0 flex-1 items-center justify-center bg-black">
      <video
        ref="videoRef"
        controls
        autoplay
        playsinline
        preload="metadata"
        class="max-h-full max-w-full"
        :src="streamUrl"
        crossorigin="use-credentials"
        @pause="persistProgress"
      >
        <track
          v-for="(sub, idx) in mediaInfo?.subtitles ?? []"
          :key="sub.relPath"
          kind="subtitles"
          :label="`${sub.name} (${sub.format.toUpperCase()})`"
          srclang="ko"
          :src="api.mediaSubtitleUrl({ rootId, relPath: sub.relPath })"
          :default="idx === 0"
        />
      </video>
    </div>

    <!-- Player Info Bar -->
    <div class="flex flex-wrap items-center justify-between gap-2 border-t border-white/10 bg-slate-950 px-3.5 py-2">
      <div class="flex min-w-0 items-center gap-3">
        <span class="truncate font-medium text-white">{{ fileName }}</span>
        <span v-if="mediaInfo" class="text-slate-400">{{ formatBytes(mediaInfo.size) }}</span>
        <span
          v-if="mediaInfo && mediaInfo.subtitles.length > 0"
          class="inline-flex items-center gap-1 rounded-md bg-emerald-500/15 px-2 py-0.5 text-[11px] text-emerald-300"
        >
          <Subtitles class="h-3 w-3" />
          자막 {{ mediaInfo.subtitles.length }}개 자동 연결됨
        </span>
      </div>

      <div class="flex items-center gap-2.5">
        <label class="flex items-center gap-1.5 text-slate-400">
          배속
          <select
            v-model.number="playbackRate"
            class="rounded-md border border-white/10 bg-slate-900 px-2 py-0.5 text-xs text-white focus:outline-none"
            @change="updatePlaybackRate"
          >
            <option :value="0.75">0.75x</option>
            <option :value="1">1.0x</option>
            <option :value="1.25">1.25x</option>
            <option :value="1.5">1.5x</option>
            <option :value="2">2.0x</option>
          </select>
        </label>

        <a
          :href="api.downloadUrl({ rootId, relPath })"
          class="inline-flex items-center gap-1 rounded-lg bg-white/10 px-2.5 py-1 text-slate-200 hover:bg-white/15"
        >
          <Download class="h-3.5 w-3.5" />
          원본 저장
        </a>
      </div>
    </div>
  </div>
</template>
