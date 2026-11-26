<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import {
  AlertTriangle,
  Check,
  ChevronRight,
  Copy,
  Cpu,
  Download,
  ExternalLink,
  FileText,
  Folder,
  FolderArchive,
  HardDrive,
  Play,
  Plus,
  RefreshCw,
  RotateCcw,
  Square,
  Terminal,
  Trash2,
  Upload,
  X,
} from 'lucide-vue-next'
import { api } from '../../api/client'
import type { WargameLaunchSource } from '../../stores/desktop'
import { useMountsStore } from '../../stores/mounts'
import type {
  FileEntry,
  MountRoot,
  WargameConnKind,
  WargameInstance,
  WargameMode,
  WargamePortBinding,
  WargameStatus,
  WargameTemplateKind,
} from '../../types/api'
import { formatBytes, formatDuration } from '../../utils/format'

const props = defineProps<{
  readonly launchSource?: WargameLaunchSource
}>()

const mountsStore = useMountsStore()

type PrepareTab = 'upload' | 'mount'

interface InstanceDraft {
  readonly name: string
  readonly templateKind: WargameTemplateKind
  readonly containerPort: number
  readonly connKind: WargameConnKind
  readonly relaxSecurity: boolean
}

type LogsDrawerState =
  | { readonly kind: 'closed' }
  | {
      readonly kind: 'open'
      readonly instanceId: string
      readonly instanceName: string
      readonly buildLog: string
      readonly containerLog: string
      readonly status: WargameStatus
      readonly loading: boolean
      readonly error: string
    }

const TEMPLATE_OPTIONS: readonly {
  readonly value: Exclude<WargameTemplateKind, ''>
  readonly label: string
  readonly defaultPort: number
  readonly defaultConn: WargameConnKind
}[] = [
  { value: 'pwn-socat', label: 'Pwnable ELF (socat 래핑)', defaultPort: 9000, defaultConn: 'nc' },
  { value: 'python-flask', label: 'Python Web (Flask / Gunicorn)', defaultPort: 5000, defaultConn: 'http' },
  { value: 'php-apache', label: 'PHP Web (Apache)', defaultPort: 80, defaultConn: 'http' },
  { value: 'node-express', label: 'Node.js Web (Express)', defaultPort: 3000, defaultConn: 'http' },
]

const ARCHIVE_SUFFIXES: readonly string[] = ['.tar.gz', '.tar.xz', '.tgz', '.zip', '.tar']

const loadedOnce = ref(false)
const loadingList = ref(false)
const dockerAvailable = ref(true)
const dockerError = ref('')
const defaultBindIp = ref('')
const instances = ref<readonly WargameInstance[]>([])
const drafts = ref<Record<string, InstanceDraft>>({})
const busyById = ref<Record<string, string>>({})
const confirmDeleteId = ref<string | null>(null)
const errorMessage = ref('')
const copiedKey = ref('')

const prepareTab = ref<PrepareTab>('upload')
const prepareName = ref('')
const preparing = ref(false)
const dragOverUpload = ref(false)
const archiveInputRef = ref<HTMLInputElement | null>(null)

const mountRoots = ref<readonly MountRoot[]>([])
const mountRootId = ref('')
const mountRelPath = ref('')
const mountEntries = ref<readonly FileEntry[]>([])
const mountSelectedRelPath = ref<string | null>(null)
const mountLoading = ref(false)

const logsDrawer = ref<LogsDrawerState>({ kind: 'closed' })
const lastHandledNonce = ref<number | null>(null)
const nowSec = ref(Math.floor(Date.now() / 1000))

let clockTimer: ReturnType<typeof setInterval> | null = null
let buildPollTimer: ReturnType<typeof setInterval> | null = null
let logsPollTimer: ReturnType<typeof setInterval> | null = null

function stripArchiveSuffix(rawName: string): string {
  const trimmed = rawName.trim()
  const lower = trimmed.toLowerCase()
  for (const suffix of ARCHIVE_SUFFIXES) {
    if (lower.endsWith(suffix) && trimmed.length > suffix.length) {
      return trimmed.slice(0, trimmed.length - suffix.length)
    }
  }
  return trimmed
}

function isSupportedArchiveName(fileName: string): boolean {
  const lower = fileName.trim().toLowerCase()
  return ARCHIVE_SUFFIXES.some((suffix) => lower.endsWith(suffix))
}

function templateDefaults(kind: WargameTemplateKind): {
  readonly port: number
  readonly connKind: WargameConnKind
} {
  switch (kind) {
    case 'pwn-socat':
      return { port: 9000, connKind: 'nc' }
    case 'python-flask':
      return { port: 5000, connKind: 'http' }
    case 'php-apache':
      return { port: 80, connKind: 'http' }
    case 'node-express':
      return { port: 3000, connKind: 'http' }
    case '':
      return { port: 8080, connKind: 'http' }
    default: {
      const _exhaustive: never = kind
      return _exhaustive
    }
  }
}

function createDraftFromInstance(inst: WargameInstance): InstanceDraft {
  const effectiveTemplate: WargameTemplateKind =
    inst.mode === 'template' && inst.templateKind === '' ? 'pwn-socat' : inst.templateKind
  const defaults = templateDefaults(effectiveTemplate)
  const firstPort = inst.ports[0]
  return {
    name: inst.name,
    templateKind: effectiveTemplate,
    containerPort: firstPort && firstPort.containerPort > 0 ? firstPort.containerPort : defaults.port,
    connKind: firstPort ? firstPort.connKind : defaults.connKind,
    relaxSecurity: inst.relaxSecurity,
  }
}

function syncDraftsForList(nextList: readonly WargameInstance[], forceResetId?: string): void {
  const nextDrafts: Record<string, InstanceDraft> = {}
  for (const inst of nextList) {
    const existing = drafts.value[inst.id]
    if (existing !== undefined && inst.id !== forceResetId) {
      nextDrafts[inst.id] = existing
    } else {
      nextDrafts[inst.id] = createDraftFromInstance(inst)
    }
  }
  drafts.value = nextDrafts
}

function readDraft(inst: WargameInstance): InstanceDraft {
  return drafts.value[inst.id] ?? createDraftFromInstance(inst)
}

function patchDraft(inst: WargameInstance, patch: Partial<InstanceDraft>): void {
  const current = readDraft(inst)
  drafts.value = {
    ...drafts.value,
    [inst.id]: {
      ...current,
      ...patch,
    },
  }
}

function parseTemplateSelectValue(raw: string): WargameTemplateKind {
  switch (raw) {
    case 'pwn-socat':
    case 'python-flask':
    case 'php-apache':
    case 'node-express':
      return raw
    default:
      return 'pwn-socat'
  }
}

function parseConnSelectValue(raw: string): WargameConnKind {
  return raw === 'http' ? 'http' : 'nc'
}

function handleDraftNameInput(inst: WargameInstance, ev: Event): void {
  if (ev.target instanceof HTMLInputElement) {
    patchDraft(inst, { name: ev.target.value })
  }
}

function handleDraftTemplateChange(inst: WargameInstance, ev: Event): void {
  if (!(ev.target instanceof HTMLSelectElement)) return
  const nextKind = parseTemplateSelectValue(ev.target.value)
  const prevDraft = readDraft(inst)
  const prevDefaults = templateDefaults(prevDraft.templateKind)
  const nextDefaults = templateDefaults(nextKind)
  const shouldAdoptPort =
    prevDraft.containerPort <= 0 || prevDraft.containerPort === prevDefaults.port
  patchDraft(inst, {
    templateKind: nextKind,
    connKind: nextDefaults.connKind,
    containerPort: shouldAdoptPort ? nextDefaults.port : prevDraft.containerPort,
  })
}

function handleDraftPortInput(inst: WargameInstance, ev: Event): void {
  if (!(ev.target instanceof HTMLInputElement)) return
  const parsed = Number.parseInt(ev.target.value, 10)
  patchDraft(inst, {
    containerPort: Number.isFinite(parsed) && parsed > 0 ? Math.min(65535, parsed) : 0,
  })
}

function handleDraftConnChange(inst: WargameInstance, ev: Event): void {
  if (ev.target instanceof HTMLSelectElement) {
    patchDraft(inst, { connKind: parseConnSelectValue(ev.target.value) })
  }
}

function handleDraftRelaxChange(inst: WargameInstance, ev: Event): void {
  if (ev.target instanceof HTMLInputElement) {
    patchDraft(inst, { relaxSecurity: ev.target.checked })
  }
}

function upsertInstance(updated: WargameInstance, resetDraft = false): void {
  const idx = instances.value.findIndex((item) => item.id === updated.id)
  const nextList =
    idx >= 0
      ? instances.value.map((item) => (item.id === updated.id ? updated : item))
      : [updated, ...instances.value]
  syncDraftsForList(nextList, resetDraft ? updated.id : undefined)
  instances.value = nextList
}

function setBusy(id: string, label: string | null): void {
  const next = { ...busyById.value }
  if (label === null) {
    delete next[id]
  } else {
    next[id] = label
  }
  busyById.value = next
}

function isBusy(id: string): boolean {
  return busyById.value[id] !== undefined
}

async function refreshWargames(silent = false): Promise<void> {
  if (!silent) {
    loadingList.value = true
  }
  try {
    const res = await api.listWargames()
    dockerAvailable.value = res.dockerAvailable
    dockerError.value = res.dockerError
    defaultBindIp.value = res.bindIp
    syncDraftsForList(res.instances)
    instances.value = res.instances
    loadedOnce.value = true
  } catch (err) {
    if (!silent) {
      errorMessage.value = err instanceof Error ? err.message : '워게임 목록을 불러오지 못했습니다.'
    }
  } finally {
    if (!silent) {
      loadingList.value = false
    }
  }
}

const hasBuildingInstance = computed<boolean>(() =>
  instances.value.some((inst) => inst.status === 'building'),
)

watch(
  hasBuildingInstance,
  (building) => {
    if (building && buildPollTimer === null) {
      buildPollTimer = setInterval(() => {
        void refreshWargames(true)
      }, 1500)
    } else if (!building && buildPollTimer !== null) {
      clearInterval(buildPollTimer)
      buildPollTimer = null
    }
  },
  { immediate: true },
)

async function prepareFromUpload(file: File): Promise<void> {
  if (!isSupportedArchiveName(file.name)) {
    errorMessage.value = '.zip, .tar.gz, .tgz, .tar.xz, .tar 형식의 압축 파일만 지원합니다.'
    return
  }
  preparing.value = true
  errorMessage.value = ''
  try {
    const customName = prepareName.value.trim()
    const created = await api.prepareWargameUpload({
      file,
      name: customName !== '' ? customName : stripArchiveSuffix(file.name),
    })
    prepareName.value = ''
    upsertInstance(created, true)
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '압축 파일 분석에 실패했습니다.'
  } finally {
    preparing.value = false
  }
}

function handleArchiveInputChange(ev: Event): void {
  const input = ev.target
  if (input instanceof HTMLInputElement && input.files) {
    const firstFile = input.files[0]
    if (firstFile) {
      void prepareFromUpload(firstFile)
    }
    input.value = ''
  }
}

function handleUploadDrop(ev: DragEvent): void {
  dragOverUpload.value = false
  const firstFile = ev.dataTransfer?.files?.[0]
  if (firstFile) {
    void prepareFromUpload(firstFile)
  }
}

async function prepareFromMountSource(params: {
  readonly rootId: string
  readonly relPath: string
  readonly name: string
}): Promise<void> {
  if (!params.rootId) return
  preparing.value = true
  errorMessage.value = ''
  try {
    const created = await api.prepareWargameMount({
      rootId: params.rootId,
      relPath: params.relPath,
      name: params.name,
    })
    prepareName.value = ''
    upsertInstance(created, true)
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '마운트 경로 워게임 준비에 실패했습니다.'
  } finally {
    preparing.value = false
  }
}

const mountBreadcrumbs = computed(() => {
  if (!mountRelPath.value) return []
  const parts = mountRelPath.value.split('/').filter(Boolean)
  return parts.map((segment, idx) => ({
    name: segment,
    relPath: parts.slice(0, idx + 1).join('/'),
  }))
})

const mountSelectableEntries = computed<readonly FileEntry[]>(() =>
  mountEntries.value.filter(
    (entry) => entry.isDir || entry.kind === 'archive' || isSupportedArchiveName(entry.name),
  ),
)

const mountSelectedEntry = computed<FileEntry | null>(() => {
  if (!mountSelectedRelPath.value) return null
  return mountSelectableEntries.value.find((e) => e.relPath === mountSelectedRelPath.value) ?? null
})

const activeMountTarget = computed<{
  readonly rootId: string
  readonly relPath: string
  readonly suggestedName: string
  readonly displayPath: string
} | null>(() => {
  if (!mountRootId.value) return null
  const rootName = mountRoots.value.find((r) => r.id === mountRootId.value)?.name ?? mountRootId.value
  const picked = mountSelectedEntry.value
  if (picked) {
    return {
      rootId: mountRootId.value,
      relPath: picked.relPath,
      suggestedName: stripArchiveSuffix(picked.name),
      displayPath: `${rootName} / ${picked.relPath}`,
    }
  }
  const leaf = mountRelPath.value.split('/').filter(Boolean).pop() ?? rootName
  return {
    rootId: mountRootId.value,
    relPath: mountRelPath.value,
    suggestedName: leaf,
    displayPath: mountRelPath.value ? `${rootName} / ${mountRelPath.value}` : `${rootName} (루트 폴더)`,
  }
})

async function loadMountDir(rootId: string, relPath: string): Promise<void> {
  if (!rootId) return
  mountLoading.value = true
  try {
    const res = await api.listFiles({ rootId, relPath })
    mountRootId.value = res.rootId
    mountRelPath.value = res.relPath
    mountEntries.value = res.entries
    mountSelectedRelPath.value = null
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '마운트 폴더 조회에 실패했습니다.'
  } finally {
    mountLoading.value = false
  }
}

async function ensureMountRootsLoaded(): Promise<void> {
  try {
    const roots = await api.fetchRoots()
    mountRoots.value = roots
    const existingStillPresent = roots.some((r) => r.id === mountRootId.value)
    if (existingStillPresent && mountRootId.value) {
      await loadMountDir(mountRootId.value, mountRelPath.value)
      return
    }
    const first = roots[0]
    if (first) {
      await loadMountDir(first.id, '')
    } else {
      mountRootId.value = ''
      mountRelPath.value = ''
      mountEntries.value = []
      mountSelectedRelPath.value = null
    }
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '마운트 목록을 불러오지 못했습니다.'
  }
}

function switchPrepareTab(tab: PrepareTab): void {
  prepareTab.value = tab
  if (tab === 'mount' && mountRoots.value.length === 0) {
    void ensureMountRootsLoaded()
  }
}

function handleMountEntryClick(entry: FileEntry): void {
  if (mountSelectedRelPath.value === entry.relPath) {
    mountSelectedRelPath.value = null
  } else {
    mountSelectedRelPath.value = entry.relPath
  }
}

function handleMountEntryDblClick(entry: FileEntry): void {
  if (entry.isDir) {
    void loadMountDir(mountRootId.value, entry.relPath)
  }
}

function handlePrepareSelectedMount(): void {
  const target = activeMountTarget.value
  if (!target) return
  const customName = prepareName.value.trim()
  void prepareFromMountSource({
    rootId: target.rootId,
    relPath: target.relPath,
    name: customName !== '' ? customName : target.suggestedName,
  })
}

watch(
  () => mountsStore.version,
  () => {
    if (prepareTab.value === 'mount') {
      void ensureMountRootsLoaded()
    }
  },
)

watch(
  () => props.launchSource?.nonce,
  (nonce) => {
    const src = props.launchSource
    if (nonce === undefined || !src || lastHandledNonce.value === nonce) return
    lastHandledNonce.value = nonce
    void prepareFromMountSource({
      rootId: src.rootId,
      relPath: src.relPath,
      name: stripArchiveSuffix(src.name),
    })
  },
  { immediate: true },
)

async function handleStart(inst: WargameInstance): Promise<void> {
  const draft = readDraft(inst)
  if (inst.mode !== 'compose' && (draft.containerPort <= 0 || draft.containerPort > 65535)) {
    errorMessage.value = '컨테이너 내부 포트(1~65535)를 올바르게 입력하세요.'
    return
  }
  const effectiveName = draft.name.trim() !== '' ? draft.name.trim() : inst.name
  const effectiveTemplate: WargameTemplateKind =
    inst.mode === 'template' ? (draft.templateKind || 'pwn-socat') : inst.templateKind

  setBusy(inst.id, 'starting')
  errorMessage.value = ''
  try {
    const updated = await api.startWargame({
      id: inst.id,
      name: effectiveName,
      templateKind: effectiveTemplate,
      containerPort: inst.mode === 'compose' ? 0 : draft.containerPort,
      connKind: draft.connKind,
      relaxSecurity: draft.relaxSecurity,
    })
    upsertInstance(updated, true)
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '워게임 빌드/실행 요청에 실패했습니다.'
    void refreshWargames(true)
  } finally {
    setBusy(inst.id, null)
  }
}

async function handleReset(id: string): Promise<void> {
  setBusy(id, 'resetting')
  errorMessage.value = ''
  try {
    const updated = await api.resetWargame(id)
    upsertInstance(updated)
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '컨테이너 리셋에 실패했습니다.'
  } finally {
    setBusy(id, null)
  }
}

async function handleStop(id: string): Promise<void> {
  setBusy(id, 'stopping')
  errorMessage.value = ''
  try {
    const updated = await api.stopWargame(id)
    upsertInstance(updated, true)
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '컨테이너 일시 중지에 실패했습니다.'
  } finally {
    setBusy(id, null)
  }
}

async function handleExtend(id: string): Promise<void> {
  setBusy(id, 'extending')
  errorMessage.value = ''
  try {
    const updated = await api.extendWargame(id)
    upsertInstance(updated)
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '만료 시간 연장에 실패했습니다.'
  } finally {
    setBusy(id, null)
  }
}

async function handleDelete(id: string): Promise<void> {
  setBusy(id, 'deleting')
  errorMessage.value = ''
  try {
    await api.deleteWargame(id)
    confirmDeleteId.value = null
    if (logsDrawer.value.kind === 'open' && logsDrawer.value.instanceId === id) {
      closeLogs()
    }
    const nextList = instances.value.filter((item) => item.id !== id)
    syncDraftsForList(nextList)
    instances.value = nextList
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '인스턴스 삭제에 실패했습니다.'
  } finally {
    setBusy(id, null)
  }
}

async function pollLogsOnce(instanceId: string, instanceName: string, initial = false): Promise<void> {
  try {
    const res = await api.fetchWargameLogs(instanceId)
    if (logsDrawer.value.kind !== 'open' || logsDrawer.value.instanceId !== instanceId) return
    logsDrawer.value = {
      kind: 'open',
      instanceId,
      instanceName,
      buildLog: res.buildLog,
      containerLog: res.containerLog,
      status: res.status,
      loading: false,
      error: '',
    }
  } catch (err) {
    if (logsDrawer.value.kind !== 'open' || logsDrawer.value.instanceId !== instanceId) return
    logsDrawer.value = {
      ...logsDrawer.value,
      loading: initial ? false : logsDrawer.value.loading,
      error: err instanceof Error ? err.message : '로그를 불러오지 못했습니다.',
    }
  }
}

function stopLogsPolling(): void {
  if (logsPollTimer !== null) {
    clearInterval(logsPollTimer)
    logsPollTimer = null
  }
}

function openLogs(inst: WargameInstance): void {
  stopLogsPolling()
  logsDrawer.value = {
    kind: 'open',
    instanceId: inst.id,
    instanceName: inst.name,
    buildLog: '',
    containerLog: '',
    status: inst.status,
    loading: true,
    error: '',
  }
  void pollLogsOnce(inst.id, inst.name, true)
  logsPollTimer = setInterval(() => {
    void pollLogsOnce(inst.id, inst.name, false)
  }, 1500)
}

function closeLogs(): void {
  stopLogsPolling()
  logsDrawer.value = { kind: 'closed' }
}

function effectiveBindIp(inst: WargameInstance): string {
  if (inst.bindIp !== '') return inst.bindIp
  if (defaultBindIp.value !== '') return defaultBindIp.value
  return window.location.hostname || '127.0.0.1'
}

function buildNcCommand(inst: WargameInstance, port: WargamePortBinding): string {
  return `nc ${effectiveBindIp(inst)} ${port.hostPort}`
}

function buildHttpUrl(inst: WargameInstance, port: WargamePortBinding): string {
  return `http://${effectiveBindIp(inst)}:${port.hostPort}`
}

async function copyConnectionText(key: string, text: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(text)
    copiedKey.value = key
    setTimeout(() => {
      if (copiedKey.value === key) {
        copiedKey.value = ''
      }
    }, 1800)
  } catch {
    errorMessage.value = '클립보드 복사에 실패했습니다.'
  }
}

function statusLabel(status: WargameStatus): string {
  switch (status) {
    case 'detected':
      return '준비됨'
    case 'building':
      return '빌드 중'
    case 'running':
      return '실행 중'
    case 'stopped':
      return '중지됨'
    case 'failed':
      return '실패'
    default: {
      const _exhaustive: never = status
      return _exhaustive
    }
  }
}

function statusBadgeClass(status: WargameStatus): string {
  switch (status) {
    case 'detected':
      return 'border-sky-400/30 bg-sky-500/15 text-sky-300'
    case 'building':
      return 'border-amber-400/30 bg-amber-500/15 text-amber-200'
    case 'running':
      return 'border-emerald-400/30 bg-emerald-500/15 text-emerald-300'
    case 'stopped':
      return 'border-slate-400/30 bg-slate-500/15 text-slate-300'
    case 'failed':
      return 'border-rose-400/30 bg-rose-500/15 text-rose-300'
    default: {
      const _exhaustive: never = status
      return _exhaustive
    }
  }
}

function modeLabel(mode: WargameMode): string {
  switch (mode) {
    case 'compose':
      return 'Docker Compose'
    case 'dockerfile':
      return 'Dockerfile'
    case 'template':
      return '자동 템플릿'
    default: {
      const _exhaustive: never = mode
      return _exhaustive
    }
  }
}

function isEditableStatus(status: WargameStatus): boolean {
  return status === 'detected' || status === 'stopped' || status === 'failed'
}

function remainingSeconds(expiresAt: number): number {
  if (expiresAt <= 0) return 0
  return Math.max(0, expiresAt - nowSec.value)
}

onMounted(() => {
  void refreshWargames()
  clockTimer = setInterval(() => {
    nowSec.value = Math.floor(Date.now() / 1000)
  }, 1000)
})

onUnmounted(() => {
  if (clockTimer !== null) clearInterval(clockTimer)
  if (buildPollTimer !== null) clearInterval(buildPollTimer)
  stopLogsPolling()
})
</script>

<template>
  <div class="relative flex h-full w-full flex-col overflow-y-auto p-4 text-xs text-slate-200">
    <div
      v-if="loadedOnce && !dockerAvailable"
      class="mb-4 flex items-start gap-2.5 rounded-2xl border border-amber-400/35 bg-amber-500/15 px-4 py-3 text-amber-100"
    >
      <AlertTriangle class="mt-0.5 h-4 w-4 shrink-0 text-amber-300" />
      <div class="min-w-0 flex-1">
        <div class="font-semibold text-amber-200">Docker 데몬에 연결할 수 없습니다</div>
        <p class="mt-0.5 break-all font-mono text-[11px] text-amber-200/85">
          {{ dockerError || '호스트에서 Docker 서비스 상태 및 소켓 권한을 확인하세요.' }}
        </p>
      </div>
    </div>

    <div
      v-if="errorMessage"
      class="mb-4 flex items-center justify-between gap-2 rounded-xl border border-rose-500/30 bg-rose-500/15 px-3.5 py-2.5 text-rose-200"
    >
      <span>{{ errorMessage }}</span>
      <button
        type="button"
        class="rounded p-0.5 text-rose-300 hover:bg-white/10"
        @click="errorMessage = ''"
      >
        <X class="h-3.5 w-3.5" />
      </button>
    </div>

    <section class="mb-4 rounded-2xl border border-white/10 bg-slate-950/60 p-4">
      <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
        <div class="flex items-center gap-1.5 rounded-xl bg-slate-900 p-1">
          <button
            type="button"
            class="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 font-semibold transition"
            :class="
              prepareTab === 'upload'
                ? 'bg-violet-600 text-white shadow'
                : 'text-slate-400 hover:text-slate-200'
            "
            @click="switchPrepareTab('upload')"
          >
            <Upload class="h-3.5 w-3.5" />
            압축 파일 업로드
          </button>
          <button
            type="button"
            class="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 font-semibold transition"
            :class="
              prepareTab === 'mount'
                ? 'bg-violet-600 text-white shadow'
                : 'text-slate-400 hover:text-slate-200'
            "
            @click="switchPrepareTab('mount')"
          >
            <HardDrive class="h-3.5 w-3.5" />
            마운트 폴더/파일 선택
          </button>
        </div>

        <div class="flex items-center gap-2">
          <input
            v-model="prepareName"
            type="text"
            placeholder="표시 이름 (선택)"
            class="w-44 rounded-xl border border-white/10 bg-slate-900 px-3 py-1.5 text-xs text-white focus:border-violet-400 focus:outline-none"
          />
        </div>
      </div>

      <div
        v-if="prepareTab === 'upload'"
        class="flex flex-col items-center justify-center rounded-xl border-2 border-dashed px-4 py-6 text-center transition"
        :class="
          dragOverUpload
            ? 'border-violet-400 bg-violet-500/15'
            : 'border-white/15 bg-slate-900/50 hover:border-violet-400/40'
        "
        @dragover.prevent="dragOverUpload = true"
        @dragleave.prevent="dragOverUpload = false"
        @drop.prevent="handleUploadDrop"
      >
        <input
          ref="archiveInputRef"
          type="file"
          accept=".zip,.tar.gz,.tgz,.tar.xz,.tar"
          class="hidden"
          @change="handleArchiveInputChange"
        />
        <FolderArchive class="mb-2 h-7 w-7 text-violet-400" />
        <p class="mb-1 text-xs font-medium text-slate-200">
          워게임 문제 압축 파일을 여기에 놓거나 선택하세요
        </p>
        <p class="mb-3 text-[11px] text-slate-400">
          지원 형식: .zip, .tar.gz, .tgz, .tar.xz, .tar (docker-compose / Dockerfile / 단일 바이너리·웹 자동 감지)
        </p>
        <button
          type="button"
          :disabled="preparing"
          class="inline-flex items-center gap-1.5 rounded-xl bg-violet-600 px-4 py-2 font-semibold text-white hover:bg-violet-500 disabled:opacity-50"
          @click="archiveInputRef?.click()"
        >
          <RefreshCw v-if="preparing" class="h-3.5 w-3.5 animate-spin" />
          <Upload v-else class="h-3.5 w-3.5" />
          {{ preparing ? '압축 해제 및 구조 분석 중...' : '압축 파일 선택' }}
        </button>
      </div>

      <div v-else class="space-y-2.5">
        <div class="flex flex-wrap items-center gap-1.5">
          <button
            v-for="root in mountRoots"
            :key="root.id"
            type="button"
            class="inline-flex items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-[11px] font-semibold transition"
            :class="
              mountRootId === root.id
                ? 'bg-violet-500/25 text-violet-200 ring-1 ring-violet-400/40'
                : 'bg-slate-900 text-slate-300 hover:bg-white/10'
            "
            @click="loadMountDir(root.id, '')"
          >
            <HardDrive class="h-3.5 w-3.5" />
            {{ root.name }}
          </button>
          <span v-if="mountRoots.length === 0" class="text-[11px] text-slate-500">
            연결된 마운트 폴더가 없습니다.
          </span>
        </div>

        <div
          v-if="mountRootId"
          class="flex items-center gap-1 overflow-x-auto rounded-lg border border-white/5 bg-slate-900/70 px-2.5 py-1.5 text-[11px]"
        >
          <button
            type="button"
            class="rounded px-1.5 py-0.5 font-semibold text-violet-300 hover:bg-white/5"
            @click="loadMountDir(mountRootId, '')"
          >
            {{ mountRoots.find((r) => r.id === mountRootId)?.name ?? mountRootId }}
          </button>
          <template v-for="crumb in mountBreadcrumbs" :key="crumb.relPath">
            <ChevronRight class="h-3 w-3 shrink-0 text-slate-500" />
            <button
              type="button"
              class="rounded px-1.5 py-0.5 text-slate-200 hover:bg-white/5"
              @click="loadMountDir(mountRootId, crumb.relPath)"
            >
              {{ crumb.name }}
            </button>
          </template>
        </div>

        <div class="max-h-40 overflow-y-auto rounded-xl border border-white/10 bg-slate-900/60">
          <div v-if="mountLoading" class="py-6 text-center text-slate-400">폴더 내용을 불러오는 중...</div>
          <div
            v-else-if="mountSelectableEntries.length === 0"
            class="py-6 text-center text-[11px] text-slate-500"
          >
            하위 폴더나 압축 파일이 없습니다. 아래 버튼으로 현재 폴더 자체를 바로 준비할 수 있습니다.
          </div>
          <div
            v-for="entry in mountSelectableEntries"
            :key="entry.relPath"
            class="flex cursor-pointer select-none items-center justify-between gap-2 border-b border-white/5 px-3 py-1.5 last:border-b-0"
            :class="
              mountSelectedRelPath === entry.relPath
                ? 'bg-violet-500/25 text-white'
                : 'hover:bg-white/5'
            "
            @click="handleMountEntryClick(entry)"
            @dblclick="handleMountEntryDblClick(entry)"
          >
            <div class="flex min-w-0 items-center gap-2">
              <Folder v-if="entry.isDir" class="h-4 w-4 shrink-0 text-amber-400" />
              <FolderArchive v-else class="h-4 w-4 shrink-0 text-orange-400" />
              <span class="truncate font-medium">{{ entry.name }}</span>
            </div>
            <div class="flex shrink-0 items-center gap-2 text-[11px] text-slate-400">
              <span v-if="!entry.isDir" class="tabular-nums">{{ formatBytes(entry.size) }}</span>
              <button
                v-if="entry.isDir"
                type="button"
                class="inline-flex items-center gap-0.5 rounded bg-white/10 px-2 py-0.5 text-[10px] font-medium text-slate-200 hover:bg-white/20"
                @click.stop="loadMountDir(mountRootId, entry.relPath)"
              >
                이동
                <ChevronRight class="h-3 w-3" />
              </button>
            </div>
          </div>
        </div>

        <div class="flex flex-wrap items-center justify-between gap-2 pt-1">
          <div class="min-w-0 text-[11px] text-slate-300">
            <span class="text-slate-400">선택 대상: </span>
            <span class="font-mono text-violet-200">
              {{ activeMountTarget ? activeMountTarget.displayPath : '-' }}
            </span>
            <button
              v-if="mountSelectedRelPath !== null"
              type="button"
              class="ml-2 text-sky-300 underline hover:text-sky-200"
              @click="mountSelectedRelPath = null"
            >
              현재 폴더로 전환
            </button>
          </div>

          <button
            type="button"
            :disabled="!activeMountTarget || preparing"
            class="inline-flex items-center gap-1.5 rounded-xl bg-violet-600 px-3.5 py-2 font-semibold text-white hover:bg-violet-500 disabled:opacity-50"
            @click="handlePrepareSelectedMount"
          >
            <RefreshCw v-if="preparing" class="h-3.5 w-3.5 animate-spin" />
            <Cpu v-else class="h-3.5 w-3.5" />
            {{ preparing ? '분석 중...' : '이 경로로 워게임 준비' }}
          </button>
        </div>
      </div>
    </section>

    <section class="flex-1 space-y-3">
      <div class="flex items-center justify-between px-1">
        <div class="flex items-center gap-2">
          <h3 class="text-sm font-semibold text-white">워게임 인스턴스 ({{ instances.length }})</h3>
          <span v-if="defaultBindIp" class="font-mono text-[11px] text-slate-400">
            바인딩 IP {{ defaultBindIp }}
          </span>
        </div>
        <button
          type="button"
          class="rounded-lg p-1.5 text-slate-400 hover:bg-white/10 hover:text-white"
          title="목록 새로고침"
          @click="refreshWargames()"
        >
          <RefreshCw class="h-3.5 w-3.5" :class="{ 'animate-spin': loadingList }" />
        </button>
      </div>

      <div
        v-if="instances.length === 0 && !loadingList"
        class="flex flex-col items-center justify-center rounded-2xl border border-white/10 bg-slate-950/40 py-12 text-center text-slate-400"
      >
        <Cpu class="mb-2.5 h-8 w-8 text-slate-600" />
        <p class="font-medium text-slate-300">준비된 워게임 인스턴스가 없습니다</p>
        <p class="mt-1 text-[11px] text-slate-500">
          상단에서 압축 파일을 업로드하거나 File Station에서 폴더/압축 파일을 선택해 워게임 실행을 누르세요.
        </p>
      </div>

      <article
        v-for="inst in instances"
        :key="inst.id"
        class="space-y-3 rounded-2xl border border-white/10 bg-slate-950/75 p-4 shadow-lg"
      >
        <div class="flex flex-wrap items-start justify-between gap-2">
          <div class="min-w-0">
            <div class="flex flex-wrap items-center gap-2">
              <h4 class="truncate text-sm font-semibold text-white">{{ inst.name }}</h4>
              <span
                class="inline-flex items-center gap-1 rounded-full border px-2.5 py-0.5 text-[11px] font-semibold"
                :class="statusBadgeClass(inst.status)"
              >
                <RefreshCw v-if="inst.status === 'building'" class="h-3 w-3 animate-spin" />
                {{ statusLabel(inst.status) }}
              </span>
              <span class="rounded-md bg-white/10 px-2 py-0.5 text-[10px] font-medium text-slate-300">
                {{ modeLabel(inst.mode) }}
              </span>
              <span
                v-if="inst.relaxSecurity"
                class="rounded-md bg-amber-500/15 px-2 py-0.5 text-[10px] font-semibold text-amber-300"
              >
                권한 제한 완화
              </span>
            </div>
            <div class="mt-1 flex flex-wrap items-center gap-3 font-mono text-[11px] text-slate-400">
              <span>ID: {{ inst.id }}</span>
              <span v-if="inst.composeFile">Compose: {{ inst.composeFile }}</span>
              <span v-if="inst.entrypointFile">엔트리: {{ inst.entrypointFile }}</span>
            </div>
          </div>

          <div v-if="inst.status === 'running'" class="flex flex-wrap items-center gap-2">
            <span
              class="rounded-lg border border-emerald-400/25 bg-emerald-500/10 px-2.5 py-1 font-mono text-xs font-semibold tabular-nums text-emerald-200"
            >
              TTL {{ formatDuration(remainingSeconds(inst.expiresAt)) }}
            </span>
            <button
              type="button"
              :disabled="isBusy(inst.id)"
              class="inline-flex items-center gap-1 rounded-lg border border-sky-400/30 bg-sky-500/15 px-2.5 py-1 font-semibold text-sky-200 hover:bg-sky-500/25 disabled:opacity-50"
              @click="handleExtend(inst.id)"
            >
              <Plus class="h-3.5 w-3.5" />
              +1시간 연장
            </button>
          </div>
        </div>

        <div
          v-if="inst.warnings.length > 0"
          class="space-y-1 rounded-xl border border-amber-400/30 bg-amber-500/10 px-3 py-2 text-[11px] text-amber-200"
        >
          <div v-for="(warn, wIdx) in inst.warnings" :key="wIdx" class="flex items-start gap-1.5">
            <AlertTriangle class="mt-0.5 h-3.5 w-3.5 shrink-0 text-amber-300" />
            <span>{{ warn }}</span>
          </div>
        </div>

        <div
          v-if="inst.lastError !== ''"
          class="rounded-xl border border-rose-500/35 bg-rose-500/15 px-3 py-2 font-mono text-[11px] whitespace-pre-wrap break-all text-rose-200"
        >
          {{ inst.lastError }}
        </div>

        <div
          v-if="isEditableStatus(inst.status)"
          class="space-y-3 rounded-xl border border-white/10 bg-slate-900/80 p-3"
        >
          <div class="grid grid-cols-1 gap-2.5 sm:grid-cols-2 lg:grid-cols-4">
            <div>
              <label class="mb-1 block text-[11px] text-slate-400">인스턴스 이름</label>
              <input
                :value="readDraft(inst).name"
                type="text"
                class="w-full rounded-lg border border-white/10 bg-slate-950 px-2.5 py-1.5 text-xs text-white focus:border-violet-400 focus:outline-none"
                @input="handleDraftNameInput(inst, $event)"
              />
            </div>

            <div v-if="inst.mode === 'template'">
              <label class="mb-1 block text-[11px] text-slate-400">실행 템플릿</label>
              <select
                :value="readDraft(inst).templateKind || 'pwn-socat'"
                class="w-full rounded-lg border border-white/10 bg-slate-950 px-2.5 py-1.5 text-xs text-white focus:border-violet-400 focus:outline-none"
                @change="handleDraftTemplateChange(inst, $event)"
              >
                <option v-for="opt in TEMPLATE_OPTIONS" :key="opt.value" :value="opt.value">
                  {{ opt.label }}
                </option>
              </select>
            </div>

            <div v-if="inst.mode !== 'compose'">
              <label class="mb-1 block text-[11px] text-slate-400">컨테이너 포트</label>
              <input
                :value="readDraft(inst).containerPort"
                type="number"
                min="1"
                max="65535"
                class="w-full rounded-lg border border-white/10 bg-slate-950 px-2.5 py-1.5 font-mono text-xs text-white focus:border-violet-400 focus:outline-none"
                @input="handleDraftPortInput(inst, $event)"
              />
            </div>

            <div v-if="inst.mode !== 'compose'">
              <label class="mb-1 block text-[11px] text-slate-400">접속 방식</label>
              <select
                :value="readDraft(inst).connKind"
                class="w-full rounded-lg border border-white/10 bg-slate-950 px-2.5 py-1.5 text-xs text-white focus:border-violet-400 focus:outline-none"
                @change="handleDraftConnChange(inst, $event)"
              >
                <option value="nc">Netcat (nc)</option>
                <option value="http">웹 브라우저 (HTTP)</option>
              </select>
            </div>
          </div>

          <div class="flex flex-wrap items-center justify-between gap-3 pt-1">
            <label class="inline-flex items-center gap-2 text-xs text-slate-300">
              <input
                :checked="readDraft(inst).relaxSecurity"
                type="checkbox"
                class="rounded border-white/20 bg-slate-950"
                @change="handleDraftRelaxChange(inst, $event)"
              />
              권한 제한 완화 (SETUID/root 필요 시)
            </label>

            <div class="flex flex-wrap items-center gap-1.5">
              <button
                v-if="inst.status === 'stopped' || inst.status === 'failed'"
                type="button"
                class="inline-flex items-center gap-1 rounded-lg bg-white/5 px-2.5 py-1.5 text-slate-200 hover:bg-white/10"
                @click="openLogs(inst)"
              >
                <FileText class="h-3.5 w-3.5 text-sky-300" />
                로그 보기
              </button>

              <a
                :href="api.wargameSourceUrl(inst.id)"
                target="_blank"
                rel="noopener noreferrer"
                class="inline-flex items-center gap-1 rounded-lg bg-white/5 px-2.5 py-1.5 text-slate-200 hover:bg-white/10"
              >
                <Download class="h-3.5 w-3.5 text-sky-300" />
                원본 ZIP
              </a>

              <template v-if="confirmDeleteId === inst.id">
                <button
                  type="button"
                  :disabled="isBusy(inst.id)"
                  class="rounded-lg bg-rose-600 px-2.5 py-1.5 font-semibold text-white hover:bg-rose-500 disabled:opacity-50"
                  @click="handleDelete(inst.id)"
                >
                  삭제 확인
                </button>
                <button
                  type="button"
                  class="rounded-lg bg-white/5 px-2.5 py-1.5 text-slate-300 hover:bg-white/10"
                  @click="confirmDeleteId = null"
                >
                  취소
                </button>
              </template>
              <button
                v-else
                type="button"
                :disabled="isBusy(inst.id)"
                class="inline-flex items-center gap-1 rounded-lg bg-rose-500/15 px-2.5 py-1.5 text-rose-300 hover:bg-rose-500/25 disabled:opacity-50"
                @click="confirmDeleteId = inst.id"
              >
                <Trash2 class="h-3.5 w-3.5" />
                삭제
              </button>

              <button
                type="button"
                :disabled="isBusy(inst.id) || !dockerAvailable"
                class="inline-flex items-center gap-1.5 rounded-xl bg-emerald-600 px-3.5 py-1.5 font-semibold text-white hover:bg-emerald-500 disabled:opacity-50"
                @click="handleStart(inst)"
              >
                <Play class="h-3.5 w-3.5" />
                빌드 &amp; 실행
              </button>
            </div>
          </div>
        </div>

        <div
          v-else-if="inst.status === 'building'"
          class="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-amber-400/25 bg-amber-500/10 px-3.5 py-2.5"
        >
          <div class="flex items-center gap-2 text-amber-200">
            <RefreshCw class="h-4 w-4 animate-spin text-amber-300" />
            <span>Docker 이미지 빌드 및 컨테이너 기동 중입니다...</span>
          </div>
          <div class="flex items-center gap-1.5">
            <button
              type="button"
              class="inline-flex items-center gap-1 rounded-lg bg-white/10 px-2.5 py-1.5 font-medium text-white hover:bg-white/15"
              @click="openLogs(inst)"
            >
              <FileText class="h-3.5 w-3.5 text-amber-200" />
              로그 보기
            </button>
            <a
              :href="api.wargameSourceUrl(inst.id)"
              target="_blank"
              rel="noopener noreferrer"
              class="inline-flex items-center gap-1 rounded-lg bg-white/10 px-2.5 py-1.5 text-slate-200 hover:bg-white/15"
            >
              <Download class="h-3.5 w-3.5 text-sky-300" />
              원본 ZIP
            </a>
          </div>
        </div>

        <div v-else-if="inst.status === 'running'" class="space-y-3">
          <div class="space-y-2">
            <div
              v-for="(port, pIdx) in inst.ports"
              :key="`${port.service}-${port.hostPort}-${pIdx}`"
              class="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-emerald-400/30 bg-emerald-950/40 px-3.5 py-2.5"
            >
              <div class="flex min-w-0 flex-wrap items-center gap-2.5">
                <span
                  class="inline-flex items-center gap-1 rounded-md bg-emerald-500/20 px-2 py-0.5 text-[10px] font-bold uppercase text-emerald-300"
                >
                  <Terminal v-if="port.connKind === 'nc'" class="h-3 w-3" />
                  <ExternalLink v-else class="h-3 w-3" />
                  {{ port.connKind }}
                </span>

                <code
                  v-if="port.connKind === 'nc'"
                  class="font-mono text-sm font-semibold text-white select-all"
                >
                  {{ buildNcCommand(inst, port) }}
                </code>
                <a
                  v-else
                  :href="buildHttpUrl(inst, port)"
                  target="_blank"
                  rel="noopener noreferrer"
                  class="inline-flex items-center gap-1.5 font-mono text-sm font-semibold text-sky-300 underline decoration-sky-400/50 hover:text-sky-200"
                >
                  {{ buildHttpUrl(inst, port) }}
                  <ExternalLink class="h-3.5 w-3.5 shrink-0" />
                </a>

                <span class="font-mono text-[11px] text-slate-400">
                  ({{ port.service ? `${port.service} · ` : '' }}{{ port.containerPort }}/{{ port.protocol || 'tcp' }})
                </span>
              </div>

              <button
                type="button"
                class="inline-flex items-center gap-1 rounded-lg bg-white/10 px-2.5 py-1 text-xs font-medium text-slate-100 hover:bg-white/20"
                @click="
                  copyConnectionText(
                    `${inst.id}-${port.hostPort}`,
                    port.connKind === 'nc' ? buildNcCommand(inst, port) : buildHttpUrl(inst, port),
                  )
                "
              >
                <Check
                  v-if="copiedKey === `${inst.id}-${port.hostPort}`"
                  class="h-3.5 w-3.5 text-emerald-400"
                />
                <Copy v-else class="h-3.5 w-3.5" />
                {{ copiedKey === `${inst.id}-${port.hostPort}` ? '복사됨' : '복사' }}
              </button>
            </div>
          </div>

          <div class="flex flex-wrap items-center justify-between gap-2 border-t border-white/10 pt-2.5">
            <div class="flex flex-wrap items-center gap-1.5">
              <button
                type="button"
                :disabled="isBusy(inst.id)"
                class="inline-flex items-center gap-1.5 rounded-lg bg-sky-600/90 px-3 py-1.5 font-semibold text-white hover:bg-sky-500 disabled:opacity-50"
                @click="handleReset(inst.id)"
              >
                <RotateCcw class="h-3.5 w-3.5" />
                리셋 (1초 초기화)
              </button>

              <button
                type="button"
                :disabled="isBusy(inst.id)"
                class="inline-flex items-center gap-1.5 rounded-lg bg-amber-500/20 px-3 py-1.5 font-semibold text-amber-200 hover:bg-amber-500/30 disabled:opacity-50"
                @click="handleStop(inst.id)"
              >
                <Square class="h-3.5 w-3.5" />
                일시 중지 (이미지 유지)
              </button>

              <template v-if="confirmDeleteId === inst.id">
                <button
                  type="button"
                  :disabled="isBusy(inst.id)"
                  class="rounded-lg bg-rose-600 px-3 py-1.5 font-semibold text-white hover:bg-rose-500 disabled:opacity-50"
                  @click="handleDelete(inst.id)"
                >
                  삭제 확인
                </button>
                <button
                  type="button"
                  class="rounded-lg bg-white/5 px-2.5 py-1.5 text-slate-300 hover:bg-white/10"
                  @click="confirmDeleteId = null"
                >
                  취소
                </button>
              </template>
              <button
                v-else
                type="button"
                :disabled="isBusy(inst.id)"
                class="inline-flex items-center gap-1.5 rounded-lg bg-rose-600/20 px-3 py-1.5 font-semibold text-rose-300 hover:bg-rose-600/30 disabled:opacity-50"
                @click="confirmDeleteId = inst.id"
              >
                <Trash2 class="h-3.5 w-3.5" />
                종료 및 이미지 삭제
              </button>
            </div>

            <div class="flex flex-wrap items-center gap-1.5">
              <button
                type="button"
                class="inline-flex items-center gap-1 rounded-lg bg-white/5 px-2.5 py-1.5 text-slate-200 hover:bg-white/10"
                @click="openLogs(inst)"
              >
                <FileText class="h-3.5 w-3.5 text-sky-300" />
                로그 보기
              </button>

              <a
                :href="api.wargameSourceUrl(inst.id)"
                target="_blank"
                rel="noopener noreferrer"
                class="inline-flex items-center gap-1 rounded-lg bg-white/5 px-2.5 py-1.5 text-slate-200 hover:bg-white/10"
              >
                <Download class="h-3.5 w-3.5 text-sky-300" />
                원본 ZIP
              </a>
            </div>
          </div>
        </div>
      </article>
    </section>

    <div
      v-if="logsDrawer.kind === 'open'"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/65 p-4"
      @click.self="closeLogs"
    >
      <div class="flex max-h-[85vh] w-full max-w-3xl flex-col rounded-2xl border border-white/15 bg-slate-950 p-4 text-slate-100 shadow-2xl">
        <div class="mb-3 flex items-center justify-between border-b border-white/10 pb-2.5">
          <div class="flex items-center gap-2">
            <Terminal class="h-4 w-4 text-violet-400" />
            <h4 class="text-sm font-semibold text-white">로그 · {{ logsDrawer.instanceName }}</h4>
            <span
              class="rounded-full border px-2 py-0.5 text-[10px] font-semibold"
              :class="statusBadgeClass(logsDrawer.status)"
            >
              {{ statusLabel(logsDrawer.status) }}
            </span>
          </div>
          <button
            type="button"
            class="rounded-lg p-1 text-slate-400 hover:bg-white/10 hover:text-white"
            @click="closeLogs"
          >
            <X class="h-4 w-4" />
          </button>
        </div>

        <p v-if="logsDrawer.error" class="mb-2 text-xs text-rose-300">{{ logsDrawer.error }}</p>

        <div class="grid min-h-0 flex-1 grid-cols-1 gap-3 overflow-y-auto lg:grid-cols-2">
          <div class="flex min-h-56 flex-col rounded-xl border border-white/10 bg-black/70">
            <div class="border-b border-white/10 px-3 py-1.5 text-[11px] font-semibold text-slate-300">
              빌드 로그 (buildLog)
            </div>
            <pre
              class="flex-1 overflow-auto p-3 font-mono text-[11px] leading-relaxed whitespace-pre-wrap break-all text-slate-200"
            >{{ logsDrawer.buildLog || (logsDrawer.loading ? '불러오는 중...' : '(출력 없음)') }}</pre>
          </div>

          <div class="flex min-h-56 flex-col rounded-xl border border-white/10 bg-black/70">
            <div class="border-b border-white/10 px-3 py-1.5 text-[11px] font-semibold text-slate-300">
              컨테이너 출력 (containerLog)
            </div>
            <pre
              class="flex-1 overflow-auto p-3 font-mono text-[11px] leading-relaxed whitespace-pre-wrap break-all text-emerald-200"
            >{{ logsDrawer.containerLog || (logsDrawer.loading ? '불러오는 중...' : '(출력 없음)') }}</pre>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
