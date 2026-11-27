<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import Hls from 'hls.js'
import {
  Check,
  Cpu,
  Download,
  FastForward,
  FileText,
  Film,
  ListVideo,
  Loader2,
  Maximize,
  Minimize,
  Pause,
  PictureInPicture2,
  Play,
  Rewind,
  RotateCcw,
  SkipBack,
  SkipForward,
  Subtitles,
  Upload,
  Volume2,
  VolumeX,
  X,
} from 'lucide-vue-next'
import { api } from '../../api/client'
import { useDesktopStore } from '../../stores/desktop'
import { isRecord, type FileEntry, type HlsHwAccel, type MediaInfo } from '../../types/api'
import { formatBytes, formatDuration } from '../../utils/format'
import {
  activeCuesAt,
  decodeSubtitleBuffer,
  parseSubtitleFile,
  type SubtitleCue,
} from '../../utils/subtitles'

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

type SubtitleFontSize = 'sm' | 'md' | 'lg' | 'xl'
type SubtitlePosition = 'low' | 'mid' | 'high'
type SubtitleBgOpacity = 0 | 45 | 75
type SeekStepSec = 5 | 10 | 30

interface LoadedSubtitleTrack {
  readonly id: string
  readonly label: string
  readonly source: 'server' | 'local'
  readonly cues: readonly SubtitleCue[]
}

interface PlayerPreferences {
  readonly seekStepSec: SeekStepSec
  readonly autoNext: boolean
  readonly subtitleFontSize: SubtitleFontSize
  readonly subtitlePosition: SubtitlePosition
  readonly subtitleBgOpacity: SubtitleBgOpacity
}

const PREFS_STORAGE_KEY = 'ps_video_player_prefs_v1'
const PLAYBACK_RATES: readonly number[] = [0.5, 0.75, 1, 1.25, 1.5, 1.75, 2]
const SEEK_STEPS: readonly SeekStepSec[] = [5, 10, 30]
const SUBTITLE_FONT_OPTIONS: readonly { readonly value: SubtitleFontSize; readonly label: string }[] = [
  { value: 'sm', label: '작게' },
  { value: 'md', label: '보통' },
  { value: 'lg', label: '크게' },
  { value: 'xl', label: '최대' },
]
const SUBTITLE_POSITION_OPTIONS: readonly { readonly value: SubtitlePosition; readonly label: string }[] = [
  { value: 'low', label: '낮게' },
  { value: 'mid', label: '보통' },
  { value: 'high', label: '높게' },
]
const SUBTITLE_BG_OPACITIES: readonly SubtitleBgOpacity[] = [0, 45, 75]

function parsePreferences(raw: string | null): PlayerPreferences {
  const defaults: PlayerPreferences = {
    seekStepSec: 5,
    autoNext: true,
    subtitleFontSize: 'md',
    subtitlePosition: 'mid',
    subtitleBgOpacity: 45,
  }
  if (!raw) return defaults
  try {
    const parsed: unknown = JSON.parse(raw)
    if (!isRecord(parsed)) return defaults
    const seekStepSec: SeekStepSec =
      parsed.seekStepSec === 10 || parsed.seekStepSec === 30 ? parsed.seekStepSec : 5
    const autoNext = typeof parsed.autoNext === 'boolean' ? parsed.autoNext : true
    const subtitleFontSize: SubtitleFontSize =
      parsed.subtitleFontSize === 'sm' ||
      parsed.subtitleFontSize === 'lg' ||
      parsed.subtitleFontSize === 'xl'
        ? parsed.subtitleFontSize
        : 'md'
    const subtitlePosition: SubtitlePosition =
      parsed.subtitlePosition === 'low' || parsed.subtitlePosition === 'high'
        ? parsed.subtitlePosition
        : 'mid'
    const subtitleBgOpacity: SubtitleBgOpacity =
      parsed.subtitleBgOpacity === 0 || parsed.subtitleBgOpacity === 75
        ? parsed.subtitleBgOpacity
        : 45
    return {
      seekStepSec,
      autoNext,
      subtitleFontSize,
      subtitlePosition,
      subtitleBgOpacity,
    }
  } catch {
    return defaults
  }
}

function savePreferences(prefs: PlayerPreferences): void {
  try {
    localStorage.setItem(PREFS_STORAGE_KEY, JSON.stringify(prefs))
  } catch {
    return
  }
}

function parentDirectoryOf(relPath: string): string {
  const idx = relPath.lastIndexOf('/')
  return idx > 0 ? relPath.slice(0, idx) : ''
}

const desktopStore = useDesktopStore()
const initialPrefs = parsePreferences(
  typeof window !== 'undefined' ? window.localStorage.getItem(PREFS_STORAGE_KEY) : null,
)

const stageRef = ref<HTMLDivElement | null>(null)
const videoRef = ref<HTMLVideoElement | null>(null)
const seekBarRef = ref<HTMLDivElement | null>(null)
const subtitleFileInputRef = ref<HTMLInputElement | null>(null)

const currentRelPath = ref(props.relPath)
const currentFileName = ref(props.fileName)

const mediaInfo = ref<MediaInfo | null>(null)
const playbackState = ref<PlaybackState>({ kind: 'loading' })
const resumeOfferSec = ref<number | null>(null)
const errorMessage = ref('')

const isPlaying = ref(false)
const isBuffering = ref(false)
const currentTime = ref(0)
const duration = ref(0)
const hlsOffsetSec = ref(0)
const probedDurationSec = ref(0)
const bufferedRanges = ref<readonly { readonly startPct: number; readonly widthPct: number }[]>([])
const volume = ref(1)
const isMuted = ref(false)
const playbackRate = ref(1)
const isFullscreen = ref(false)
const isPipSupported = ref(false)

const seekStepSec = ref<SeekStepSec>(initialPrefs.seekStepSec)
const autoNext = ref<boolean>(initialPrefs.autoNext)

const subtitleTracks = ref<readonly LoadedSubtitleTrack[]>([])
const activeSubtitleId = ref<string | null>(null)
const lastSelectedSubtitleId = ref<string | null>(null)
const subtitleOffsetSec = ref(0)
const subtitleFontSize = ref<SubtitleFontSize>(initialPrefs.subtitleFontSize)
const subtitlePosition = ref<SubtitlePosition>(initialPrefs.subtitlePosition)
const subtitleBgOpacity = ref<SubtitleBgOpacity>(initialPrefs.subtitleBgOpacity)
const subtitleMenuOpen = ref(false)
const subtitleDragOver = ref(false)

const playlistItems = ref<readonly FileEntry[]>([])
const playlistLoading = ref(false)
const playlistOpen = ref(false)

const hudVisible = ref(true)
const hudHoveringControls = ref(false)
const isScrubbing = ref(false)
const hoverSeekPct = ref<number | null>(null)
const hoverSeekTime = ref(0)

const osdText = ref('')
const osdVisible = ref(false)

let saveTimer: ReturnType<typeof setInterval> | null = null
let hudHideTimer: ReturnType<typeof setTimeout> | null = null
let osdTimer: ReturnType<typeof setTimeout> | null = null
let clickToggleTimer: ReturnType<typeof setTimeout> | null = null
let pendingHlsSeekTimer: ReturnType<typeof setTimeout> | null = null
let hlsInstance: Hls | null = null
let activeHlsSessionId: string | null = null
let isUnmounted = false
let loadGeneration = 0
let hlsSeekGeneration = 0

const activeTrack = computed<LoadedSubtitleTrack | null>(() => {
  if (activeSubtitleId.value === null) return null
  return subtitleTracks.value.find((t) => t.id === activeSubtitleId.value) ?? null
})

const visibleCues = computed<readonly SubtitleCue[]>(() => {
  const track = activeTrack.value
  if (!track) return []
  return activeCuesAt(track.cues, currentTime.value, subtitleOffsetSec.value)
})

const progressPct = computed<number>(() => {
  if (duration.value <= 0 || !Number.isFinite(currentTime.value)) return 0
  return Math.min(100, Math.max(0, (currentTime.value / duration.value) * 100))
})

const currentPlaylistIndex = computed<number>(() =>
  playlistItems.value.findIndex((entry) => entry.relPath === currentRelPath.value),
)

const prevEpisode = computed<FileEntry | null>(() => {
  const idx = currentPlaylistIndex.value
  if (idx <= 0) return null
  return playlistItems.value[idx - 1] ?? null
})

const nextEpisode = computed<FileEntry | null>(() => {
  const idx = currentPlaylistIndex.value
  if (idx < 0 || idx >= playlistItems.value.length - 1) return null
  return playlistItems.value[idx + 1] ?? null
})

const codecSummary = computed<string>(() => {
  const info = mediaInfo.value
  if (!info) return ''
  const parts: string[] = []
  if (info.videoCodec !== '') parts.push(info.videoCodec.toUpperCase())
  if (info.audioCodec !== '') parts.push(info.audioCodec.toUpperCase())
  return parts.join(' / ')
})

const showHud = computed<boolean>(
  () =>
    hudVisible.value ||
    !isPlaying.value ||
    hudHoveringControls.value ||
    isScrubbing.value ||
    subtitleMenuOpen.value ||
    playlistOpen.value,
)

const subtitleOverlayStyle = computed<Record<string, string>>(() => {
  const fontMap: Readonly<Record<SubtitleFontSize, string>> = {
    sm: '15px',
    md: '20px',
    lg: '26px',
    xl: '32px',
  }
  const bottomMap: Readonly<Record<SubtitlePosition, string>> = {
    low: showHud.value ? '82px' : '24px',
    mid: showHud.value ? '96px' : '48px',
    high: showHud.value ? '124px' : '84px',
  }
  return {
    fontSize: fontMap[subtitleFontSize.value],
    bottom: bottomMap[subtitlePosition.value],
  }
})

const subtitleCueBgStyle = computed<Record<string, string>>(() => ({
  backgroundColor: `rgba(0, 0, 0, ${subtitleBgOpacity.value / 100})`,
}))

function persistPrefs(): void {
  savePreferences({
    seekStepSec: seekStepSec.value,
    autoNext: autoNext.value,
    subtitleFontSize: subtitleFontSize.value,
    subtitlePosition: subtitlePosition.value,
    subtitleBgOpacity: subtitleBgOpacity.value,
  })
}

function showOsd(text: string): void {
  osdText.value = text
  osdVisible.value = true
  if (osdTimer) clearTimeout(osdTimer)
  osdTimer = setTimeout(() => {
    osdVisible.value = false
  }, 700)
}

function scheduleHudAutoHide(): void {
  if (hudHideTimer) clearTimeout(hudHideTimer)
  hudVisible.value = true
  if (!isPlaying.value) return
  hudHideTimer = setTimeout(() => {
    if (
      isPlaying.value &&
      !hudHoveringControls.value &&
      !isScrubbing.value &&
      !subtitleMenuOpen.value &&
      !playlistOpen.value
    ) {
      hudVisible.value = false
    }
  }, 2500)
}

function handleStageMouseMove(): void {
  scheduleHudAutoHide()
}

function handleStageMouseLeave(): void {
  if (isPlaying.value && !subtitleMenuOpen.value && !playlistOpen.value && !isScrubbing.value) {
    hudVisible.value = false
  }
}

async function persistProgress(targetRelPath = currentRelPath.value): Promise<void> {
  const pos = currentTime.value
  if (!Number.isFinite(pos) || pos < 1) return
  try {
    await api.saveMediaProgress({
      rootId: props.rootId,
      relPath: targetRelPath,
      positionSec: pos,
      durationSec: Number.isFinite(duration.value) ? duration.value : 0,
    })
  } catch {
    return
  }
}

function cleanupHlsSession(): void {
  if (pendingHlsSeekTimer) {
    clearTimeout(pendingHlsSeekTimer)
    pendingHlsSeekTimer = null
  }
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

function syncDuration(): void {
  const el = videoRef.value
  const elDur = el && Number.isFinite(el.duration) && el.duration > 0 ? el.duration : 0
  if (mediaInfo.value?.playbackMode === 'hls') {
    duration.value = Math.max(probedDurationSec.value, hlsOffsetSec.value + elDur)
    return
  }
  duration.value = elDur > 0 ? elDur : probedDurationSec.value
}

function updateBufferedRanges(): void {
  const el = videoRef.value
  const total = duration.value
  if (!el || !Number.isFinite(total) || total <= 0) {
    bufferedRanges.value = []
    return
  }
  const offset = mediaInfo.value?.playbackMode === 'hls' ? hlsOffsetSec.value : 0
  const out: { readonly startPct: number; readonly widthPct: number }[] = []
  for (let i = 0; i < el.buffered.length; i += 1) {
    const start = offset + el.buffered.start(i)
    const end = offset + el.buffered.end(i)
    const startPct = Math.min(100, Math.max(0, (start / total) * 100))
    const endPct = Math.min(100, Math.max(startPct, (end / total) * 100))
    out.push({ startPct, widthPct: endPct - startPct })
  }
  bufferedRanges.value = out
}

function attachDirectPlayback(relPath: string): void {
  const el = videoRef.value
  if (!el) return
  playbackState.value = { kind: 'direct' }
  el.src = api.mediaStreamUrl({ rootId: props.rootId, relPath })
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

async function loadServerSubtitles(info: MediaInfo, gen: number): Promise<void> {
  const loaded: LoadedSubtitleTrack[] = []
  for (const sub of info.subtitles) {
    try {
      const res = await fetch(api.mediaSubtitleUrl({ rootId: props.rootId, relPath: sub.relPath }), {
        credentials: 'same-origin',
      })
      if (!res.ok) continue
      const buf = await res.arrayBuffer()
      if (isUnmounted || gen !== loadGeneration) return
      const text = decodeSubtitleBuffer(buf)
      const cues = parseSubtitleFile(sub.name, text)
      if (cues.length > 0) {
        loaded.push({
          id: `server:${sub.relPath}`,
          label: `${sub.name} (${sub.format.toUpperCase()})`,
          source: 'server',
          cues,
        })
      }
    } catch {
      continue
    }
  }
  if (isUnmounted || gen !== loadGeneration) return
  subtitleTracks.value = loaded
  const firstTrack = loaded[0]
  if (firstTrack) {
    activeSubtitleId.value = firstTrack.id
    lastSelectedSubtitleId.value = firstTrack.id
  } else {
    activeSubtitleId.value = null
    lastSelectedSubtitleId.value = null
  }
}

async function loadFolderPlaylist(relPath: string, gen: number): Promise<void> {
  playlistLoading.value = true
  try {
    const parentDir = parentDirectoryOf(relPath)
    const list = await api.listFiles({ rootId: props.rootId, relPath: parentDir })
    if (isUnmounted || gen !== loadGeneration) return
    const videos = list.entries
      .filter((entry) => !entry.isDir && entry.kind === 'video')
      .slice()
      .sort((a, b) => a.name.localeCompare(b.name, 'ko', { numeric: true, sensitivity: 'base' }))
    playlistItems.value = videos
  } catch {
    if (gen === loadGeneration) {
      playlistItems.value = []
    }
  } finally {
    if (gen === loadGeneration) {
      playlistLoading.value = false
    }
  }
}

async function startHlsAt(relPath: string, startSec: number, gen: number): Promise<void> {
  const seekGen = ++hlsSeekGeneration
  cleanupHlsSession()
  const el = videoRef.value
  if (el) {
    el.pause()
  }

  const cleanStart = Math.max(0, Math.floor(startSec))
  hlsOffsetSec.value = cleanStart
  currentTime.value = cleanStart
  bufferedRanges.value = []
  playbackState.value = { kind: 'hls-preparing' }

  try {
    const started = await api.startMediaHls({
      rootId: props.rootId,
      relPath,
      startSec: cleanStart,
    })
    if (isUnmounted || gen !== loadGeneration || seekGen !== hlsSeekGeneration) {
      void api.stopMediaHls(started.sessionId).catch(() => undefined)
      return
    }
    activeHlsSessionId = started.sessionId
    hlsOffsetSec.value = started.startSec
    currentTime.value = started.startSec
    playbackState.value = {
      kind: 'hls-ready',
      sessionId: started.sessionId,
      playlistUrl: started.playlistUrl,
      hwAccel: started.hwAccel,
    }
    attachHlsPlayback(started.playlistUrl)
  } catch (err) {
    if (gen !== loadGeneration || seekGen !== hlsSeekGeneration) return
    const msg = err instanceof Error ? err.message : 'HLS 트랜스코딩 세션을 시작하지 못했습니다.'
    errorMessage.value = msg
    playbackState.value = { kind: 'error', message: msg }
  }
}

async function initPlayback(info: MediaInfo, relPath: string, gen: number): Promise<void> {
  if (info.playbackMode === 'direct') {
    hlsOffsetSec.value = 0
    attachDirectPlayback(relPath)
    return
  }

  if (!info.ffmpegAvailable) {
    const msg = '이 영상은 HLS 트랜스코딩이 필요하지만 서버에 FFmpeg가 설치되어 있지 않습니다.'
    errorMessage.value = msg
    playbackState.value = { kind: 'error', message: msg }
    return
  }

  await startHlsAt(relPath, 0, gen)
}

async function loadVideo(relPath: string, fileName: string, isSwitch: boolean): Promise<void> {
  const gen = ++loadGeneration
  if (isSwitch) {
    await persistProgress(currentRelPath.value)
    cleanupHlsSession()
    const el = videoRef.value
    if (el) {
      el.pause()
      el.removeAttribute('src')
      el.load()
    }
  }

  currentRelPath.value = relPath
  currentFileName.value = fileName
  desktopStore.setWindowTitle({ windowId: props.windowId, title: fileName })

  mediaInfo.value = null
  playbackState.value = { kind: 'loading' }
  resumeOfferSec.value = null
  errorMessage.value = ''
  currentTime.value = 0
  duration.value = 0
  hlsOffsetSec.value = 0
  probedDurationSec.value = 0
  bufferedRanges.value = []
  isPlaying.value = false
  isBuffering.value = false
  subtitleTracks.value = []
  activeSubtitleId.value = null
  lastSelectedSubtitleId.value = null
  subtitleOffsetSec.value = 0

  void loadFolderPlaylist(relPath, gen)

  try {
    const info = await api.fetchMediaInfo({ rootId: props.rootId, relPath })
    if (isUnmounted || gen !== loadGeneration) return
    mediaInfo.value = info
    if (Number.isFinite(info.durationSec) && info.durationSec > 0) {
      probedDurationSec.value = info.durationSec
      duration.value = info.durationSec
    }
    if (info.positionSec >= 10 && (info.durationSec === 0 || info.positionSec < info.durationSec - 15)) {
      resumeOfferSec.value = info.positionSec
    }
    void loadServerSubtitles(info, gen)
    await initPlayback(info, relPath, gen)
  } catch (err) {
    if (gen !== loadGeneration) return
    const msg = err instanceof Error ? err.message : '영상 메타데이터를 읽을 수 없습니다.'
    errorMessage.value = msg
    playbackState.value = { kind: 'error', message: msg }
  }
}

function switchToEpisode(entry: FileEntry): void {
  if (entry.relPath === currentRelPath.value) return
  void loadVideo(entry.relPath, entry.name, true)
}

function togglePlay(): void {
  const el = videoRef.value
  if (!el) return
  if (el.paused) {
    void el.play().catch(() => undefined)
    showOsd('재생')
  } else {
    el.pause()
    showOsd('일시정지')
  }
  scheduleHudAutoHide()
}

function handleCenterClick(): void {
  if (subtitleMenuOpen.value) {
    subtitleMenuOpen.value = false
    return
  }
  if (clickToggleTimer) clearTimeout(clickToggleTimer)
  clickToggleTimer = setTimeout(() => {
    clickToggleTimer = null
    togglePlay()
  }, 180)
}

function handleCenterDoubleClick(): void {
  if (clickToggleTimer) {
    clearTimeout(clickToggleTimer)
    clickToggleTimer = null
  }
  void toggleFullscreen()
}

function seekToAbsolute(targetSec: number, immediateHlsRestart: boolean): void {
  const el = videoRef.value
  const maxTime = duration.value > 0 ? duration.value : Math.max(targetSec, 0)
  const target = Math.min(maxTime, Math.max(0, targetSec))
  currentTime.value = target

  if (mediaInfo.value?.playbackMode !== 'hls') {
    if (el && Number.isFinite(target)) {
      el.currentTime = target
    }
    return
  }

  const localTarget = target - hlsOffsetSec.value
  const elDur = el && Number.isFinite(el.duration) && el.duration > 0 ? el.duration : 0
  if (el && localTarget >= 0 && elDur > 0 && localTarget <= Math.max(0, elDur - 0.5)) {
    if (pendingHlsSeekTimer) {
      clearTimeout(pendingHlsSeekTimer)
      pendingHlsSeekTimer = null
    }
    el.currentTime = localTarget
    return
  }

  if (pendingHlsSeekTimer) {
    clearTimeout(pendingHlsSeekTimer)
    pendingHlsSeekTimer = null
  }
  const relPath = currentRelPath.value
  const gen = loadGeneration
  if (immediateHlsRestart) {
    void startHlsAt(relPath, target, gen)
    return
  }
  pendingHlsSeekTimer = setTimeout(() => {
    pendingHlsSeekTimer = null
    if (gen === loadGeneration && currentRelPath.value === relPath) {
      void startHlsAt(relPath, currentTime.value, gen)
    }
  }, 250)
}

function seekBy(deltaSec: number): void {
  const base = Number.isFinite(currentTime.value) ? currentTime.value : 0
  seekToAbsolute(base + deltaSec, false)
  scheduleHudAutoHide()
}

function setVolumeClamped(nextVol: number): void {
  const el = videoRef.value
  const clamped = Math.min(1, Math.max(0, Math.round(nextVol * 100) / 100))
  volume.value = clamped
  isMuted.value = clamped === 0
  if (el) {
    el.volume = clamped
    el.muted = clamped === 0
  }
  showOsd(clamped === 0 ? '음소거' : `볼륨 ${Math.round(clamped * 100)}%`)
  scheduleHudAutoHide()
}

function toggleMute(): void {
  const el = videoRef.value
  if (!el) return
  const nextMuted = !el.muted
  el.muted = nextMuted
  isMuted.value = nextMuted
  if (!nextMuted && el.volume === 0) {
    el.volume = 0.5
    volume.value = 0.5
  }
  showOsd(nextMuted ? '음소거' : `볼륨 ${Math.round(volume.value * 100)}%`)
  scheduleHudAutoHide()
}

function setPlaybackRate(rate: number): void {
  const clamped = Math.min(2, Math.max(0.5, Math.round(rate * 100) / 100))
  playbackRate.value = clamped
  if (videoRef.value) {
    videoRef.value.playbackRate = clamped
  }
  showOsd(`배속 ${clamped}x`)
  scheduleHudAutoHide()
}

function adjustSubtitleOffset(deltaSec: number): void {
  const next = Math.round((subtitleOffsetSec.value + deltaSec) * 10) / 10
  subtitleOffsetSec.value = next
  const sign = next > 0 ? '+' : ''
  showOsd(`자막 싱크 ${sign}${next.toFixed(1)}s`)
}

function resetSubtitleOffset(): void {
  subtitleOffsetSec.value = 0
  showOsd('자막 싱크 0.0s')
}

function selectSubtitleTrack(id: string | null): void {
  activeSubtitleId.value = id
  if (id !== null) {
    lastSelectedSubtitleId.value = id
  }
}

function toggleSubtitles(): void {
  if (activeSubtitleId.value !== null) {
    lastSelectedSubtitleId.value = activeSubtitleId.value
    activeSubtitleId.value = null
    showOsd('자막 꺼짐')
    return
  }
  const candidate = lastSelectedSubtitleId.value ?? subtitleTracks.value[0]?.id ?? null
  if (candidate !== null) {
    activeSubtitleId.value = candidate
    showOsd('자막 켜짐')
  } else {
    showOsd('불러온 자막 없음')
  }
}

async function importLocalSubtitleFile(file: File): Promise<void> {
  try {
    const buf = await file.arrayBuffer()
    const raw = decodeSubtitleBuffer(buf)
    const cues = parseSubtitleFile(file.name, raw)
    if (cues.length === 0) {
      errorMessage.value = `'${file.name}'에서 유효한 자막 항목을 찾지 못했습니다.`
      return
    }
    const id = `local:${file.name}:${Date.now()}`
    const track: LoadedSubtitleTrack = {
      id,
      label: `${file.name} (로컬)`,
      source: 'local',
      cues,
    }
    subtitleTracks.value = [...subtitleTracks.value, track]
    activeSubtitleId.value = id
    lastSelectedSubtitleId.value = id
    subtitleOffsetSec.value = 0
    showOsd(`자막 불러옴: ${file.name}`)
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '자막 파일을 읽지 못했습니다.'
  }
}

function handleSubtitleFilePicked(ev: Event): void {
  if (!(ev.target instanceof HTMLInputElement)) return
  const file = ev.target.files?.[0]
  if (file) {
    void importLocalSubtitleFile(file)
  }
  ev.target.value = ''
}

function handleStageDragOver(ev: DragEvent): void {
  if (ev.dataTransfer?.types.includes('Files')) {
    ev.preventDefault()
    subtitleDragOver.value = true
  }
}

function handleStageDragLeave(): void {
  subtitleDragOver.value = false
}

function handleStageDrop(ev: DragEvent): void {
  ev.preventDefault()
  subtitleDragOver.value = false
  const file = ev.dataTransfer?.files?.[0]
  if (!file) return
  const lower = file.name.toLowerCase()
  if (lower.endsWith('.srt') || lower.endsWith('.smi') || lower.endsWith('.sami') || lower.endsWith('.vtt')) {
    void importLocalSubtitleFile(file)
  } else {
    errorMessage.value = '지원하는 자막 형식(.srt, .smi, .vtt)만 드롭할 수 있습니다.'
  }
}

function calcSeekPosition(clientX: number): { readonly pct: number; readonly sec: number } {
  const bar = seekBarRef.value
  if (!bar || duration.value <= 0) return { pct: 0, sec: 0 }
  const rect = bar.getBoundingClientRect()
  const ratio = rect.width > 0 ? Math.min(1, Math.max(0, (clientX - rect.left) / rect.width)) : 0
  return { pct: ratio * 100, sec: ratio * duration.value }
}

function handleSeekHover(ev: MouseEvent): void {
  const pos = calcSeekPosition(ev.clientX)
  hoverSeekPct.value = pos.pct
  hoverSeekTime.value = pos.sec
}

function handleSeekLeave(): void {
  if (!isScrubbing.value) {
    hoverSeekPct.value = null
  }
}

function applySeekFromClientX(clientX: number, commitHls: boolean): void {
  const pos = calcSeekPosition(clientX)
  hoverSeekPct.value = pos.pct
  hoverSeekTime.value = pos.sec
  currentTime.value = pos.sec
  if (mediaInfo.value?.playbackMode === 'hls') {
    const el = videoRef.value
    const localTarget = pos.sec - hlsOffsetSec.value
    const elDur = el && Number.isFinite(el.duration) && el.duration > 0 ? el.duration : 0
    if (el && localTarget >= 0 && elDur > 0 && localTarget <= Math.max(0, elDur - 0.5)) {
      el.currentTime = localTarget
    } else if (commitHls) {
      seekToAbsolute(pos.sec, true)
    }
    return
  }
  if (videoRef.value && Number.isFinite(pos.sec)) {
    videoRef.value.currentTime = pos.sec
  }
}

function handleSeekPointerDown(ev: PointerEvent): void {
  if (ev.button !== 0) return
  ev.preventDefault()
  isScrubbing.value = true
  applySeekFromClientX(ev.clientX, false)
  window.addEventListener('pointermove', handleSeekPointerMove)
  window.addEventListener('pointerup', handleSeekPointerUp)
}

function handleSeekPointerMove(ev: PointerEvent): void {
  if (!isScrubbing.value) return
  applySeekFromClientX(ev.clientX, false)
}

function handleSeekPointerUp(): void {
  isScrubbing.value = false
  hoverSeekPct.value = null
  window.removeEventListener('pointermove', handleSeekPointerMove)
  window.removeEventListener('pointerup', handleSeekPointerUp)
  if (mediaInfo.value?.playbackMode === 'hls') {
    seekToAbsolute(currentTime.value, true)
  }
  scheduleHudAutoHide()
}

async function toggleFullscreen(): Promise<void> {
  const container = stageRef.value
  if (!container) return
  try {
    if (document.fullscreenElement === container) {
      await document.exitFullscreen()
    } else {
      await container.requestFullscreen()
    }
  } catch {
    return
  }
}

async function togglePip(): Promise<void> {
  const el = videoRef.value
  if (!el || !isPipSupported.value) return
  try {
    if (document.pictureInPictureElement === el) {
      await document.exitPictureInPicture()
    } else {
      await el.requestPictureInPicture()
    }
  } catch {
    return
  }
}

function handleFullscreenChange(): void {
  isFullscreen.value = stageRef.value !== null && document.fullscreenElement === stageRef.value
}

function applyResume(): void {
  if (resumeOfferSec.value === null) return
  const target = resumeOfferSec.value
  resumeOfferSec.value = null
  seekToAbsolute(target, true)
  if (mediaInfo.value?.playbackMode !== 'hls') {
    void videoRef.value?.play().catch(() => undefined)
  }
}

function handleVideoTimeUpdate(): void {
  const el = videoRef.value
  if (!el || isScrubbing.value) return
  const raw = Number.isFinite(el.currentTime) ? el.currentTime : 0
  const offset = mediaInfo.value?.playbackMode === 'hls' ? hlsOffsetSec.value : 0
  currentTime.value = offset + raw
}

function handleVideoLoadedMetadata(): void {
  const el = videoRef.value
  if (!el) return
  syncDuration()
  el.volume = volume.value
  el.muted = isMuted.value
  el.playbackRate = playbackRate.value
  updateBufferedRanges()
}

function handleVideoEnded(): void {
  isPlaying.value = false
  hudVisible.value = true
  void persistProgress()
  if (autoNext.value && nextEpisode.value) {
    switchToEpisode(nextEpisode.value)
  }
}

function isEditableElement(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  const tag = target.tagName
  return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || target.isContentEditable
}

function handleKeyDown(ev: KeyboardEvent): void {
  const isActivePlayer = isFullscreen.value || desktopStore.activeWindowId === props.windowId
  if (!isActivePlayer) return
  if (ev.metaKey || ev.ctrlKey || ev.altKey) return
  if (isEditableElement(ev.target)) return

  switch (ev.key) {
    case 'ArrowLeft':
      ev.preventDefault()
      seekBy(ev.shiftKey ? -30 : -seekStepSec.value)
      return
    case 'ArrowRight':
      ev.preventDefault()
      seekBy(ev.shiftKey ? 30 : seekStepSec.value)
      return
    case ' ':
    case 'k':
    case 'K':
      ev.preventDefault()
      togglePlay()
      return
    case 'ArrowUp':
      ev.preventDefault()
      setVolumeClamped((isMuted.value ? 0 : volume.value) + 0.05)
      return
    case 'ArrowDown':
      ev.preventDefault()
      setVolumeClamped((isMuted.value ? 0 : volume.value) - 0.05)
      return
    case 'm':
    case 'M':
      ev.preventDefault()
      toggleMute()
      return
    case 'f':
    case 'F':
      ev.preventDefault()
      void toggleFullscreen()
      return
    case 'p':
    case 'P':
      ev.preventDefault()
      void togglePip()
      return
    case '[':
      ev.preventDefault()
      setPlaybackRate(playbackRate.value - 0.25)
      return
    case ']':
      ev.preventDefault()
      setPlaybackRate(playbackRate.value + 0.25)
      return
    case 'c':
    case 'C':
      ev.preventDefault()
      toggleSubtitles()
      return
    case ',':
      ev.preventDefault()
      adjustSubtitleOffset(-0.5)
      return
    case '.':
      ev.preventDefault()
      adjustSubtitleOffset(0.5)
      return
    default:
      return
  }
}

function handleVolumeInput(ev: Event): void {
  if (ev.target instanceof HTMLInputElement) {
    setVolumeClamped(Number(ev.target.value))
  }
}

function handleSeekStepChange(step: SeekStepSec): void {
  seekStepSec.value = step
  persistPrefs()
  showOsd(`탐색 간격 ${step}초`)
}

function handleAutoNextToggle(): void {
  autoNext.value = !autoNext.value
  persistPrefs()
}

function setSubtitleFontSize(size: SubtitleFontSize): void {
  subtitleFontSize.value = size
  persistPrefs()
}

function setSubtitlePosition(pos: SubtitlePosition): void {
  subtitlePosition.value = pos
  persistPrefs()
}

function setSubtitleBgOpacity(opacity: SubtitleBgOpacity): void {
  subtitleBgOpacity.value = opacity
  persistPrefs()
}

onMounted(() => {
  isPipSupported.value =
    'pictureInPictureEnabled' in document && Boolean(document.pictureInPictureEnabled)
  window.addEventListener('beforeunload', handleBeforeUnload)
  window.addEventListener('keydown', handleKeyDown)
  document.addEventListener('fullscreenchange', handleFullscreenChange)

  void loadVideo(props.relPath, props.fileName, false)

  saveTimer = setInterval(() => {
    if (videoRef.value && !videoRef.value.paused) {
      void persistProgress()
    }
  }, 10000)
})

onUnmounted(() => {
  isUnmounted = true
  window.removeEventListener('beforeunload', handleBeforeUnload)
  window.removeEventListener('keydown', handleKeyDown)
  window.removeEventListener('pointermove', handleSeekPointerMove)
  window.removeEventListener('pointerup', handleSeekPointerUp)
  document.removeEventListener('fullscreenchange', handleFullscreenChange)
  if (saveTimer) clearInterval(saveTimer)
  if (hudHideTimer) clearTimeout(hudHideTimer)
  if (osdTimer) clearTimeout(osdTimer)
  if (clickToggleTimer) clearTimeout(clickToggleTimer)
  void persistProgress()
  cleanupHlsSession()
})
function handlePlaybackRateSelect(ev: Event): void {
  if (ev.target instanceof HTMLSelectElement) {
    setPlaybackRate(Number(ev.target.value))
  }
}
</script>

<template>
  <div class="flex h-full w-full flex-col bg-black text-[12px] text-[#334155] select-none">
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
      class="flex shrink-0 items-center justify-between gap-2 border-b border-[#fecdd3] bg-[#fff1f2] px-3.5 py-1.5 text-[#be123c]"
    >
      <span class="truncate">{{ errorMessage }}</span>
      <button type="button" class="cursor-pointer hover:underline" @click="errorMessage = ''">닫기</button>
    </div>

    <div class="flex min-h-0 flex-1 overflow-hidden">
      <div
        ref="stageRef"
        class="relative flex min-h-0 min-w-0 flex-1 items-center justify-center overflow-hidden bg-black"
        :class="{ 'cursor-none': !showHud }"
        @mousemove="handleStageMouseMove"
        @mouseleave="handleStageMouseLeave"
        @dragover="handleStageDragOver"
        @dragleave="handleStageDragLeave"
        @drop="handleStageDrop"
      >
        <video
          ref="videoRef"
          playsinline
          preload="metadata"
          class="h-full w-full object-contain"
          crossorigin="use-credentials"
          @play="
            () => {
              isPlaying = true
              isBuffering = false
              scheduleHudAutoHide()
            }
          "
          @pause="
            () => {
              isPlaying = false
              hudVisible = true
              void persistProgress()
            }
          "
          @waiting="isBuffering = true"
          @playing="isBuffering = false"
          @canplay="isBuffering = false"
          @timeupdate="handleVideoTimeUpdate"
          @durationchange="handleVideoLoadedMetadata"
          @loadedmetadata="handleVideoLoadedMetadata"
          @progress="updateBufferedRanges"
          @ended="handleVideoEnded"
        />

        <div
          class="absolute inset-0 z-10"
          @click="handleCenterClick"
          @dblclick="handleCenterDoubleClick"
        />

        <div
          v-if="subtitleDragOver"
          class="pointer-events-none absolute inset-0 z-30 flex flex-col items-center justify-center gap-2 border-2 border-dashed border-[#3b9bff] bg-[#0572ec]/25 text-white"
        >
          <Upload class="h-8 w-8" />
          <span class="text-[14px] font-semibold">여기에 자막 파일(.srt, .smi, .vtt)을 놓으세요</span>
        </div>

        <div
          v-if="playbackState.kind === 'hls-preparing' || playbackState.kind === 'loading' || isBuffering"
          class="pointer-events-none absolute inset-0 z-20 flex flex-col items-center justify-center gap-2.5 bg-black/50 text-white"
        >
          <Loader2 class="h-8 w-8 animate-spin text-[#3b9bff]" />
          <span class="text-[13px] font-medium">
            {{
              playbackState.kind === 'hls-preparing'
                ? '트랜스코딩 세션 준비 중...'
                : playbackState.kind === 'loading'
                  ? '영상 정보를 불러오는 중...'
                  : '버퍼링 중...'
            }}
          </span>
        </div>

        <div
          class="pointer-events-none absolute top-1/2 left-1/2 z-20 -translate-x-1/2 -translate-y-1/2 rounded-[8px] bg-black/75 px-4 py-2 text-[15px] font-semibold text-white shadow-lg backdrop-blur-sm transition-opacity duration-200"
          :class="osdVisible ? 'opacity-100' : 'opacity-0'"
        >
          {{ osdText }}
        </div>

        <div
          v-if="visibleCues.length > 0"
          class="pointer-events-none absolute right-6 left-6 z-20 flex flex-col items-center gap-1 text-center transition-all duration-150"
          :style="subtitleOverlayStyle"
        >
          <div
            v-for="(cue, idx) in visibleCues"
            :key="`${cue.startSec}-${idx}`"
            class="max-w-[85%] rounded-[4px] px-3 py-1 leading-snug font-semibold whitespace-pre-line text-white"
            :style="{
              ...subtitleCueBgStyle,
              textShadow: '0 1px 3px rgba(0,0,0,0.95), 0 0 6px rgba(0,0,0,0.85)',
            }"
          >
            {{ cue.text }}
          </div>
        </div>

        <div
          class="pointer-events-none absolute inset-x-0 top-0 z-20 flex items-center justify-between bg-gradient-to-b from-black/75 via-black/30 to-transparent px-4 pt-3 pb-8 text-white transition-opacity duration-200"
          :class="showHud ? 'opacity-100' : 'opacity-0'"
        >
          <div class="flex min-w-0 items-center gap-2">
            <span class="truncate text-[13px] font-semibold drop-shadow">{{ currentFileName }}</span>
            <span
              v-if="subtitleOffsetSec !== 0 && activeTrack"
              class="shrink-0 rounded-[3px] bg-black/55 px-1.5 py-0.5 font-mono text-[10.5px] text-[#93c5fd]"
            >
              싱크 {{ subtitleOffsetSec > 0 ? '+' : '' }}{{ subtitleOffsetSec.toFixed(1) }}s
            </span>
          </div>
        </div>

        <div
          v-if="subtitleMenuOpen"
          class="absolute right-3 bottom-16 z-30 w-72 overflow-hidden rounded-[6px] border border-white/15 bg-[#121821]/95 text-white shadow-2xl backdrop-blur-md"
          @click.stop
          @mouseenter="hudHoveringControls = true"
          @mouseleave="hudHoveringControls = false"
        >
          <div class="flex items-center justify-between border-b border-white/10 px-3 py-2">
            <span class="text-[12px] font-semibold">자막 설정</span>
            <button type="button" class="cursor-pointer text-white/70 hover:text-white" @click="subtitleMenuOpen = false">
              <X class="h-3.5 w-3.5" />
            </button>
          </div>

          <div class="max-h-72 space-y-3 overflow-auto p-3 text-[11.5px]">
            <div>
              <div class="mb-1.5 flex items-center justify-between text-[11px] text-white/70">
                <span>트랙 선택</span>
                <button
                  type="button"
                  class="inline-flex cursor-pointer items-center gap-1 rounded-[3px] bg-[#0572ec] px-2 py-0.5 text-[10.5px] font-medium text-white hover:bg-[#005ec4]"
                  @click="subtitleFileInputRef?.click()"
                >
                  <Upload class="h-3 w-3" />
                  파일 불러오기
                </button>
                <input
                  ref="subtitleFileInputRef"
                  type="file"
                  accept=".srt,.smi,.sami,.vtt"
                  class="hidden"
                  @change="handleSubtitleFilePicked"
                />
              </div>

              <div class="space-y-1">
                <button
                  type="button"
                  class="flex w-full cursor-pointer items-center justify-between rounded-[4px] px-2 py-1.5 text-left transition-colors"
                  :class="activeSubtitleId === null ? 'bg-[#0572ec]/30 text-white' : 'bg-white/5 text-white/80 hover:bg-white/10'"
                  @click="selectSubtitleTrack(null)"
                >
                  <span>자막 끄기 (OFF)</span>
                  <Check v-if="activeSubtitleId === null" class="h-3.5 w-3.5 text-[#3b9bff]" />
                </button>

                <button
                  v-for="track in subtitleTracks"
                  :key="track.id"
                  type="button"
                  class="flex w-full cursor-pointer items-center justify-between gap-2 rounded-[4px] px-2 py-1.5 text-left transition-colors"
                  :class="activeSubtitleId === track.id ? 'bg-[#0572ec]/30 text-white' : 'bg-white/5 text-white/80 hover:bg-white/10'"
                  @click="selectSubtitleTrack(track.id)"
                >
                  <span class="truncate">{{ track.label }}</span>
                  <Check v-if="activeSubtitleId === track.id" class="h-3.5 w-3.5 shrink-0 text-[#3b9bff]" />
                </button>
              </div>
            </div>

            <div class="border-t border-white/10 pt-2.5">
              <div class="mb-1.5 flex items-center justify-between text-[11px] text-white/70">
                <span>싱크 조절 (, / .)</span>
                <span class="font-mono text-white">
                  {{ subtitleOffsetSec > 0 ? '+' : '' }}{{ subtitleOffsetSec.toFixed(1) }}s
                </span>
              </div>
              <div class="grid grid-cols-3 gap-1.5">
                <button
                  type="button"
                  class="cursor-pointer rounded-[4px] bg-white/10 py-1 text-center hover:bg-white/20"
                  @click="adjustSubtitleOffset(-0.5)"
                >
                  -0.5s
                </button>
                <button
                  type="button"
                  class="cursor-pointer rounded-[4px] bg-white/10 py-1 text-center hover:bg-white/20"
                  @click="resetSubtitleOffset"
                >
                  0.0s
                </button>
                <button
                  type="button"
                  class="cursor-pointer rounded-[4px] bg-white/10 py-1 text-center hover:bg-white/20"
                  @click="adjustSubtitleOffset(0.5)"
                >
                  +0.5s
                </button>
              </div>
            </div>

            <div class="border-t border-white/10 pt-2.5">
              <div class="mb-1.5 text-[11px] text-white/70">글자 크기</div>
              <div class="grid grid-cols-4 gap-1">
                <button
                  v-for="opt in SUBTITLE_FONT_OPTIONS"
                  :key="opt.value"
                  type="button"
                  class="cursor-pointer rounded-[4px] py-1 text-center text-[11px]"
                  :class="subtitleFontSize === opt.value ? 'bg-[#0572ec] text-white' : 'bg-white/10 text-white/80 hover:bg-white/20'"
                  @click="setSubtitleFontSize(opt.value)"
                >
                  {{ opt.label }}
                </button>
              </div>
            </div>

            <div class="border-t border-white/10 pt-2.5">
              <div class="mb-1.5 text-[11px] text-white/70">자막 위치</div>
              <div class="grid grid-cols-3 gap-1">
                <button
                  v-for="opt in SUBTITLE_POSITION_OPTIONS"
                  :key="opt.value"
                  type="button"
                  class="cursor-pointer rounded-[4px] py-1 text-center text-[11px]"
                  :class="subtitlePosition === opt.value ? 'bg-[#0572ec] text-white' : 'bg-white/10 text-white/80 hover:bg-white/20'"
                  @click="setSubtitlePosition(opt.value)"
                >
                  {{ opt.label }}
                </button>
              </div>
            </div>

            <div class="border-t border-white/10 pt-2.5">
              <div class="mb-1.5 text-[11px] text-white/70">배경 불투명도</div>
              <div class="grid grid-cols-3 gap-1">
                <button
                  v-for="op in SUBTITLE_BG_OPACITIES"
                  :key="op"
                  type="button"
                  class="cursor-pointer rounded-[4px] py-1 text-center text-[11px]"
                  :class="subtitleBgOpacity === op ? 'bg-[#0572ec] text-white' : 'bg-white/10 text-white/80 hover:bg-white/20'"
                  @click="setSubtitleBgOpacity(op)"
                >
                  {{ op }}%
                </button>
              </div>
            </div>
          </div>
        </div>

        <div
          class="absolute inset-x-0 bottom-0 z-20 bg-gradient-to-t from-black/90 via-black/65 to-transparent px-3.5 pt-8 pb-2.5 text-white transition-opacity duration-200"
          :class="showHud ? 'opacity-100' : 'pointer-events-none opacity-0'"
          @mouseenter="hudHoveringControls = true"
          @mouseleave="hudHoveringControls = false"
          @click.stop
        >
          <div
            ref="seekBarRef"
            class="group relative mb-2 flex h-4 cursor-pointer items-center"
            @mousemove="handleSeekHover"
            @mouseleave="handleSeekLeave"
            @pointerdown="handleSeekPointerDown"
          >
            <div
              v-if="hoverSeekPct !== null && duration > 0"
              class="pointer-events-none absolute -top-7 -translate-x-1/2 rounded-[4px] bg-black/85 px-1.5 py-0.5 font-mono text-[10.5px] tabular-nums text-white shadow"
              :style="{ left: `${Math.min(96, Math.max(4, hoverSeekPct))}%` }"
            >
              {{ formatDuration(hoverSeekTime) }}
            </div>

            <div class="relative h-1.5 w-full overflow-hidden rounded-full bg-white/20 transition-all group-hover:h-2">
              <div
                v-for="(seg, idx) in bufferedRanges"
                :key="idx"
                class="absolute inset-y-0 bg-white/30"
                :style="{ left: `${seg.startPct}%`, width: `${seg.widthPct}%` }"
              />
              <div
                class="absolute inset-y-0 left-0 rounded-full bg-[#0572ec]"
                :style="{ width: `${progressPct}%` }"
              />
            </div>

            <div
              class="pointer-events-none absolute h-3.5 w-3.5 -translate-x-1/2 rounded-full border-2 border-white bg-[#0572ec] shadow transition-transform"
              :class="isScrubbing ? 'scale-110' : 'scale-0 group-hover:scale-100'"
              :style="{ left: `${progressPct}%` }"
            />
          </div>

          <div class="flex flex-nowrap items-center justify-between gap-2 overflow-x-auto">
            <div class="flex items-center gap-1">
              <button
                type="button"
                class="flex h-7 w-7 cursor-pointer items-center justify-center rounded-[4px] hover:bg-white/15"
                :title="isPlaying ? '일시정지 (Space / K)' : '재생 (Space / K)'"
                @click="togglePlay"
              >
                <Pause v-if="isPlaying" class="h-4 w-4" />
                <Play v-else class="h-4 w-4" />
              </button>

              <button
                type="button"
                class="flex h-7 cursor-pointer items-center gap-0.5 rounded-[4px] px-1.5 text-[11px] font-medium hover:bg-white/15"
                :title="`-${seekStepSec}초 (←)`"
                @click="seekBy(-seekStepSec)"
              >
                <Rewind class="h-3.5 w-3.5" />
                <span class="tabular-nums">-{{ seekStepSec }}s</span>
              </button>

              <button
                type="button"
                class="flex h-7 cursor-pointer items-center gap-0.5 rounded-[4px] px-1.5 text-[11px] font-medium hover:bg-white/15"
                :title="`+${seekStepSec}초 (→)`"
                @click="seekBy(seekStepSec)"
              >
                <span class="tabular-nums">+{{ seekStepSec }}s</span>
                <FastForward class="h-3.5 w-3.5" />
              </button>

              <div class="mx-0.5 hidden items-center rounded-[4px] bg-white/10 p-0.5 sm:inline-flex">
                <button
                  v-for="step in SEEK_STEPS"
                  :key="step"
                  type="button"
                  class="cursor-pointer rounded-[3px] px-1.5 py-0.5 text-[10px] tabular-nums transition-colors"
                  :class="seekStepSec === step ? 'bg-[#0572ec] text-white' : 'text-white/75 hover:text-white'"
                  @click="handleSeekStepChange(step)"
                >
                  {{ step }}s
                </button>
              </div>

              <button
                type="button"
                class="flex h-7 w-7 cursor-pointer items-center justify-center rounded-[4px] hover:bg-white/15 disabled:cursor-not-allowed disabled:opacity-35"
                title="이전 화"
                :disabled="!prevEpisode"
                @click="prevEpisode && switchToEpisode(prevEpisode)"
              >
                <SkipBack class="h-3.5 w-3.5" />
              </button>

              <button
                type="button"
                class="flex h-7 w-7 cursor-pointer items-center justify-center rounded-[4px] hover:bg-white/15 disabled:cursor-not-allowed disabled:opacity-35"
                title="다음 화"
                :disabled="!nextEpisode"
                @click="nextEpisode && switchToEpisode(nextEpisode)"
              >
                <SkipForward class="h-3.5 w-3.5" />
              </button>

              <div class="ml-1 flex items-center gap-1.5">
                <button
                  type="button"
                  class="flex h-7 w-7 cursor-pointer items-center justify-center rounded-[4px] hover:bg-white/15"
                  title="음소거 (M)"
                  @click="toggleMute"
                >
                  <VolumeX v-if="isMuted || volume === 0" class="h-4 w-4" />
                  <Volume2 v-else class="h-4 w-4" />
                </button>
                <input
                  type="range"
                  min="0"
                  max="1"
                  step="0.05"
                  :value="isMuted ? 0 : volume"
                  class="h-1 w-16 cursor-pointer accent-[#0572ec]"
                  aria-label="볼륨"
                  @input="handleVolumeInput"
                />
              </div>

              <span class="ml-1.5 font-mono text-[11px] tabular-nums text-white/90">
                {{ formatDuration(currentTime) }} / {{ formatDuration(duration) }}
              </span>
            </div>

            <div class="flex items-center gap-1">
              <select
                :value="playbackRate"
                class="h-7 cursor-pointer rounded-[4px] border border-white/15 bg-black/50 px-1.5 text-[11px] text-white outline-none hover:bg-white/15"
                title="재생 배속 ([ / ])"
                @change="handlePlaybackRateSelect"
              >
                <option v-for="rate in PLAYBACK_RATES" :key="rate" :value="rate" class="bg-[#121821] text-white">
                  {{ rate }}x
                </option>
              </select>

              <button
                type="button"
                class="flex h-7 cursor-pointer items-center gap-1 rounded-[4px] px-2 text-[11px] font-medium transition-colors"
                :class="activeTrack || subtitleMenuOpen ? 'bg-[#0572ec] text-white' : 'hover:bg-white/15 text-white/90'"
                title="자막 설정 (C)"
                @click="subtitleMenuOpen = !subtitleMenuOpen"
              >
                <Subtitles class="h-3.5 w-3.5" />
                <span>자막</span>
              </button>

              <button
                type="button"
                class="flex h-7 cursor-pointer items-center gap-1 rounded-[4px] px-2 text-[11px] font-medium transition-colors"
                :class="playlistOpen ? 'bg-[#0572ec] text-white' : 'hover:bg-white/15 text-white/90'"
                title="폴더 재생목록"
                @click="playlistOpen = !playlistOpen"
              >
                <ListVideo class="h-3.5 w-3.5" />
                <span class="tabular-nums">{{ playlistItems.length }}</span>
              </button>

              <button
                v-if="isPipSupported"
                type="button"
                class="flex h-7 w-7 cursor-pointer items-center justify-center rounded-[4px] hover:bg-white/15"
                title="PIP 모드 (P)"
                @click="togglePip"
              >
                <PictureInPicture2 class="h-3.5 w-3.5" />
              </button>

              <button
                type="button"
                class="flex h-7 w-7 cursor-pointer items-center justify-center rounded-[4px] hover:bg-white/15"
                title="전체화면 (F)"
                @click="toggleFullscreen"
              >
                <Minimize v-if="isFullscreen" class="h-3.5 w-3.5" />
                <Maximize v-else class="h-3.5 w-3.5" />
              </button>
            </div>
          </div>
        </div>
      </div>

      <aside
        v-if="playlistOpen"
        class="flex w-64 shrink-0 flex-col border-l border-[#dce2ea] bg-[#f8fafc] text-[#1e293b]"
      >
        <div class="flex shrink-0 items-center justify-between border-b border-[#dce2ea] px-3 py-2">
          <div class="flex items-center gap-1.5 font-semibold">
            <Film class="h-3.5 w-3.5 text-[#0572ec]" />
            <span>폴더 재생목록 ({{ playlistItems.length }})</span>
          </div>
          <button type="button" class="cursor-pointer text-[#64748b] hover:text-[#1e293b]" @click="playlistOpen = false">
            <X class="h-3.5 w-3.5" />
          </button>
        </div>

        <div class="flex shrink-0 items-center justify-between border-b border-[#dce2ea] bg-white px-3 py-1.5 text-[11px]">
          <span class="text-[#475569]">자동 다음 화</span>
          <button
            type="button"
            class="cursor-pointer rounded-full px-2 py-0.5 text-[10.5px] font-semibold transition-colors"
            :class="autoNext ? 'bg-[#0572ec] text-white' : 'bg-[#e2e8f0] text-[#475569]'"
            @click="handleAutoNextToggle"
          >
            {{ autoNext ? 'ON' : 'OFF' }}
          </button>
        </div>

        <div class="min-h-0 flex-1 overflow-auto p-1.5">
          <div v-if="playlistLoading" class="py-6 text-center text-[11px] text-[#64748b]">
            목록을 불러오는 중...
          </div>
          <div v-else-if="playlistItems.length === 0" class="py-6 text-center text-[11px] text-[#64748b]">
            같은 폴더에 영상이 없습니다.
          </div>
          <div v-else class="space-y-0.5">
            <button
              v-for="(item, idx) in playlistItems"
              :key="item.relPath"
              type="button"
              class="flex w-full cursor-pointer items-center gap-2 rounded-[4px] px-2 py-1.5 text-left transition-colors"
              :class="
                item.relPath === currentRelPath
                  ? 'bg-[#e6f0ff] font-semibold text-[#0572ec]'
                  : 'text-[#334155] hover:bg-[#e9eef5]'
              "
              @click="switchToEpisode(item)"
            >
              <span class="w-4 shrink-0 text-right font-mono text-[10.5px] tabular-nums text-[#64748b]">
                {{ idx + 1 }}
              </span>
              <span class="min-w-0 flex-1 truncate text-[11.5px]" :title="item.name">{{ item.name }}</span>
            </button>
          </div>
        </div>
      </aside>
    </div>

    <div class="flex shrink-0 flex-nowrap items-center justify-between gap-2 overflow-x-auto border-t border-[#dce2ea] bg-[#f8fafc] px-3 py-1.5 text-[#334155]">
      <div class="flex min-w-0 flex-nowrap items-center gap-2">
        <span class="max-w-[260px] truncate font-semibold text-[#1e293b]" :title="currentFileName">
          {{ currentFileName }}
        </span>
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
          v-else-if="playbackState.kind === 'hls-ready' && playbackState.hwAccel === 'copy'"
          class="inline-flex shrink-0 items-center gap-1 rounded-[3px] border border-[#bbf7d0] bg-[#f0fdf4] px-2 py-0.5 text-[11px] font-medium text-[#15803d]"
        >
          <Play class="h-3 w-3" />
          비디오 복사 (오디오 변환)
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
          v-if="activeTrack"
          class="inline-flex shrink-0 items-center gap-1 rounded-[3px] border border-[#a7f3d0] bg-[#ecfdf5] px-2 py-0.5 text-[11px] font-medium text-[#047857]"
        >
          <FileText class="h-3 w-3" />
          {{ activeTrack.label }}
        </span>
      </div>

      <div class="flex items-center gap-2">
        <a :href="api.downloadUrl({ rootId, relPath: currentRelPath })" class="dsm-btn">
          <Download class="h-3.5 w-3.5" />
          원본 저장
        </a>
      </div>
    </div>
  </div>
</template>
