<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { Download, RotateCcw, Subtitles } from 'lucide-vue-next'
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
    return
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
  <div class="flex h-full w-full flex-col bg-black text-[12px] text-[#334155]">
    <div
      v-if="resumeOfferSec !== null"
      class="flex shrink-0 items-center justify-between gap-2 border-b border-[#b8d4fe] bg-[#eff6ff] px-3.5 py-1.5 text-[#1e3a8a]"
    >
      <span>이전에 시청하던 위치({{ formatDuration(resumeOfferSec) }})가 있습니다.</span>
      <div class="flex items-center gap-1.5">
        <button type="button" class="dsm-btn dsm-btn-primary" @click="applyResume">
          <RotateCcw class="h-3.5 w-3.5" />
          이어보기
        </button>
        <button type="button" class="dsm-btn" @click="resumeOfferSec = null">
          처음부터
        </button>
      </div>
    </div>

    <div
      v-if="errorMessage"
      class="shrink-0 border-b border-[#fecdd3] bg-[#fff1f2] px-3.5 py-1.5 text-[#be123c]"
    >
      {{ errorMessage }}
    </div>

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

    <div class="flex shrink-0 flex-wrap items-center justify-between gap-2 border-t border-[#dce2ea] bg-[#f8fafc] px-3 py-1.5 text-[#334155]">
      <div class="flex min-w-0 items-center gap-2.5">
        <span class="truncate font-semibold text-[#1e293b]">{{ fileName }}</span>
        <span v-if="mediaInfo" class="shrink-0 tabular-nums text-[11px] text-[#64748b]">
          {{ formatBytes(mediaInfo.size) }}
        </span>
        <span
          v-if="mediaInfo && mediaInfo.subtitles.length > 0"
          class="inline-flex shrink-0 items-center gap-1 rounded-[3px] border border-[#a7f3d0] bg-[#ecfdf5] px-2 py-0.5 text-[11px] font-medium text-[#047857]"
        >
          <Subtitles class="h-3 w-3" />
          자막 {{ mediaInfo.subtitles.length }}개 자동 연결됨
        </span>
      </div>

      <div class="flex items-center gap-2">
        <label class="flex items-center gap-1.5 text-[11px] font-medium text-[#475569]">
          배속
          <select
            v-model.number="playbackRate"
            class="dsm-select"
            @change="updatePlaybackRate"
          >
            <option :value="0.75">0.75x</option>
            <option :value="1">1.0x</option>
            <option :value="1.25">1.25x</option>
            <option :value="1.5">1.5x</option>
            <option :value="2">2.0x</option>
          </select>
        </label>

        <a :href="api.downloadUrl({ rootId, relPath })" class="dsm-btn">
          <Download class="h-3.5 w-3.5" />
          원본 저장
        </a>
      </div>
    </div>
  </div>
</template>
