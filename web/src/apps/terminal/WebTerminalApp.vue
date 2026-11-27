<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref, watch, type ComponentPublicInstance } from 'vue'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import { Plus, RefreshCw, RotateCw, Terminal as TerminalIcon, X } from 'lucide-vue-next'
import { api } from '../../api/client'
import type { TerminalLaunchTarget } from '../../stores/desktop'
import type { TerminalTarget, TerminalTargetKind } from '../../types/api'

const props = defineProps<{
  readonly initialTarget?: TerminalLaunchTarget
}>()

type TabConnStatus = 'connecting' | 'connected' | 'disconnected' | 'error'

interface TerminalTab {
  readonly id: string
  readonly targetKind: TerminalTargetKind
  readonly container: string
  readonly label: string
  status: TabConnStatus
  errorText: string
}

interface TerminalRuntime {
  readonly term: Terminal
  readonly fitAddon: FitAddon
  ws: WebSocket | null
  resizeObserver: ResizeObserver | null
  containerEl: HTMLElement | null
}

const DEFAULT_HOST_TARGET: TerminalTarget = {
  kind: 'host',
  id: 'host',
  label: '호스트 쉘',
}

const targets = ref<readonly TerminalTarget[]>([DEFAULT_HOST_TARGET])
const selectedTargetKey = ref<string>('host:host')
const loadingTargets = ref(false)
const tabs = ref<TerminalTab[]>([])
const activeTabId = ref<string | null>(null)

const runtimes = new Map<string, TerminalRuntime>()
let tabCounter = 1
let lastHandledNonce: number | null = null
let isUnmounted = false

function makeTargetKey(target: TerminalTarget): string {
  return `${target.kind}:${target.id}`
}

function findSelectedTarget(): TerminalTarget {
  const found = targets.value.find((t) => makeTargetKey(t) === selectedTargetKey.value)
  return found ?? targets.value[0] ?? DEFAULT_HOST_TARGET
}

async function refreshTargets(): Promise<void> {
  loadingTargets.value = true
  try {
    const list = await api.listTerminalTargets()
    if (isUnmounted) return
    targets.value = list.length > 0 ? list : [DEFAULT_HOST_TARGET]
    const stillExists = targets.value.some((t) => makeTargetKey(t) === selectedTargetKey.value)
    if (!stillExists) {
      const first = targets.value[0] ?? DEFAULT_HOST_TARGET
      selectedTargetKey.value = makeTargetKey(first)
    }
  } catch {
    if (targets.value.length === 0) {
      targets.value = [DEFAULT_HOST_TARGET]
    }
  } finally {
    loadingTargets.value = false
  }
}

function sendResize(rt: TerminalRuntime): void {
  if (rt.ws?.readyState !== WebSocket.OPEN) return
  if (rt.term.cols <= 0 || rt.term.rows <= 0) return
  rt.ws.send(JSON.stringify({ type: 'resize', cols: rt.term.cols, rows: rt.term.rows }))
}

function fitAndSyncSize(tabId: string): void {
  const rt = runtimes.get(tabId)
  if (!rt || !rt.containerEl) return
  if (rt.containerEl.clientWidth <= 0 || rt.containerEl.clientHeight <= 0) return
  try {
    rt.fitAddon.fit()
    sendResize(rt)
  } catch {
    return
  }
}

function connectTabSocket(tab: TerminalTab): void {
  const rt = runtimes.get(tab.id)
  if (!rt) return

  if (rt.ws !== null) {
    rt.ws.onopen = null
    rt.ws.onmessage = null
    rt.ws.onerror = null
    rt.ws.onclose = null
    rt.ws.close()
    rt.ws = null
  }

  tab.status = 'connecting'
  tab.errorText = ''

  const cols = rt.term.cols > 0 ? rt.term.cols : 80
  const rows = rt.term.rows > 0 ? rt.term.rows : 24
  const wsUrl = api.terminalWsUrl({
    target: tab.targetKind,
    container: tab.targetKind === 'docker' ? tab.container : undefined,
    cols,
    rows,
  })

  const socket = new WebSocket(wsUrl)
  socket.binaryType = 'arraybuffer'
  rt.ws = socket

  socket.onopen = () => {
    if (isUnmounted || rt.ws !== socket) {
      socket.close()
      return
    }
    tab.status = 'connected'
    fitAndSyncSize(tab.id)
    if (activeTabId.value === tab.id) {
      rt.term.focus()
    }
  }

  socket.onmessage = (event) => {
    if (isUnmounted || rt.ws !== socket) return
    if (typeof event.data === 'string') {
      rt.term.write(event.data)
    } else if (event.data instanceof ArrayBuffer) {
      rt.term.write(new Uint8Array(event.data))
    } else if (event.data instanceof Blob) {
      void event.data.arrayBuffer().then((buf) => {
        if (!isUnmounted && rt.ws === socket) {
          rt.term.write(new Uint8Array(buf))
        }
      })
    }
  }

  socket.onerror = () => {
    if (rt.ws !== socket) return
    tab.status = 'error'
    tab.errorText = '터미널 WebSocket 연결에 실패했습니다.'
  }

  socket.onclose = (event) => {
    if (rt.ws !== socket) return
    rt.ws = null
    if (tab.status !== 'error') {
      tab.status = 'disconnected'
    }
    const reason = event.reason ? ` (${event.reason})` : ''
    rt.term.write(`\r\n\x1b[90m[세션이 종료되었습니다${reason}]\x1b[0m\r\n`)
  }
}

function createRuntime(tab: TerminalTab): TerminalRuntime {
  const term = new Terminal({
    cursorBlink: true,
    fontSize: 13,
    fontFamily: '"JetBrains Mono", "SFMono-Regular", Menlo, Monaco, Consolas, monospace',
    scrollback: 5000,
    theme: {
      background: '#12161f',
      foreground: '#e2e8f0',
      cursor: '#3b9bff',
      cursorAccent: '#12161f',
      selectionBackground: '#264f78',
      black: '#1e293b',
      red: '#f87171',
      green: '#34d399',
      yellow: '#fbbf24',
      blue: '#60a5fa',
      magenta: '#c084fc',
      cyan: '#22d3ee',
      white: '#e2e8f0',
      brightBlack: '#64748b',
      brightRed: '#fca5a5',
      brightGreen: '#6ee7b7',
      brightYellow: '#fde68a',
      brightBlue: '#93c5fd',
      brightMagenta: '#d8b4fe',
      brightCyan: '#67e8f9',
      brightWhite: '#f8fafc',
    },
  })
  const fitAddon = new FitAddon()
  term.loadAddon(fitAddon)

  const rt: TerminalRuntime = {
    term,
    fitAddon,
    ws: null,
    resizeObserver: null,
    containerEl: null,
  }

  term.onData((data) => {
    if (rt.ws?.readyState === WebSocket.OPEN) {
      rt.ws.send(JSON.stringify({ type: 'input', data }))
    }
  })

  runtimes.set(tab.id, rt)
  return rt
}

function bindTabContainer(tabId: string, el: Element | ComponentPublicInstance | null): void {
  const rt = runtimes.get(tabId)
  const tab = tabs.value.find((t) => t.id === tabId)
  if (!rt || !tab) return

  if (!(el instanceof HTMLElement)) {
    if (rt.resizeObserver) {
      rt.resizeObserver.disconnect()
      rt.resizeObserver = null
    }
    rt.containerEl = null
    return
  }

  if (rt.containerEl === el) return
  rt.containerEl = el
  rt.term.open(el)

  const observer = new ResizeObserver(() => {
    fitAndSyncSize(tabId)
  })
  observer.observe(el)
  rt.resizeObserver = observer

  void nextTick(() => {
    fitAndSyncSize(tabId)
    if (rt.ws === null) {
      connectTabSocket(tab)
    }
  })
}

function openTab(params: {
  readonly kind: TerminalTargetKind
  readonly container: string
  readonly label: string
}): void {
  const id = `term-tab-${tabCounter++}`
  const tab: TerminalTab = {
    id,
    targetKind: params.kind,
    container: params.container,
    label: params.label,
    status: 'connecting',
    errorText: '',
  }
  createRuntime(tab)
  tabs.value.push(tab)
  activeTabId.value = id
}

function openSelectedTab(): void {
  const target = findSelectedTarget()
  openTab({
    kind: target.kind,
    container: target.kind === 'docker' ? target.id : '',
    label: target.label,
  })
}

function disposeRuntime(tabId: string): void {
  const rt = runtimes.get(tabId)
  if (!rt) return
  if (rt.resizeObserver) {
    rt.resizeObserver.disconnect()
    rt.resizeObserver = null
  }
  if (rt.ws) {
    rt.ws.onopen = null
    rt.ws.onmessage = null
    rt.ws.onerror = null
    rt.ws.onclose = null
    rt.ws.close()
    rt.ws = null
  }
  rt.term.dispose()
  runtimes.delete(tabId)
}

function closeTab(tabId: string): void {
  const idx = tabs.value.findIndex((t) => t.id === tabId)
  if (idx < 0) return
  disposeRuntime(tabId)
  tabs.value.splice(idx, 1)
  if (activeTabId.value === tabId) {
    const fallback = tabs.value[Math.min(idx, tabs.value.length - 1)]
    activeTabId.value = fallback ? fallback.id : null
  }
}

function reconnectActiveTab(): void {
  const current = tabs.value.find((t) => t.id === activeTabId.value)
  if (!current) return
  const rt = runtimes.get(current.id)
  if (rt) {
    rt.term.write('\r\n\x1b[36m[다시 연결하는 중...]\x1b[0m\r\n')
  }
  connectTabSocket(current)
}

function selectTab(tabId: string): void {
  activeTabId.value = tabId
}

watch(activeTabId, (nextId) => {
  if (!nextId) return
  void nextTick(() => {
    fitAndSyncSize(nextId)
    runtimes.get(nextId)?.term.focus()
  })
})

watch(
  () => props.initialTarget?.nonce,
  (nonce) => {
    const target = props.initialTarget
    if (nonce === undefined || !target || lastHandledNonce === nonce) return
    lastHandledNonce = nonce
    const container = target.kind === 'docker' ? (target.container ?? '') : ''
    const label = target.label && target.label !== '' ? target.label : container || '호스트 쉘'
    openTab({
      kind: target.kind,
      container,
      label,
    })
    void refreshTargets()
  },
  { immediate: true },
)

onMounted(async () => {
  await refreshTargets()
  if (isUnmounted) return
  if (tabs.value.length === 0) {
    openSelectedTab()
  }
})

onUnmounted(() => {
  isUnmounted = true
  for (const tabId of Array.from(runtimes.keys())) {
    disposeRuntime(tabId)
  }
})

function statusDotClass(status: TabConnStatus): string {
  switch (status) {
    case 'connected':
      return 'bg-[#10b981]'
    case 'connecting':
      return 'bg-[#f59e0b] animate-pulse'
    case 'disconnected':
      return 'bg-[#94a3b8]'
    case 'error':
      return 'bg-[#e11d48]'
    default: {
      const _exhaustive: never = status
      return _exhaustive
    }
  }
}
</script>

<template>
  <div class="flex h-full w-full flex-col overflow-hidden bg-[#12161f] text-[12px]">
    <div class="flex shrink-0 flex-nowrap items-center justify-between gap-2 overflow-x-auto border-b border-[#dce2ea] bg-[#f4f6f9] px-2.5 py-1.5 text-[#1e293b]">
      <div class="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto">
        <div
          v-for="tab in tabs"
          :key="tab.id"
          class="group flex h-7 max-w-[200px] shrink-0 cursor-pointer items-center gap-1.5 rounded-[4px] border px-2 text-[11.5px] transition"
          :class="
            activeTabId === tab.id
              ? 'border-[#b8d4fe] bg-white font-semibold text-[#0058cc] shadow-2xs'
              : 'border-transparent bg-transparent text-[#475569] hover:border-[#dce2ea] hover:bg-[#eaeef4]'
          "
          @click="selectTab(tab.id)"
        >
          <span class="h-2 w-2 shrink-0 rounded-full" :class="statusDotClass(tab.status)" />
          <TerminalIcon class="h-3.5 w-3.5 shrink-0" />
          <span class="truncate">{{ tab.label }}</span>
          <button
            type="button"
            class="ml-0.5 rounded-[3px] p-0.5 text-[#64748b] hover:bg-[#e2e8f0] hover:text-[#1e293b]"
            title="탭 닫기"
            @click.stop="closeTab(tab.id)"
          >
            <X class="h-3 w-3" />
          </button>
        </div>
      </div>

      <div class="flex shrink-0 items-center gap-1.5">
        <select
          v-model="selectedTargetKey"
          class="dsm-select max-w-[210px]"
          aria-label="터미널 대상 선택"
        >
          <option
            v-for="target in targets"
            :key="makeTargetKey(target)"
            :value="makeTargetKey(target)"
          >
            {{ target.label }}
          </option>
        </select>

        <button
          type="button"
          class="dsm-btn"
          title="대상 목록 새로고침"
          @click="refreshTargets"
        >
          <RefreshCw class="h-3.5 w-3.5" :class="{ 'animate-spin': loadingTargets }" />
        </button>

        <button type="button" class="dsm-btn" @click="openSelectedTab">
          <Plus class="h-3.5 w-3.5 text-[#0572ec]" />
          새 탭
        </button>

        <button
          type="button"
          class="dsm-btn"
          :disabled="activeTabId === null"
          @click="reconnectActiveTab"
        >
          <RotateCw class="h-3.5 w-3.5" />
          재연결
        </button>
      </div>
    </div>

    <div class="relative min-h-0 flex-1 overflow-hidden bg-[#12161f]">
      <div
        v-if="tabs.length === 0"
        class="flex h-full w-full flex-col items-center justify-center gap-2.5 text-[#94a3b8]"
      >
        <TerminalIcon class="h-8 w-8 text-[#475569]" />
        <p class="text-[12.5px]">열려 있는 터미널 탭이 없습니다.</p>
        <button type="button" class="dsm-btn dsm-btn-primary" @click="openSelectedTab">
          <Plus class="h-3.5 w-3.5" />
          새 터미널 열기
        </button>
      </div>

      <div
        v-for="tab in tabs"
        :key="tab.id"
        v-show="activeTabId === tab.id"
        class="h-full w-full p-2"
      >
        <div
          :ref="(el) => bindTabContainer(tab.id, el)"
          class="h-full w-full"
        />
      </div>
    </div>
  </div>
</template>
