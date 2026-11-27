<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import Hls from 'hls.js'
import { Cpu, Download, Loader2, Play, RotateCcw, Subtitles } from 'lucide-vue-next'
import { api } from '../../api/client'
import type { HlsHwAccel, MediaInfo } from '../../types/api'
import { formatBytes, formatDuration } from '../../utils/format'

const props = defineProps<{
  readonly windowId: string
  readonly rootId: string
  readonly relPath: string
  readonly fileName: string
}>()

type PlaybackState =
  | { readonly kind: 'loading' }
  | { readonly kind: 'direct' }
  | { readonly kind: 'hls-preparing' }
  | {
      readonly kind: 'hls-ready'
      readonly sessionId: string
      readonly playlistUrl: string
      readonly hwAccel: HlsHwAccel
    }
  | { readonly kind: 'error'; readonly message: string }

const videoRef = ref<HTMLVideoElement | null>(null)
const mediaInfo = ref<MediaInfo | null>(null)
const playbackState = ref<PlaybackState>({ kind: 'loading' })
const resumeOfferSec = ref<number | null>(null)
const playbackRate = ref(1)
const errorMessage = ref('')

let saveTimer: ReturnType<typeof setInterval> | null = null
let hlsInstance: Hls | null = null
let activeHlsSessionId: string | null = null
let isUnmounted = false

const codecSummary = computed<string>(() => {
  const info = mediaInfo.value
  if (!info) return ''
  const parts: string[] = []
  if (info.videoCodec !== '') parts.push(info.videoCodec.toUpperCase())
  if (info.audioCodec !== '') parts.push(info.audioCodec.toUpperCase())
  return parts.join(' / ')
})

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

function cleanupHlsSession(): void {
  if (hlsInstance !== null) {
    hlsInstance.destroy()
    hlsInstance = null
  }
  if (activeHlsSessionId !== null) {
    const sid = activeHlsSessionId
    activeHlsSessionId = null
    void api.stopMediaHls(sid).catch(() => undefined)
  }
}

function handleBeforeUnload(): void {
  cleanupHlsSession()
}

function attachDirectPlayback(): void {
  const el = videoRef.value
  if (!el) return
  playbackState.value = { kind: 'direct' }
  el.src = api.mediaStreamUrl({ rootId: props.rootId, relPath: props.relPath })
  el.playbackRate = playbackRate.value
  void el.play().catch(() => undefined)
}

function attachHlsPlayback(playlistUrl: string): void {
  const el = videoRef.value
  if (!el) return

  if (Hls.isSupported()) {
    const hls = new Hls({
      enableWorker: true,
    })
    hlsInstance = hls
    hls.on(Hls.Events.ERROR, (_event, data) => {
      if (!data.fatal) return
      if (data.type === Hls.ErrorTypes.NETWORK_ERROR) {
        hls.startLoad()
      } else if (data.type === Hls.ErrorTypes.MEDIA_ERROR) {
        hls.recoverMediaError()
      } else {
        errorMessage.value = 'HLS 스트리밍 재생 중 치명적인 오류가 발생했습니다.'
        hls.destroy()
        hlsInstance = null
      }
    })
    hls.on(Hls.Events.MANIFEST_PARSED, () => {
      el.playbackRate = playbackRate.value
      void el.play().catch(() => undefined)
    })
    hls.loadSource(playlistUrl)
    hls.attachMedia(el)
    return
  }

  if (el.canPlayType('application/vnd.apple.mpegurl') !== '') {
    el.src = playlistUrl
    el.playbackRate = playbackRate.value
    void el.play().catch(() => undefined)
    return
  }

  const msg = '현재 브라우저에서는 HLS 스트리밍 재생을 지원하지 않습니다.'
  errorMessage.value = msg
  playbackState.value = { kind: 'error', message: msg }
}

async function initPlayback(info: MediaInfo): Promise<void> {
  if (info.playbackMode === 'direct') {
    attachDirectPlayback()
    return
  }

  if (!info.ffmpegAvailable) {
    const msg = '이 영상은 HLS 트랜스코딩이 필요하지만 서버에 FFmpeg가 설치되어 있지 않습니다.'
    errorMessage.value = msg
    playbackState.value = { kind: 'error', message: msg }
    return
  }

  playbackState.value = { kind: 'hls-preparing' }
  try {
    const started = await api.startMediaHls({ rootId: props.rootId, relPath: props.relPath })
    if (isUnmounted) {
      void api.stopMediaHls(started.sessionId).catch(() => undefined)
      return
    }
    activeHlsSessionId = started.sessionId
    playbackState.value = {
      kind: 'hls-ready',
      sessionId: started.sessionId,
      playlistUrl: started.playlistUrl,
      hwAccel: started.hwAccel,
    }
    attachHlsPlayback(started.playlistUrl)
  } catch (err) {
    const msg = err instanceof Error ? err.message : 'HLS 트랜스코딩 세션을 시작하지 못했습니다.'
    errorMessage.value = msg
    playbackState.value = { kind: 'error', message: msg }
  }
}

function applyResume(): void {
  const el = videoRef.value
  if (!el || resumeOfferSec.value === null) return
  el.currentTime = resumeOfferSec.value
  resumeOfferSec.value = null
  void el.play().catch(() => undefined)
}

function updatePlaybackRate(): void {
  if (videoRef.value) {
    videoRef.value.playbackRate = playbackRate.value
  }
}

onMounted(async () => {
  window.addEventListener('beforeunload', handleBeforeUnload)

  try {
    const info = await api.fetchMediaInfo({ rootId: props.rootId, relPath: props.relPath })
    if (isUnmounted) return
    mediaInfo.value = info
    if (info.positionSec >= 10 && (info.durationSec === 0 || info.positionSec < info.durationSec - 15)) {
      resumeOfferSec.value = info.positionSec
    }
    await initPlayback(info)
  } catch (err) {
    const msg = err instanceof Error ? err.message : '영상 메타데이터를 읽을 수 없습니다.'
    errorMessage.value = msg
    playbackState.value = { kind: 'error', message: msg }
  }

  saveTimer = setInterval(() => {
    if (videoRef.value && !videoRef.value.paused) {
      void persistProgress()
    }
  }, 10000)
})

onUnmounted(() => {
  isUnmounted = true
  window.removeEventListener('beforeunload', handleBeforeUnload)
  if (saveTimer) clearInterval(saveTimer)
  void persistProgress()
  cleanupHlsSession()
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
        playsinline
        preload="metadata"
        class="max-h-full max-w-full"
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

      <div
        v-if="playbackState.kind === 'hls-preparing' || playbackState.kind === 'loading'"
        class="pointer-events-none absolute inset-0 flex flex-col items-center justify-center gap-2.5 bg-black/75 text-white"
      >
        <Loader2 class="h-7 w-7 animate-spin text-[#3b9bff]" />
        <span class="text-[13px] font-medium">
          {{ playbackState.kind === 'hls-preparing' ? '트랜스코딩 세션 준비 중...' : '영상 정보를 불러오는 중...' }}
        </span>
      </div>
    </div>

    <div class="flex shrink-0 flex-wrap items-center justify-between gap-2 border-t border-[#dce2ea] bg-[#f8fafc] px-3 py-1.5 text-[#334155]">
      <div class="flex min-w-0 flex-wrap items-center gap-2">
        <span class="truncate font-semibold text-[#1e293b]">{{ fileName }}</span>
        <span v-if="mediaInfo" class="shrink-0 tabular-nums text-[11px] text-[#64748b]">
          {{ formatBytes(mediaInfo.size) }}
        </span>

        <span
          v-if="playbackState.kind === 'direct'"
          class="inline-flex shrink-0 items-center gap-1 rounded-[3px] border border-[#b8d4fe] bg-[#e6f0ff] px-2 py-0.5 text-[11px] font-medium text-[#0058cc]"
        >
          <Play class="h-3 w-3" />
          Direct Play
        </span>

        <span
          v-else-if="playbackState.kind === 'hls-ready' && playbackState.hwAccel === 'vaapi'"
          class="inline-flex shrink-0 items-center gap-1 rounded-[3px] border border-[#99f6e4] bg-[#f0fdfa] px-2 py-0.5 text-[11px] font-medium text-[#0f766e]"
        >
          <Cpu class="h-3 w-3" />
          VAAPI 하드웨어 가속
        </span>

        <span
          v-else-if="playbackState.kind === 'hls-ready'"
          class="inline-flex shrink-0 items-center gap-1 rounded-[3px] border border-[#fde68a] bg-[#fffbeb] px-2 py-0.5 text-[11px] font-medium text-[#b45309]"
        >
          <Cpu class="h-3 w-3" />
          SW 트랜스코딩
        </span>

        <span
          v-if="codecSummary"
          class="shrink-0 rounded-[3px] border border-[#dce2ea] bg-white px-1.5 py-0.5 font-mono text-[10px] text-[#475569]"
        >
          {{ codecSummary }}
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
