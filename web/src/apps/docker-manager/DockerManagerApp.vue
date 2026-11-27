<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import {
  AlertTriangle,
  Box,
  Check,
  Copy,
  Download,
  FileCode2,
  FileText,
  HardDrive,
  Info,
  Layers,
  LayoutDashboard,
  Network,
  Play,
  Plus,
  RefreshCw,
  RotateCcw,
  Save,
  Search,
  Square,
  Terminal,
  Trash2,
  X,
} from 'lucide-vue-next'
import { EditorState } from '@codemirror/state'
import { EditorView, keymap } from '@codemirror/view'
import { basicSetup } from 'codemirror'
import { oneDark } from '@codemirror/theme-one-dark'
import { yaml } from '@codemirror/lang-yaml'
import { api } from '../../api/client'
import { useDesktopStore } from '../../stores/desktop'
import type {
  DockerContainer,
  DockerContainerInspect,
  DockerImage,
  DockerNetwork,
  DockerOverview,
  DockerStack,
  DockerVolume,
} from '../../types/api'

type DockerSection = 'overview' | 'containers' | 'stacks' | 'images' | 'resources'
type ContainerStateFilter = 'all' | 'running' | 'stopped'

type LogsModalState =
  | { readonly kind: 'closed' }
  | {
      readonly kind: 'open'
      readonly containerId: string
      readonly containerName: string
      readonly tail: number
      readonly logs: string
      readonly loading: boolean
      readonly error: string
    }

type InspectModalState =
  | { readonly kind: 'closed' }
  | {
      readonly kind: 'open'
      readonly containerId: string
      readonly containerName: string
      readonly loading: boolean
      readonly error: string
      readonly data: DockerContainerInspect | null
    }

type StackEditorMode =
  | { readonly kind: 'none' }
  | { readonly kind: 'create'; readonly name: string }
  | { readonly kind: 'edit'; readonly name: string; readonly status: string }

const DEFAULT_COMPOSE_TEMPLATE = `services:
  app:
    image: nginx:alpine
    restart: unless-stopped
    ports:
      - "8080:80"
`

const PROTECTED_NETWORKS: ReadonlySet<string> = new Set(['bridge', 'host', 'none'])

const desktopStore = useDesktopStore()

const activeSection = ref<DockerSection>('overview')
const loading = ref(false)
const errorMessage = ref('')
const noticeMessage = ref('')

const overview = ref<DockerOverview | null>(null)
const containers = ref<readonly DockerContainer[]>([])
const images = ref<readonly DockerImage[]>([])
const stacks = ref<readonly DockerStack[]>([])
const volumes = ref<readonly DockerVolume[]>([])
const networks = ref<readonly DockerNetwork[]>([])

const containerQuery = ref('')
const containerStateFilter = ref<ContainerStateFilter>('all')
const busyMap = ref<Record<string, boolean>>({})
const confirmDeleteKey = ref<string | null>(null)

const logsModal = ref<LogsModalState>({ kind: 'closed' })
const inspectModal = ref<InspectModalState>({ kind: 'closed' })
const copiedLogs = ref(false)

const pullImageInput = ref('')
const pullingImage = ref(false)
const pruningImages = ref(false)
const imageCommandOutput = ref('')
const overviewPruneOutput = ref('')

const stackEditorMode = ref<StackEditorMode>({ kind: 'none' })
const stackEditorHostRef = ref<HTMLDivElement | null>(null)
const stackDirty = ref(false)
const stackBusy = ref(false)
const stackOutput = ref('')
const stackOutputTitle = ref('')
const confirmDeleteStack = ref<string | null>(null)

let stackEditorView: EditorView | null = null
let stackSavedBaseline = ''
let noticeTimer: ReturnType<typeof setTimeout> | null = null

const NAV_ITEMS: readonly {
  readonly id: DockerSection
  readonly label: string
}[] = [
  { id: 'overview', label: '개요' },
  { id: 'containers', label: '컨테이너' },
  { id: 'stacks', label: '스택 (Compose)' },
  { id: 'images', label: '이미지' },
  { id: 'resources', label: '볼륨 · 네트워크' },
]

const filteredContainers = computed<readonly DockerContainer[]>(() => {
  const q = containerQuery.value.trim().toLowerCase()
  return containers.value.filter((item) => {
    const running = item.state.toLowerCase() === 'running'
    if (containerStateFilter.value === 'running' && !running) return false
    if (containerStateFilter.value === 'stopped' && running) return false
    if (q === '') return true
    return (
      item.name.toLowerCase().includes(q) ||
      item.image.toLowerCase().includes(q) ||
      item.id.toLowerCase().includes(q) ||
      item.ports.toLowerCase().includes(q)
    )
  })
})

function showNotice(message: string): void {
  noticeMessage.value = message
  if (noticeTimer) clearTimeout(noticeTimer)
  noticeTimer = setTimeout(() => {
    noticeMessage.value = ''
  }, 3500)
}

function setBusy(key: string, busy: boolean): void {
  const next = { ...busyMap.value }
  if (busy) {
    next[key] = true
  } else {
    delete next[key]
  }
  busyMap.value = next
}

function isBusy(key: string): boolean {
  return busyMap.value[key] === true
}

function stateDotClass(state: string): string {
  const lower = state.toLowerCase()
  if (lower === 'running') return 'bg-[#10b981]'
  if (lower === 'paused' || lower === 'restarting') return 'bg-[#f59e0b]'
  return 'bg-[#94a3b8]'
}

function stackBadgeClass(status: string): string {
  const lower = status.toLowerCase()
  if (lower === 'running') return 'border-[#a7f3d0] bg-[#ecfdf5] text-[#047857]'
  if (lower === 'partial' || lower === 'degraded') return 'border-[#fcd34d] bg-[#fffbeb] text-[#b45309]'
  return 'border-[#cbd5e1] bg-[#f1f5f9] text-[#475569]'
}

function isProtectedNetwork(net: DockerNetwork): boolean {
  return PROTECTED_NETWORKS.has(net.name.toLowerCase())
}

async function refreshOverview(): Promise<void> {
  overview.value = await api.fetchDockerOverview()
}

async function refreshContainers(): Promise<void> {
  containers.value = await api.listDockerContainers()
}

async function refreshImages(): Promise<void> {
  images.value = await api.listDockerImages()
}

async function refreshStacks(): Promise<void> {
  stacks.value = await api.listDockerStacks()
}

async function refreshResources(): Promise<void> {
  const [volList, netList] = await Promise.all([api.listDockerVolumes(), api.listDockerNetworks()])
  volumes.value = volList
  networks.value = netList
}

async function refreshSection(section: DockerSection): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  confirmDeleteKey.value = null
  try {
    switch (section) {
      case 'overview':
        await refreshOverview()
        return
      case 'containers':
        await Promise.all([refreshOverview(), refreshContainers()])
        return
      case 'stacks':
        await Promise.all([refreshOverview(), refreshStacks()])
        return
      case 'images':
        await Promise.all([refreshOverview(), refreshImages()])
        return
      case 'resources':
        await Promise.all([refreshOverview(), refreshResources()])
        return
      default: {
        const _exhaustive: never = section
        return _exhaustive
      }
    }
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : 'Docker 정보를 불러오지 못했습니다.'
  } finally {
    loading.value = false
  }
}

function selectSection(section: DockerSection): void {
  activeSection.value = section
  void refreshSection(section)
}

async function handleContainerAction(item: DockerContainer, action: 'start' | 'stop' | 'restart'): Promise<void> {
  const key = `container:${item.id}`
  setBusy(key, true)
  errorMessage.value = ''
  try {
    await api.controlDockerContainer({ id: item.id, action })
    const label = action === 'start' ? '시작' : action === 'stop' ? '중지' : '재시작'
    showNotice(`컨테이너 '${item.name}' ${label} 완료`)
    await Promise.all([refreshOverview(), refreshContainers()])
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '컨테이너 제어에 실패했습니다.'
  } finally {
    setBusy(key, false)
  }
}

async function handleDeleteContainer(item: DockerContainer): Promise<void> {
  const key = `container:${item.id}`
  setBusy(key, true)
  errorMessage.value = ''
  confirmDeleteKey.value = null
  try {
    await api.deleteDockerContainer({ id: item.id, force: true })
    showNotice(`컨테이너 '${item.name}' 삭제 완료`)
    await Promise.all([refreshOverview(), refreshContainers()])
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '컨테이너 삭제에 실패했습니다.'
  } finally {
    setBusy(key, false)
  }
}

function openContainerShell(item: DockerContainer): void {
  desktopStore.openWindow({
    title: '터미널',
    payload: {
      kind: 'terminal',
      initialTarget: {
        kind: 'docker',
        container: item.name,
        label: '컨테이너: ' + item.name,
        nonce: Date.now(),
      },
    },
  })
}

async function openContainerLogs(item: DockerContainer, tail = 300): Promise<void> {
  copiedLogs.value = false
  logsModal.value = {
    kind: 'open',
    containerId: item.id,
    containerName: item.name,
    tail,
    logs: '',
    loading: true,
    error: '',
  }
  try {
    const text = await api.fetchDockerContainerLogs({ id: item.id, tail })
    if (logsModal.value.kind === 'open' && logsModal.value.containerId === item.id) {
      logsModal.value = {
        ...logsModal.value,
        logs: text,
        loading: false,
      }
    }
  } catch (err) {
    if (logsModal.value.kind === 'open' && logsModal.value.containerId === item.id) {
      logsModal.value = {
        ...logsModal.value,
        loading: false,
        error: err instanceof Error ? err.message : '로그를 불러오지 못했습니다.',
      }
    }
  }
}

async function reloadLogsModal(nextTail?: number): Promise<void> {
  if (logsModal.value.kind !== 'open') return
  const targetId = logsModal.value.containerId
  const targetName = logsModal.value.containerName
  const tail = nextTail ?? logsModal.value.tail
  logsModal.value = {
    ...logsModal.value,
    tail,
    loading: true,
    error: '',
  }
  try {
    const text = await api.fetchDockerContainerLogs({ id: targetId, tail })
    if (logsModal.value.kind === 'open' && logsModal.value.containerId === targetId) {
      logsModal.value = {
        kind: 'open',
        containerId: targetId,
        containerName: targetName,
        tail,
        logs: text,
        loading: false,
        error: '',
      }
    }
  } catch (err) {
    if (logsModal.value.kind === 'open' && logsModal.value.containerId === targetId) {
      logsModal.value = {
        ...logsModal.value,
        loading: false,
        error: err instanceof Error ? err.message : '로그를 불러오지 못했습니다.',
      }
    }
  }
}

async function copyModalLogs(): Promise<void> {
  if (logsModal.value.kind !== 'open' || logsModal.value.logs === '') return
  try {
    await navigator.clipboard.writeText(logsModal.value.logs)
    copiedLogs.value = true
    setTimeout(() => {
      copiedLogs.value = false
    }, 1800)
  } catch {
    return
  }
}

async function openContainerInspect(item: DockerContainer): Promise<void> {
  inspectModal.value = {
    kind: 'open',
    containerId: item.id,
    containerName: item.name,
    loading: true,
    error: '',
    data: null,
  }
  try {
    const detail = await api.inspectDockerContainer(item.id)
    if (inspectModal.value.kind === 'open' && inspectModal.value.containerId === item.id) {
      inspectModal.value = {
        ...inspectModal.value,
        loading: false,
        data: detail,
      }
    }
  } catch (err) {
    if (inspectModal.value.kind === 'open' && inspectModal.value.containerId === item.id) {
      inspectModal.value = {
        ...inspectModal.value,
        loading: false,
        error: err instanceof Error ? err.message : '상세 정보를 불러오지 못했습니다.',
      }
    }
  }
}

async function handlePullImage(): Promise<void> {
  const name = pullImageInput.value.trim()
  if (name === '' || pullingImage.value) return
  pullingImage.value = true
  errorMessage.value = ''
  imageCommandOutput.value = ''
  try {
    const out = await api.pullDockerImage(name)
    imageCommandOutput.value = out !== '' ? out : `'${name}' 이미지를 가져왔습니다.`
    pullImageInput.value = ''
    showNotice(`이미지 '${name}' Pull 완료`)
    await Promise.all([refreshOverview(), refreshImages()])
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '이미지 Pull에 실패했습니다.'
  } finally {
    pullingImage.value = false
  }
}

async function handlePruneImages(source: 'overview' | 'images'): Promise<void> {
  if (pruningImages.value) return
  pruningImages.value = true
  errorMessage.value = ''
  try {
    const out = await api.pruneDockerImages()
    const message = out !== '' ? out : '미사용 이미지 정리를 완료했습니다.'
    if (source === 'overview') {
      overviewPruneOutput.value = message
    } else {
      imageCommandOutput.value = message
    }
    showNotice('미사용 이미지 정리 완료')
    await Promise.all([refreshOverview(), refreshImages()])
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '미사용 이미지 정리에 실패했습니다.'
  } finally {
    pruningImages.value = false
  }
}

async function handleDeleteImage(item: DockerImage): Promise<void> {
  const key = `image:${item.id}`
  setBusy(key, true)
  errorMessage.value = ''
  confirmDeleteKey.value = null
  try {
    await api.deleteDockerImage({ id: item.id, force: false })
    showNotice(`이미지 '${item.repository}:${item.tag}' 삭제 완료`)
    await Promise.all([refreshOverview(), refreshImages()])
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '이미지 삭제에 실패했습니다.'
  } finally {
    setBusy(key, false)
  }
}

async function handleDeleteVolume(vol: DockerVolume): Promise<void> {
  const key = `volume:${vol.name}`
  setBusy(key, true)
  errorMessage.value = ''
  confirmDeleteKey.value = null
  try {
    await api.deleteDockerVolume(vol.name)
    showNotice(`볼륨 '${vol.name}' 삭제 완료`)
    await Promise.all([refreshOverview(), refreshResources()])
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '볼륨 삭제에 실패했습니다.'
  } finally {
    setBusy(key, false)
  }
}

async function handleDeleteNetwork(net: DockerNetwork): Promise<void> {
  if (isProtectedNetwork(net)) return
  const key = `network:${net.id}`
  setBusy(key, true)
  errorMessage.value = ''
  confirmDeleteKey.value = null
  try {
    await api.deleteDockerNetwork(net.id)
    showNotice(`네트워크 '${net.name}' 삭제 완료`)
    await Promise.all([refreshOverview(), refreshResources()])
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '네트워크 삭제에 실패했습니다.'
  } finally {
    setBusy(key, false)
  }
}

function destroyStackEditor(): void {
  if (stackEditorView !== null) {
    stackEditorView.destroy()
    stackEditorView = null
  }
}

async function mountStackEditor(initialDoc: string): Promise<void> {
  await nextTick()
  destroyStackEditor()
  const host = stackEditorHostRef.value
  if (!host) return
  stackSavedBaseline = initialDoc
  stackDirty.value = false

  const state = EditorState.create({
    doc: initialDoc,
    extensions: [
      basicSetup,
      oneDark,
      yaml(),
      keymap.of([
        {
          key: 'Mod-s',
          run: () => {
            void handleSaveStack()
            return true
          },
        },
      ]),
      EditorView.updateListener.of((update) => {
        if (update.docChanged) {
          stackDirty.value = update.state.doc.toString() !== stackSavedBaseline
        }
      }),
      EditorView.theme({
        '&': { height: '100%', fontSize: '12.5px' },
        '.cm-scroller': {
          overflow: 'auto',
          fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
        },
      }),
    ],
  })

  stackEditorView = new EditorView({
    state,
    parent: host,
  })
}

function startNewStack(): void {
  errorMessage.value = ''
  confirmDeleteStack.value = null
  stackOutput.value = ''
  stackOutputTitle.value = ''
  stackEditorMode.value = { kind: 'create', name: '' }
  void mountStackEditor(DEFAULT_COMPOSE_TEMPLATE)
}

function handleNewStackNameInput(ev: Event): void {
  if (stackEditorMode.value.kind !== 'create') return
  if (ev.target instanceof HTMLInputElement) {
    stackEditorMode.value = { kind: 'create', name: ev.target.value }
  }
}

async function openExistingStack(stack: DockerStack): Promise<void> {
  errorMessage.value = ''
  confirmDeleteStack.value = null
  stackOutput.value = ''
  stackOutputTitle.value = ''
  stackBusy.value = true
  try {
    const detail = await api.fetchDockerStack(stack.name)
    stackEditorMode.value = { kind: 'edit', name: detail.name, status: detail.status }
    await mountStackEditor(detail.composeYaml)
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '스택 정보를 불러오지 못했습니다.'
  } finally {
    stackBusy.value = false
  }
}

function activeStackName(): string {
  if (stackEditorMode.value.kind === 'none') return ''
  return stackEditorMode.value.name.trim()
}

async function handleSaveStack(): Promise<boolean> {
  if (stackEditorMode.value.kind === 'none' || !stackEditorView) return false
  const name = activeStackName()
  if (name === '') {
    errorMessage.value = '스택 이름을 입력하세요 (영문 소문자, 숫자, 하이픈).'
    return false
  }
  const composeYaml = stackEditorView.state.doc.toString()
  stackBusy.value = true
  errorMessage.value = ''
  try {
    await api.saveDockerStack({ name, composeYaml })
    stackSavedBaseline = composeYaml
    stackDirty.value = false
    if (stackEditorMode.value.kind === 'create') {
      stackEditorMode.value = { kind: 'edit', name, status: 'stopped' }
    }
    showNotice(`스택 '${name}' 저장 완료`)
    await refreshStacks()
    return true
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '스택 저장에 실패했습니다.'
    return false
  } finally {
    stackBusy.value = false
  }
}

async function handleStackAction(action: 'up' | 'down' | 'recreate'): Promise<void> {
  if (stackEditorMode.value.kind === 'none') return
  if (stackEditorMode.value.kind === 'create' || stackDirty.value) {
    const saved = await handleSaveStack()
    if (!saved) return
  }
  const name = activeStackName()
  if (name === '') return
  stackBusy.value = true
  errorMessage.value = ''
  const titleMap: Readonly<Record<'up' | 'down' | 'recreate', string>> = {
    up: `docker compose up -d (${name})`,
    down: `docker compose down (${name})`,
    recreate: `docker compose pull && up -d --force-recreate (${name})`,
  }
  stackOutputTitle.value = titleMap[action]
  try {
    const out = await api.controlDockerStack({ name, action })
    stackOutput.value = out !== '' ? out : '명령이 정상적으로 완료되었습니다.'
    showNotice(`스택 '${name}' ${action === 'up' ? '배포' : action === 'down' ? '중지' : '재생성'} 완료`)
    await Promise.all([refreshOverview(), refreshStacks()])
    const updated = stacks.value.find((s) => s.name === name)
    if (updated && stackEditorMode.value.kind === 'edit') {
      stackEditorMode.value = { kind: 'edit', name: updated.name, status: updated.status }
    }
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '스택 실행에 실패했습니다.'
  } finally {
    stackBusy.value = false
  }
}

async function handleStackLogs(): Promise<void> {
  if (stackEditorMode.value.kind !== 'edit') return
  const name = stackEditorMode.value.name
  stackBusy.value = true
  errorMessage.value = ''
  stackOutputTitle.value = `스택 로그 (${name})`
  try {
    const out = await api.fetchDockerStackLogs({ name, tail: 200 })
    stackOutput.value = out !== '' ? out : '(출력된 로그가 없습니다)'
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '스택 로그를 불러오지 못했습니다.'
  } finally {
    stackBusy.value = false
  }
}

async function handleDeleteStack(name: string): Promise<void> {
  stackBusy.value = true
  errorMessage.value = ''
  confirmDeleteStack.value = null
  try {
    await api.deleteDockerStack(name)
    if (stackEditorMode.value.kind === 'edit' && stackEditorMode.value.name === name) {
      destroyStackEditor()
      stackEditorMode.value = { kind: 'none' }
      stackOutput.value = ''
      stackOutputTitle.value = ''
    }
    showNotice(`스택 '${name}' 삭제 완료`)
    await Promise.all([refreshOverview(), refreshStacks()])
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '스택 삭제에 실패했습니다.'
  } finally {
    stackBusy.value = false
  }
}

watch(activeSection, (next, prev) => {
  if (prev === 'stacks' && next !== 'stacks') {
    destroyStackEditor()
  } else if (next === 'stacks' && stackEditorMode.value.kind !== 'none') {
    void mountStackEditor(stackSavedBaseline)
  }
})

onMounted(() => {
  void refreshSection('overview')
})

onUnmounted(() => {
  destroyStackEditor()
  if (noticeTimer) clearTimeout(noticeTimer)
})
function handleLogsTailChange(ev: Event): void {
  if (ev.target instanceof HTMLSelectElement) {
    void reloadLogsModal(Number(ev.target.value))
  }
}
</script>

<template>
  <div class="relative flex h-full w-full overflow-hidden bg-white text-[12px] text-[#1e293b]">
    <aside class="flex w-44 shrink-0 flex-col border-r border-[#dce2ea] bg-[#f5f7fa]">
      <div class="border-b border-[#dce2ea] px-3 py-2.5">
        <div class="text-[12px] font-semibold text-[#1e293b]">Docker 관리</div>
        <div class="mt-0.5 text-[11px] text-[#64748b]">컨테이너 · 스택 · 이미지</div>
      </div>

      <nav class="flex-1 space-y-0.5 p-2">
        <button
          v-for="item in NAV_ITEMS"
          :key="item.id"
          type="button"
          class="flex w-full cursor-pointer items-center gap-2 rounded-[4px] px-2.5 py-2 text-left text-[12px] font-medium transition-colors"
          :class="
            activeSection === item.id
              ? 'bg-[#e6f0ff] text-[#0572ec]'
              : 'text-[#334155] hover:bg-[#e9eef5]'
          "
          @click="selectSection(item.id)"
        >
          <LayoutDashboard v-if="item.id === 'overview'" class="h-3.5 w-3.5 shrink-0" />
          <Box v-else-if="item.id === 'containers'" class="h-3.5 w-3.5 shrink-0" />
          <Layers v-else-if="item.id === 'stacks'" class="h-3.5 w-3.5 shrink-0" />
          <FileCode2 v-else-if="item.id === 'images'" class="h-3.5 w-3.5 shrink-0" />
          <HardDrive v-else class="h-3.5 w-3.5 shrink-0" />
          <span class="truncate">{{ item.label }}</span>
        </button>
      </nav>

      <div class="border-t border-[#dce2ea] px-3 py-2.5 text-[11px]">
        <div class="flex items-center gap-1.5">
          <span
            class="h-2 w-2 shrink-0 rounded-full"
            :class="overview?.available ? 'bg-[#10b981]' : 'bg-[#f59e0b]'"
          />
          <span class="truncate font-medium text-[#334155]">
            {{ overview?.available ? 'Docker Engine 연결됨' : '연결 확인 필요' }}
          </span>
        </div>
        <div class="mt-1 truncate font-mono text-[10.5px] text-[#64748b]">
          {{ overview?.serverVersion ? `v${overview.serverVersion}` : '-' }}
        </div>
      </div>
    </aside>

    <section class="flex min-w-0 flex-1 flex-col overflow-hidden bg-white">
      <header class="flex shrink-0 items-center justify-between gap-2 border-b border-[#dce2ea] bg-[#f8fafc] px-4 py-2">
        <div class="flex items-center gap-2">
          <h2 class="text-[13px] font-semibold text-[#1e293b]">
            {{ NAV_ITEMS.find((n) => n.id === activeSection)?.label }}
          </h2>
          <span
            v-if="noticeMessage"
            class="inline-flex items-center gap-1 rounded-[3px] border border-[#a7f3d0] bg-[#ecfdf5] px-2 py-0.5 text-[11px] font-medium text-[#047857]"
          >
            <Check class="h-3 w-3" />
            {{ noticeMessage }}
          </span>
        </div>

        <button
          type="button"
          class="dsm-btn"
          :disabled="loading"
          @click="refreshSection(activeSection)"
        >
          <RefreshCw class="h-3.5 w-3.5" :class="{ 'animate-spin': loading }" />
          새로고침
        </button>
      </header>

      <div
        v-if="overview && !overview.available"
        class="flex shrink-0 items-center gap-2 border-b border-[#fde68a] bg-[#fffbeb] px-4 py-2 text-[#92400e]"
      >
        <AlertTriangle class="h-4 w-4 shrink-0" />
        <span>Docker 데몬에 연결할 수 없습니다: {{ overview.error || '권한 또는 데몬 상태를 확인하세요.' }}</span>
      </div>

      <div
        v-if="errorMessage"
        class="flex shrink-0 items-center justify-between gap-2 border-b border-[#fecdd3] bg-[#fff1f2] px-4 py-2 text-[#be123c]"
      >
        <span class="truncate">{{ errorMessage }}</span>
        <button type="button" class="cursor-pointer text-[#be123c] hover:underline" @click="errorMessage = ''">
          닫기
        </button>
      </div>

      <div v-if="activeSection === 'overview'" class="min-h-0 flex-1 overflow-auto bg-[#f4f6f9] p-4">
        <div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
          <div class="rounded-[6px] border border-[#dce2ea] bg-white p-3.5">
            <div class="text-[11px] font-medium text-[#64748b]">실행 중 컨테이너</div>
            <div class="mt-1.5 flex items-baseline gap-1.5">
              <span class="text-[22px] font-bold tabular-nums text-[#059669]">
                {{ overview?.containersRunning ?? 0 }}
              </span>
              <span class="text-[11px] text-[#64748b]">개</span>
            </div>
          </div>

          <div class="rounded-[6px] border border-[#dce2ea] bg-white p-3.5">
            <div class="text-[11px] font-medium text-[#64748b]">중지됨</div>
            <div class="mt-1.5 flex items-baseline gap-1.5">
              <span class="text-[22px] font-bold tabular-nums text-[#475569]">
                {{ overview?.containersStopped ?? 0 }}
              </span>
              <span class="text-[11px] text-[#64748b]">개</span>
            </div>
          </div>

          <div class="rounded-[6px] border border-[#dce2ea] bg-white p-3.5">
            <div class="text-[11px] font-medium text-[#64748b]">이미지</div>
            <div class="mt-1.5 flex items-baseline gap-1.5">
              <span class="text-[22px] font-bold tabular-nums text-[#0572ec]">
                {{ overview?.imagesCount ?? 0 }}
              </span>
              <span class="text-[11px] text-[#64748b]">개</span>
            </div>
          </div>

          <div class="rounded-[6px] border border-[#dce2ea] bg-white p-3.5">
            <div class="text-[11px] font-medium text-[#64748b]">볼륨 · 네트워크</div>
            <div class="mt-1.5 flex items-baseline gap-1.5">
              <span class="text-[22px] font-bold tabular-nums text-[#1e293b]">
                {{ overview?.volumesCount ?? 0 }}
              </span>
              <span class="text-[11px] text-[#64748b]">/</span>
              <span class="text-[22px] font-bold tabular-nums text-[#1e293b]">
                {{ overview?.networksCount ?? 0 }}
              </span>
            </div>
          </div>
        </div>

        <div class="mt-4 overflow-hidden rounded-[6px] border border-[#dce2ea] bg-white">
          <div class="flex items-center justify-between border-b border-[#dce2ea] bg-[#f8fafc] px-3.5 py-2.5">
            <div>
              <div class="text-[12.5px] font-semibold text-[#1e293b]">디스크 사용량 (docker system df)</div>
              <div class="text-[11px] text-[#64748b]">이미지, 컨테이너, 로컬 볼륨 및 빌드 캐시 사용 현황</div>
            </div>
            <button
              type="button"
              class="dsm-btn dsm-btn-danger"
              :disabled="pruningImages || !overview?.available"
              @click="handlePruneImages('overview')"
            >
              <Trash2 class="h-3.5 w-3.5" />
              {{ pruningImages ? '정리 중...' : '미사용 이미지 정리 (Prune)' }}
            </button>
          </div>

          <table class="w-full border-collapse text-left">
            <thead>
              <tr class="border-b border-[#dce2ea] bg-[#f8fafc] text-[11px] font-semibold text-[#64748b]">
                <th class="px-3.5 py-2">항목</th>
                <th class="px-3.5 py-2 text-right">전체</th>
                <th class="px-3.5 py-2 text-right">활성</th>
                <th class="px-3.5 py-2 text-right">사용 용량</th>
                <th class="px-3.5 py-2 text-right">회수 가능</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-[#eef2f6]">
              <tr
                v-for="row in overview?.diskUsage ?? []"
                :key="row.type"
                class="h-9 hover:bg-[#f8fafc]"
              >
                <td class="px-3.5 font-medium text-[#1e293b]">{{ row.type }}</td>
                <td class="px-3.5 text-right tabular-nums text-[#475569]">{{ row.totalCount }}</td>
                <td class="px-3.5 text-right tabular-nums text-[#475569]">{{ row.active }}</td>
                <td class="px-3.5 text-right font-mono tabular-nums text-[#1e293b]">{{ row.size }}</td>
                <td class="px-3.5 text-right font-mono tabular-nums text-[#0572ec]">{{ row.reclaimable }}</td>
              </tr>
              <tr v-if="!overview || overview.diskUsage.length === 0">
                <td colspan="5" class="px-3.5 py-6 text-center text-[#64748b]">
                  디스크 사용량 정보가 없습니다.
                </td>
              </tr>
            </tbody>
          </table>

          <div v-if="overviewPruneOutput" class="border-t border-[#dce2ea] bg-[#0f172a] p-3 font-mono text-[11px] text-[#e2e8f0]">
            <pre class="max-h-36 overflow-auto whitespace-pre-wrap">{{ overviewPruneOutput }}</pre>
          </div>
        </div>
      </div>

      <div v-else-if="activeSection === 'containers'" class="flex min-h-0 flex-1 flex-col overflow-hidden">
        <div class="flex shrink-0 flex-wrap items-center justify-between gap-2 border-b border-[#dce2ea] bg-white px-4 py-2">
          <div class="flex items-center gap-2">
            <div class="relative">
              <Search class="pointer-events-none absolute top-1/2 left-2.5 h-3.5 w-3.5 -translate-y-1/2 text-[#94a3b8]" />
              <input
                v-model="containerQuery"
                type="text"
                placeholder="이름, 이미지, 포트 검색..."
                class="dsm-input w-56 pl-8"
              />
            </div>

            <div class="inline-flex overflow-hidden rounded-[4px] border border-[#c6d0dc]">
              <button
                type="button"
                class="cursor-pointer px-2.5 py-1 text-[11px] font-medium transition-colors"
                :class="containerStateFilter === 'all' ? 'bg-[#0572ec] text-white' : 'bg-white text-[#475569] hover:bg-[#f4f7fa]'"
                @click="containerStateFilter = 'all'"
              >
                전체 ({{ containers.length }})
              </button>
              <button
                type="button"
                class="cursor-pointer border-l border-[#c6d0dc] px-2.5 py-1 text-[11px] font-medium transition-colors"
                :class="containerStateFilter === 'running' ? 'bg-[#0572ec] text-white' : 'bg-white text-[#475569] hover:bg-[#f4f7fa]'"
                @click="containerStateFilter = 'running'"
              >
                실행 중
              </button>
              <button
                type="button"
                class="cursor-pointer border-l border-[#c6d0dc] px-2.5 py-1 text-[11px] font-medium transition-colors"
                :class="containerStateFilter === 'stopped' ? 'bg-[#0572ec] text-white' : 'bg-white text-[#475569] hover:bg-[#f4f7fa]'"
                @click="containerStateFilter = 'stopped'"
              >
                중지됨
              </button>
            </div>
          </div>
        </div>

        <div class="min-h-0 flex-1 overflow-auto">
          <table class="w-full border-collapse text-left">
            <thead class="sticky top-0 z-10 border-b border-[#dce2ea] bg-[#f8fafc] text-[11px] font-semibold text-[#64748b]">
              <tr>
                <th class="px-3 py-2">상태</th>
                <th class="px-3 py-2">이름</th>
                <th class="px-3 py-2">이미지</th>
                <th class="px-3 py-2">CPU / RAM</th>
                <th class="px-3 py-2">포트</th>
                <th class="px-3 py-2 text-right">작업</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-[#eef2f6]">
              <tr
                v-for="item in filteredContainers"
                :key="item.id"
                class="h-9 hover:bg-[#f8fafc]"
              >
                <td class="whitespace-nowrap px-3">
                  <div class="flex items-center gap-1.5">
                    <span class="h-2 w-2 shrink-0 rounded-full" :class="stateDotClass(item.state)" />
                    <span class="text-[11px] text-[#475569]" :title="item.status">{{ item.state }}</span>
                  </div>
                </td>
                <td class="max-w-[180px] px-3">
                  <div class="flex items-center gap-1.5">
                    <span class="truncate font-semibold text-[#1e293b]" :title="item.name">{{ item.name }}</span>
                    <span
                      v-if="item.isWargame"
                      class="shrink-0 rounded-[3px] border border-[#ddd6fe] bg-[#f5f3ff] px-1.5 py-0.5 text-[10px] font-semibold text-[#6d28d9]"
                    >
                      워게임
                    </span>
                  </div>
                </td>
                <td class="max-w-[180px] px-3 font-mono text-[11px] text-[#475569]">
                  <div class="truncate" :title="item.image">{{ item.image }}</div>
                </td>
                <td class="whitespace-nowrap px-3 font-mono text-[11px] tabular-nums text-[#475569]">
                  <span v-if="item.state.toLowerCase() === 'running' && (item.cpuPercent || item.memUsage)">
                    {{ item.cpuPercent || '-' }} · {{ item.memUsage || '-' }}
                  </span>
                  <span v-else class="text-[#94a3b8]">-</span>
                </td>
                <td class="max-w-[170px] px-3 font-mono text-[11px] text-[#475569]">
                  <div class="truncate" :title="item.ports">{{ item.ports || '-' }}</div>
                </td>
                <td class="whitespace-nowrap px-3 text-right">
                  <div class="inline-flex items-center gap-1">
                    <button
                      v-if="item.state.toLowerCase() !== 'running'"
                      type="button"
                      class="dsm-btn !h-6 !px-1.5"
                      title="시작"
                      :disabled="isBusy(`container:${item.id}`)"
                      @click="handleContainerAction(item, 'start')"
                    >
                      <Play class="h-3 w-3 text-[#059669]" />
                    </button>
                    <button
                      v-else
                      type="button"
                      class="dsm-btn !h-6 !px-1.5"
                      title="중지"
                      :disabled="isBusy(`container:${item.id}`)"
                      @click="handleContainerAction(item, 'stop')"
                    >
                      <Square class="h-3 w-3 text-[#d97706]" />
                    </button>

                    <button
                      type="button"
                      class="dsm-btn !h-6 !px-1.5"
                      title="재시작"
                      :disabled="isBusy(`container:${item.id}`)"
                      @click="handleContainerAction(item, 'restart')"
                    >
                      <RotateCcw class="h-3 w-3" />
                    </button>

                    <button
                      type="button"
                      class="dsm-btn !h-6 !px-1.5"
                      title="로그 보기"
                      @click="openContainerLogs(item)"
                    >
                      <FileText class="h-3 w-3" />
                    </button>

                    <button
                      type="button"
                      class="dsm-btn !h-6 !px-1.5"
                      title="상세 정보 (Inspect)"
                      @click="openContainerInspect(item)"
                    >
                      <Info class="h-3 w-3" />
                    </button>

                    <button
                      v-if="item.state.toLowerCase() === 'running'"
                      type="button"
                      class="dsm-btn !h-6 !px-2 text-[#0572ec]"
                      title="컨테이너 쉘 접속"
                      @click="openContainerShell(item)"
                    >
                      <Terminal class="h-3 w-3" />
                      쉘
                    </button>

                    <template v-if="confirmDeleteKey === `container:${item.id}`">
                      <button
                        type="button"
                        class="dsm-btn dsm-btn-danger-solid !h-6 !px-2"
                        :disabled="isBusy(`container:${item.id}`)"
                        @click="handleDeleteContainer(item)"
                      >
                        확인
                      </button>
                      <button
                        type="button"
                        class="dsm-btn !h-6 !px-1.5"
                        @click="confirmDeleteKey = null"
                      >
                        취소
                      </button>
                    </template>
                    <button
                      v-else
                      type="button"
                      class="dsm-btn dsm-btn-danger !h-6 !px-1.5"
                      title="삭제"
                      :disabled="isBusy(`container:${item.id}`)"
                      @click="confirmDeleteKey = `container:${item.id}`"
                    >
                      <Trash2 class="h-3 w-3" />
                    </button>
                  </div>
                </td>
              </tr>
              <tr v-if="filteredContainers.length === 0">
                <td colspan="6" class="px-4 py-8 text-center text-[#64748b]">
                  표시할 컨테이너가 없습니다.
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <div v-else-if="activeSection === 'stacks'" class="flex min-h-0 flex-1 overflow-hidden">
        <div class="flex w-56 shrink-0 flex-col border-r border-[#dce2ea] bg-[#f8fafc]">
          <div class="flex items-center justify-between border-b border-[#dce2ea] px-3 py-2">
            <span class="text-[11.5px] font-semibold text-[#334155]">Compose 스택 ({{ stacks.length }})</span>
            <button type="button" class="dsm-btn dsm-btn-primary !h-6 !px-2" @click="startNewStack">
              <Plus class="h-3 w-3" />
              새 스택
            </button>
          </div>

          <div class="min-h-0 flex-1 space-y-1 overflow-auto p-2">
            <button
              v-for="st in stacks"
              :key="st.name"
              type="button"
              class="flex w-full cursor-pointer flex-col gap-1 rounded-[5px] border px-2.5 py-2 text-left transition-colors"
              :class="
                stackEditorMode.kind === 'edit' && stackEditorMode.name === st.name
                  ? 'border-[#b8d4fe] bg-[#e6f0ff]'
                  : 'border-[#dce2ea] bg-white hover:bg-[#f4f7fa]'
              "
              @click="openExistingStack(st)"
            >
              <div class="flex items-center justify-between gap-1.5">
                <span class="truncate font-semibold text-[#1e293b]">{{ st.name }}</span>
                <span
                  class="shrink-0 rounded-[3px] border px-1.5 py-0.5 text-[10px] font-medium"
                  :class="stackBadgeClass(st.status)"
                >
                  {{ st.status }}
                </span>
              </div>
              <div class="text-[10.5px] tabular-nums text-[#64748b]">
                실행 {{ st.runningCount }} / {{ st.servicesCount }} 서비스
              </div>
            </button>

            <div v-if="stacks.length === 0" class="px-2 py-8 text-center text-[11px] text-[#64748b]">
              등록된 Compose 스택이 없습니다.
            </div>
          </div>
        </div>

        <div class="flex min-w-0 flex-1 flex-col overflow-hidden bg-white">
          <div
            v-if="stackEditorMode.kind === 'none'"
            class="flex flex-1 flex-col items-center justify-center gap-2.5 bg-[#f4f6f9] text-[#64748b]"
          >
            <Layers class="h-8 w-8 text-[#94a3b8]" />
            <div class="text-[12.5px] font-medium text-[#334155]">좌측에서 스택을 선택하거나 새 스택을 생성하세요.</div>
            <button type="button" class="dsm-btn dsm-btn-primary" @click="startNewStack">
              <Plus class="h-3.5 w-3.5" />
              새 스택 만들기
            </button>
          </div>

          <template v-else>
            <div class="flex shrink-0 flex-wrap items-center justify-between gap-2 border-b border-[#dce2ea] bg-[#f8fafc] px-3 py-2">
              <div class="flex items-center gap-2">
                <input
                  v-if="stackEditorMode.kind === 'create'"
                  :value="stackEditorMode.name"
                  type="text"
                  placeholder="스택 이름 (예: jellyfin)"
                  class="dsm-input w-48 font-mono"
                  @input="handleNewStackNameInput"
                />
                <template v-else>
                  <span class="font-mono text-[12.5px] font-semibold text-[#1e293b]">
                    {{ stackEditorMode.name }}
                  </span>
                  <span
                    class="rounded-[3px] border px-1.5 py-0.5 text-[10.5px] font-medium"
                    :class="stackBadgeClass(stackEditorMode.status)"
                  >
                    {{ stackEditorMode.status }}
                  </span>
                </template>
                <span
                  v-if="stackDirty"
                  class="rounded-[3px] border border-[#fcd34d] bg-[#fffbeb] px-1.5 py-0.5 text-[10.5px] font-semibold text-[#b45309]"
                >
                  수정됨
                </span>
              </div>

              <div class="flex flex-wrap items-center gap-1.5">
                <button
                  type="button"
                  class="dsm-btn"
                  :disabled="stackBusy"
                  @click="handleSaveStack"
                >
                  <Save class="h-3.5 w-3.5" />
                  저장
                </button>
                <button
                  type="button"
                  class="dsm-btn dsm-btn-primary"
                  :disabled="stackBusy"
                  @click="handleStackAction('up')"
                >
                  <Play class="h-3.5 w-3.5" />
                  배포 (up -d)
                </button>
                <button
                  type="button"
                  class="dsm-btn"
                  :disabled="stackBusy"
                  @click="handleStackAction('recreate')"
                >
                  <RotateCcw class="h-3.5 w-3.5" />
                  재생성
                </button>
                <button
                  v-if="stackEditorMode.kind === 'edit'"
                  type="button"
                  class="dsm-btn"
                  :disabled="stackBusy"
                  @click="handleStackAction('down')"
                >
                  <Square class="h-3.5 w-3.5 text-[#d97706]" />
                  중지 (down)
                </button>
                <button
                  v-if="stackEditorMode.kind === 'edit'"
                  type="button"
                  class="dsm-btn"
                  :disabled="stackBusy"
                  @click="handleStackLogs"
                >
                  <FileText class="h-3.5 w-3.5" />
                  로그
                </button>

                <template v-if="stackEditorMode.kind === 'edit'">
                  <template v-if="confirmDeleteStack === stackEditorMode.name">
                    <button
                      type="button"
                      class="dsm-btn dsm-btn-danger-solid"
                      :disabled="stackBusy"
                      @click="handleDeleteStack(stackEditorMode.name)"
                    >
                      삭제 확인
                    </button>
                    <button type="button" class="dsm-btn" @click="confirmDeleteStack = null">
                      취소
                    </button>
                  </template>
                  <button
                    v-else
                    type="button"
                    class="dsm-btn dsm-btn-danger"
                    :disabled="stackBusy"
                    @click="confirmDeleteStack = stackEditorMode.name"
                  >
                    <Trash2 class="h-3.5 w-3.5" />
                    삭제
                  </button>
                </template>
              </div>
            </div>

            <div ref="stackEditorHostRef" class="min-h-0 flex-1 overflow-hidden bg-[#282c34]" />

            <div
              v-if="stackOutput !== ''"
              class="flex h-40 shrink-0 flex-col border-t border-[#dce2ea] bg-[#0f172a] text-[#e2e8f0]"
            >
              <div class="flex shrink-0 items-center justify-between border-b border-white/10 px-3 py-1 text-[11px] text-[#94a3b8]">
                <span class="font-mono">{{ stackOutputTitle || '실행 출력' }}</span>
                <button
                  type="button"
                  class="cursor-pointer text-[#94a3b8] hover:text-white"
                  @click="stackOutput = ''"
                >
                  <X class="h-3.5 w-3.5" />
                </button>
              </div>
              <pre class="min-h-0 flex-1 overflow-auto p-3 font-mono text-[11px] leading-relaxed whitespace-pre-wrap">{{ stackOutput }}</pre>
            </div>
          </template>
        </div>
      </div>

      <div v-else-if="activeSection === 'images'" class="flex min-h-0 flex-1 flex-col overflow-hidden">
        <div class="flex shrink-0 flex-wrap items-center justify-between gap-2 border-b border-[#dce2ea] bg-white px-4 py-2">
          <form class="flex items-center gap-2" @submit.prevent="handlePullImage">
            <input
              v-model="pullImageInput"
              type="text"
              placeholder="이미지 이름 (예: nginx:alpine, postgres:16)"
              class="dsm-input w-72 font-mono"
              :disabled="pullingImage"
            />
            <button
              type="submit"
              class="dsm-btn dsm-btn-primary"
              :disabled="pullingImage || pullImageInput.trim() === ''"
            >
              <Download class="h-3.5 w-3.5" />
              {{ pullingImage ? 'Pull 중...' : '이미지 Pull' }}
            </button>
          </form>

          <button
            type="button"
            class="dsm-btn dsm-btn-danger"
            :disabled="pruningImages"
            @click="handlePruneImages('images')"
          >
            <Trash2 class="h-3.5 w-3.5" />
            {{ pruningImages ? '정리 중...' : '미사용 정리 (Prune)' }}
          </button>
        </div>

        <div
          v-if="imageCommandOutput"
          class="flex shrink-0 flex-col border-b border-[#dce2ea] bg-[#0f172a] text-[#e2e8f0]"
        >
          <div class="flex items-center justify-between border-b border-white/10 px-3 py-1 text-[11px] text-[#94a3b8]">
            <span>실행 결과</span>
            <button type="button" class="cursor-pointer hover:text-white" @click="imageCommandOutput = ''">
              <X class="h-3.5 w-3.5" />
            </button>
          </div>
          <pre class="max-h-32 overflow-auto p-3 font-mono text-[11px] whitespace-pre-wrap">{{ imageCommandOutput }}</pre>
        </div>

        <div class="min-h-0 flex-1 overflow-auto">
          <table class="w-full border-collapse text-left">
            <thead class="sticky top-0 z-10 border-b border-[#dce2ea] bg-[#f8fafc] text-[11px] font-semibold text-[#64748b]">
              <tr>
                <th class="px-3.5 py-2">리포지토리</th>
                <th class="px-3.5 py-2">태그</th>
                <th class="px-3.5 py-2">이미지 ID</th>
                <th class="px-3.5 py-2 text-right">크기</th>
                <th class="px-3.5 py-2">생성</th>
                <th class="px-3.5 py-2">상태</th>
                <th class="px-3.5 py-2 text-right">작업</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-[#eef2f6]">
              <tr v-for="img in images" :key="img.id + img.repository + img.tag" class="h-9 hover:bg-[#f8fafc]">
                <td class="max-w-[220px] truncate px-3.5 font-mono text-[11.5px] font-medium text-[#1e293b]" :title="img.repository">
                  {{ img.repository }}
                </td>
                <td class="px-3.5 font-mono text-[11px] text-[#475569]">{{ img.tag }}</td>
                <td class="px-3.5 font-mono text-[11px] text-[#64748b]">{{ img.id.slice(0, 12) }}</td>
                <td class="px-3.5 text-right font-mono text-[11px] tabular-nums text-[#1e293b]">{{ img.size }}</td>
                <td class="px-3.5 text-[11px] text-[#64748b]">{{ img.createdAt || '-' }}</td>
                <td class="px-3.5">
                  <span
                    v-if="img.inUse"
                    class="inline-flex rounded-[3px] border border-[#b8d4fe] bg-[#e6f0ff] px-1.5 py-0.5 text-[10.5px] font-medium text-[#0572ec]"
                  >
                    사용 중
                  </span>
                  <span v-else class="text-[11px] text-[#94a3b8]">미사용</span>
                </td>
                <td class="whitespace-nowrap px-3.5 text-right">
                  <template v-if="confirmDeleteKey === `image:${img.id}`">
                    <button
                      type="button"
                      class="dsm-btn dsm-btn-danger-solid !h-6 !px-2"
                      :disabled="isBusy(`image:${img.id}`)"
                      @click="handleDeleteImage(img)"
                    >
                      확인
                    </button>
                    <button type="button" class="dsm-btn !h-6 !px-1.5" @click="confirmDeleteKey = null">
                      취소
                    </button>
                  </template>
                  <button
                    v-else
                    type="button"
                    class="dsm-btn dsm-btn-danger !h-6 !px-2"
                    :disabled="img.inUse || isBusy(`image:${img.id}`)"
                    :title="img.inUse ? '컨테이너에서 사용 중인 이미지는 삭제할 수 없습니다.' : '이미지 삭제'"
                    @click="confirmDeleteKey = `image:${img.id}`"
                  >
                    <Trash2 class="h-3 w-3" />
                    삭제
                  </button>
                </td>
              </tr>
              <tr v-if="images.length === 0">
                <td colspan="7" class="px-4 py-8 text-center text-[#64748b]">
                  로컬 이미지가 없습니다.
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <div v-else class="min-h-0 flex-1 space-y-4 overflow-auto bg-[#f4f6f9] p-4">
        <div class="overflow-hidden rounded-[6px] border border-[#dce2ea] bg-white">
          <div class="flex items-center gap-2 border-b border-[#dce2ea] bg-[#f8fafc] px-3.5 py-2">
            <HardDrive class="h-3.5 w-3.5 text-[#0572ec]" />
            <span class="text-[12.5px] font-semibold text-[#1e293b]">볼륨 ({{ volumes.length }})</span>
          </div>
          <table class="w-full border-collapse text-left">
            <thead>
              <tr class="border-b border-[#dce2ea] bg-[#f8fafc] text-[11px] font-semibold text-[#64748b]">
                <th class="px-3.5 py-2">이름</th>
                <th class="px-3.5 py-2">드라이버</th>
                <th class="px-3.5 py-2">마운트 경로</th>
                <th class="px-3.5 py-2 text-right">작업</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-[#eef2f6]">
              <tr v-for="vol in volumes" :key="vol.name" class="h-9 hover:bg-[#f8fafc]">
                <td class="max-w-[200px] truncate px-3.5 font-mono text-[11.5px] font-medium text-[#1e293b]" :title="vol.name">
                  {{ vol.name }}
                </td>
                <td class="px-3.5 text-[11px] text-[#475569]">{{ vol.driver }}</td>
                <td class="max-w-[280px] truncate px-3.5 font-mono text-[11px] text-[#64748b]" :title="vol.mountpoint">
                  {{ vol.mountpoint || '-' }}
                </td>
                <td class="whitespace-nowrap px-3.5 text-right">
                  <template v-if="confirmDeleteKey === `volume:${vol.name}`">
                    <button
                      type="button"
                      class="dsm-btn dsm-btn-danger-solid !h-6 !px-2"
                      :disabled="isBusy(`volume:${vol.name}`)"
                      @click="handleDeleteVolume(vol)"
                    >
                      확인
                    </button>
                    <button type="button" class="dsm-btn !h-6 !px-1.5" @click="confirmDeleteKey = null">
                      취소
                    </button>
                  </template>
                  <button
                    v-else
                    type="button"
                    class="dsm-btn dsm-btn-danger !h-6 !px-2"
                    :disabled="isBusy(`volume:${vol.name}`)"
                    @click="confirmDeleteKey = `volume:${vol.name}`"
                  >
                    <Trash2 class="h-3 w-3" />
                    삭제
                  </button>
                </td>
              </tr>
              <tr v-if="volumes.length === 0">
                <td colspan="4" class="px-4 py-6 text-center text-[#64748b]">볼륨이 없습니다.</td>
              </tr>
            </tbody>
          </table>
        </div>

        <div class="overflow-hidden rounded-[6px] border border-[#dce2ea] bg-white">
          <div class="flex items-center gap-2 border-b border-[#dce2ea] bg-[#f8fafc] px-3.5 py-2">
            <Network class="h-3.5 w-3.5 text-[#0572ec]" />
            <span class="text-[12.5px] font-semibold text-[#1e293b]">네트워크 ({{ networks.length }})</span>
          </div>
          <table class="w-full border-collapse text-left">
            <thead>
              <tr class="border-b border-[#dce2ea] bg-[#f8fafc] text-[11px] font-semibold text-[#64748b]">
                <th class="px-3.5 py-2">이름</th>
                <th class="px-3.5 py-2">ID</th>
                <th class="px-3.5 py-2">드라이버</th>
                <th class="px-3.5 py-2">범위</th>
                <th class="px-3.5 py-2 text-right">작업</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-[#eef2f6]">
              <tr v-for="net in networks" :key="net.id" class="h-9 hover:bg-[#f8fafc]">
                <td class="px-3.5 font-mono text-[11.5px] font-medium text-[#1e293b]">
                  <div class="flex items-center gap-1.5">
                    <span>{{ net.name }}</span>
                    <span
                      v-if="isProtectedNetwork(net)"
                      class="rounded-[3px] border border-[#cbd5e1] bg-[#f1f5f9] px-1.5 py-0.5 text-[10px] font-medium text-[#475569]"
                    >
                      기본
                    </span>
                  </div>
                </td>
                <td class="px-3.5 font-mono text-[11px] text-[#64748b]">{{ net.id.slice(0, 12) }}</td>
                <td class="px-3.5 text-[11px] text-[#475569]">{{ net.driver || '-' }}</td>
                <td class="px-3.5 text-[11px] text-[#64748b]">{{ net.scope }}</td>
                <td class="whitespace-nowrap px-3.5 text-right">
                  <template v-if="isProtectedNetwork(net)">
                    <span class="text-[11px] text-[#94a3b8]">보호됨</span>
                  </template>
                  <template v-else-if="confirmDeleteKey === `network:${net.id}`">
                    <button
                      type="button"
                      class="dsm-btn dsm-btn-danger-solid !h-6 !px-2"
                      :disabled="isBusy(`network:${net.id}`)"
                      @click="handleDeleteNetwork(net)"
                    >
                      확인
                    </button>
                    <button type="button" class="dsm-btn !h-6 !px-1.5" @click="confirmDeleteKey = null">
                      취소
                    </button>
                  </template>
                  <button
                    v-else
                    type="button"
                    class="dsm-btn dsm-btn-danger !h-6 !px-2"
                    :disabled="isBusy(`network:${net.id}`)"
                    @click="confirmDeleteKey = `network:${net.id}`"
                  >
                    <Trash2 class="h-3 w-3" />
                    삭제
                  </button>
                </td>
              </tr>
              <tr v-if="networks.length === 0">
                <td colspan="5" class="px-4 py-6 text-center text-[#64748b]">네트워크가 없습니다.</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </section>

    <div
      v-if="logsModal.kind === 'open'"
      class="absolute inset-0 z-30 flex items-center justify-center bg-black/45 p-5"
      @click.self="logsModal = { kind: 'closed' }"
    >
      <div class="flex h-full max-h-[540px] w-full max-w-[760px] flex-col overflow-hidden rounded-[6px] border border-[#c5ced9] bg-white shadow-xl">
        <div class="flex shrink-0 items-center justify-between gap-2 border-b border-[#dce2ea] bg-[#f8fafc] px-3.5 py-2">
          <div class="flex items-center gap-2">
            <FileText class="h-4 w-4 text-[#0572ec]" />
            <span class="font-semibold text-[#1e293b]">컨테이너 로그 · {{ logsModal.containerName }}</span>
          </div>
          <div class="flex items-center gap-1.5">
            <select
              :value="logsModal.tail"
              class="dsm-select !h-6 text-[11px]"
              @change="handleLogsTailChange"
            >
              <option :value="100">최근 100줄</option>
              <option :value="300">최근 300줄</option>
              <option :value="1000">최근 1000줄</option>
            </select>
            <button type="button" class="dsm-btn !h-6 !px-2" :disabled="logsModal.loading" @click="reloadLogsModal()">
              <RefreshCw class="h-3 w-3" :class="{ 'animate-spin': logsModal.loading }" />
              새로고침
            </button>
            <button type="button" class="dsm-btn !h-6 !px-2" @click="copyModalLogs">
              <Check v-if="copiedLogs" class="h-3 w-3 text-[#059669]" />
              <Copy v-else class="h-3 w-3" />
              {{ copiedLogs ? '복사됨' : '복사' }}
            </button>
            <button type="button" class="dsm-btn !h-6 !px-1.5" @click="logsModal = { kind: 'closed' }">
              <X class="h-3.5 w-3.5" />
            </button>
          </div>
        </div>

        <div v-if="logsModal.error" class="border-b border-[#fecdd3] bg-[#fff1f2] px-3.5 py-2 text-[#be123c]">
          {{ logsModal.error }}
        </div>

        <div class="min-h-0 flex-1 overflow-auto bg-[#0f172a] p-3.5 font-mono text-[11.5px] leading-relaxed text-[#e2e8f0]">
          <div v-if="logsModal.loading" class="text-[#94a3b8]">로그를 불러오는 중...</div>
          <pre v-else class="whitespace-pre-wrap break-all">{{ logsModal.logs || '(출력된 로그가 없습니다)' }}</pre>
        </div>
      </div>
    </div>

    <div
      v-if="inspectModal.kind === 'open'"
      class="absolute inset-0 z-30 flex items-center justify-center bg-black/45 p-5"
      @click.self="inspectModal = { kind: 'closed' }"
    >
      <div class="flex h-full max-h-[540px] w-full max-w-[720px] flex-col overflow-hidden rounded-[6px] border border-[#c5ced9] bg-white shadow-xl">
        <div class="flex shrink-0 items-center justify-between border-b border-[#dce2ea] bg-[#f8fafc] px-3.5 py-2">
          <div class="flex items-center gap-2">
            <Info class="h-4 w-4 text-[#0572ec]" />
            <span class="font-semibold text-[#1e293b]">컨테이너 상세 · {{ inspectModal.containerName }}</span>
          </div>
          <button type="button" class="dsm-btn !h-6 !px-1.5" @click="inspectModal = { kind: 'closed' }">
            <X class="h-3.5 w-3.5" />
          </button>
        </div>

        <div class="min-h-0 flex-1 space-y-4 overflow-auto bg-[#f4f6f9] p-4">
          <div v-if="inspectModal.loading" class="py-10 text-center text-[#64748b]">상세 정보를 불러오는 중...</div>
          <div v-else-if="inspectModal.error" class="rounded-[4px] border border-[#fecdd3] bg-[#fff1f2] p-3 text-[#be123c]">
            {{ inspectModal.error }}
          </div>
          <template v-else-if="inspectModal.data">
            <div class="rounded-[6px] border border-[#dce2ea] bg-white p-3.5">
              <div class="mb-2 text-[12px] font-semibold text-[#1e293b]">기본 정보</div>
              <dl class="grid grid-cols-2 gap-x-4 gap-y-1.5 text-[11.5px]">
                <dt class="text-[#64748b]">ID</dt>
                <dd class="truncate font-mono text-[#1e293b]">{{ inspectModal.data.id }}</dd>
                <dt class="text-[#64748b]">이미지</dt>
                <dd class="truncate font-mono text-[#1e293b]">{{ inspectModal.data.image }}</dd>
                <dt class="text-[#64748b]">상태</dt>
                <dd class="font-medium text-[#1e293b]">{{ inspectModal.data.state }}</dd>
                <dt class="text-[#64748b]">재시작 정책</dt>
                <dd class="font-mono text-[#1e293b]">{{ inspectModal.data.restartPolicy || 'no' }}</dd>
              </dl>
            </div>

            <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
              <div class="rounded-[6px] border border-[#dce2ea] bg-white p-3.5">
                <div class="mb-2 text-[12px] font-semibold text-[#1e293b]">포트 바인딩</div>
                <ul v-if="inspectModal.data.ports.length > 0" class="space-y-1 font-mono text-[11px] text-[#334155]">
                  <li v-for="p in inspectModal.data.ports" :key="p">{{ p }}</li>
                </ul>
                <div v-else class="text-[11px] text-[#94a3b8]">바인딩된 포트가 없습니다.</div>
              </div>

              <div class="rounded-[6px] border border-[#dce2ea] bg-white p-3.5">
                <div class="mb-2 text-[12px] font-semibold text-[#1e293b]">네트워크</div>
                <ul v-if="inspectModal.data.networks.length > 0" class="space-y-1 font-mono text-[11px] text-[#334155]">
                  <li v-for="n in inspectModal.data.networks" :key="n">{{ n }}</li>
                </ul>
                <div v-else class="text-[11px] text-[#94a3b8]">연결된 네트워크가 없습니다.</div>
              </div>
            </div>

            <div class="rounded-[6px] border border-[#dce2ea] bg-white p-3.5">
              <div class="mb-2 text-[12px] font-semibold text-[#1e293b]">마운트 ({{ inspectModal.data.mounts.length }})</div>
              <div v-if="inspectModal.data.mounts.length > 0" class="space-y-1.5 font-mono text-[11px]">
                <div
                  v-for="(m, idx) in inspectModal.data.mounts"
                  :key="`${m.source}-${m.destination}-${idx}`"
                  class="rounded-[4px] border border-[#eef2f6] bg-[#f8fafc] px-2.5 py-1.5"
                >
                  <span class="font-semibold text-[#0572ec]">[{{ m.type }}]</span>
                  <span class="ml-1.5 break-all text-[#1e293b]">{{ m.source }}</span>
                  <span v-if="m.destination" class="text-[#64748b]"> → {{ m.destination }}</span>
                  <span class="ml-1.5 text-[10px] text-[#64748b]">({{ m.rw ? 'rw' : 'ro' }})</span>
                </div>
              </div>
              <div v-else class="text-[11px] text-[#94a3b8]">마운트된 볼륨이 없습니다.</div>
            </div>

            <div class="rounded-[6px] border border-[#dce2ea] bg-white p-3.5">
              <div class="mb-2 text-[12px] font-semibold text-[#1e293b]">환경 변수 ({{ inspectModal.data.env.length }})</div>
              <div
                v-if="inspectModal.data.env.length > 0"
                class="max-h-44 space-y-1 overflow-auto rounded-[4px] border border-[#eef2f6] bg-[#f8fafc] p-2.5 font-mono text-[11px] text-[#334155]"
              >
                <div v-for="(envLine, idx) in inspectModal.data.env" :key="idx" class="break-all">
                  {{ envLine }}
                </div>
              </div>
              <div v-else class="text-[11px] text-[#94a3b8]">설정된 환경 변수가 없습니다.</div>
            </div>
          </template>
        </div>
      </div>
    </div>
  </div>
</template>
