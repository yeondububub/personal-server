<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { Activity, Clock, Cpu, HardDrive, Network, Server } from 'lucide-vue-next'
import { api } from '../../api/client'
import { parseMonitorSnapshot, type MonitorSnapshot } from '../../types/api'
import { formatBytes, formatDuration } from '../../utils/format'

type MonitorTab = 'overview' | 'compute' | 'network' | 'storage'
type WsStatus = 'connecting' | 'live' | 'reconnecting'

interface MetricSample {
  readonly timestamp: number
  readonly cpuPercent: number
  readonly memPercent: number
  readonly netRxKBps: number
  readonly netTxKBps: number
}

const MAX_SAMPLES = 30

const activeTab = ref<MonitorTab>('overview')
const snapshot = ref<MonitorSnapshot | null>(null)
const samples = ref<readonly MetricSample[]>([])
const wsStatus = ref<WsStatus>('connecting')

let ws: WebSocket | null = null
let reconnectTimer: ReturnType<typeof setTimeout> | null = null
let isUnmounted = false

function clampPercent(val: number): number {
  if (!Number.isFinite(val)) return 0
  return Math.min(100, Math.max(0, val))
}

function pushSnapshot(next: MonitorSnapshot): void {
  snapshot.value = next
  const entry: MetricSample = {
    timestamp: next.timestamp,
    cpuPercent: clampPercent(next.cpuTotalPercent),
    memPercent: clampPercent(next.memUsedPercent),
    netRxKBps: Math.max(0, next.netRxBytesPerSec / 1024),
    netTxKBps: Math.max(0, next.netTxBytesPerSec / 1024),
  }
  const combined = [...samples.value, entry]
  samples.value = combined.length > MAX_SAMPLES ? combined.slice(combined.length - MAX_SAMPLES) : combined
}

async function fetchInitialSnapshot(): Promise<void> {
  try {
    const snap = await api.fetchMonitorSnapshot()
    if (!isUnmounted) {
      pushSnapshot(snap)
    }
  } catch {
    return
  }
}

function scheduleReconnect(): void {
  if (isUnmounted || reconnectTimer !== null) return
  wsStatus.value = 'reconnecting'
  reconnectTimer = setTimeout(() => {
    reconnectTimer = null
    connectWebSocket()
  }, 3000)
}

function connectWebSocket(): void {
  if (isUnmounted) return
  if (ws !== null) {
    ws.onclose = null
    ws.onerror = null
    ws.onmessage = null
    ws.close()
    ws = null
  }

  wsStatus.value = snapshot.value ? 'reconnecting' : 'connecting'
  const socket = new WebSocket(api.monitorWsUrl())
  ws = socket

  socket.onopen = () => {
    if (isUnmounted) {
      socket.close()
      return
    }
    wsStatus.value = 'live'
  }

  socket.onmessage = (event) => {
    if (isUnmounted || typeof event.data !== 'string') return
    try {
      const parsed: unknown = JSON.parse(event.data)
      pushSnapshot(parseMonitorSnapshot(parsed))
    } catch {
      return
    }
  }

  socket.onerror = () => {
    socket.close()
  }

  socket.onclose = () => {
    if (ws === socket) {
      ws = null
    }
    scheduleReconnect()
  }
}

onMounted(() => {
  void fetchInitialSnapshot()
  connectWebSocket()
})

onUnmounted(() => {
  isUnmounted = true
  if (reconnectTimer !== null) {
    clearTimeout(reconnectTimer)
    reconnectTimer = null
  }
  if (ws !== null) {
    ws.onclose = null
    ws.onerror = null
    ws.onmessage = null
    ws.close()
    ws = null
  }
})

const cpuSeries = computed<readonly number[]>(() => samples.value.map((s) => s.cpuPercent))
const memSeries = computed<readonly number[]>(() => samples.value.map((s) => s.memPercent))
const netRxSeries = computed<readonly number[]>(() => samples.value.map((s) => s.netRxKBps))
const netTxSeries = computed<readonly number[]>(() => samples.value.map((s) => s.netTxKBps))

const netMaxKBps = computed<number>(() => {
  let peak = 10
  for (const s of samples.value) {
    if (s.netRxKBps > peak) peak = s.netRxKBps
    if (s.netTxKBps > peak) peak = s.netTxKBps
  }
  return Math.ceil(peak * 1.15)
})

const netPeakRxKBps = computed<number>(() => {
  let peak = 0
  for (const s of samples.value) {
    if (s.netRxKBps > peak) peak = s.netRxKBps
  }
  return peak
})

const netPeakTxKBps = computed<number>(() => {
  let peak = 0
  for (const s of samples.value) {
    if (s.netTxKBps > peak) peak = s.netTxKBps
  }
  return peak
})

const swapUsedPercent = computed<number>(() => {
  const snap = snapshot.value
  if (!snap || snap.swapTotalBytes <= 0) return 0
  return clampPercent((snap.swapUsedBytes / snap.swapTotalBytes) * 100)
})

function buildSeriesCoords(
  values: readonly number[],
  maxVal: number,
  width: number,
  height: number,
  pad = 4,
): readonly { readonly x: number; readonly y: number }[] {
  if (values.length === 0) return []
  const safeMax = maxVal > 0 ? maxVal : 1
  const usableW = Math.max(1, width - pad * 2)
  const usableH = Math.max(1, height - pad * 2)
  const stepX = MAX_SAMPLES > 1 ? usableW / (MAX_SAMPLES - 1) : usableW
  const startOffset = MAX_SAMPLES - values.length

  return values.map((v, idx) => {
    const ratio = Math.min(1, Math.max(0, v / safeMax))
    const x = pad + (startOffset + idx) * stepX
    const y = height - pad - ratio * usableH
    return { x, y }
  })
}

function buildLinePoints(
  values: readonly number[],
  maxVal: number,
  width: number,
  height: number,
  pad = 4,
): string {
  const coords = buildSeriesCoords(values, maxVal, width, height, pad)
  if (coords.length === 0) return ''
  if (coords.length === 1) {
    const pt = coords[0]
    if (!pt) return ''
    return `${pad.toFixed(1)},${pt.y.toFixed(1)} ${pt.x.toFixed(1)},${pt.y.toFixed(1)}`
  }
  return coords.map((pt) => `${pt.x.toFixed(1)},${pt.y.toFixed(1)}`).join(' ')
}

function buildAreaPath(
  values: readonly number[],
  maxVal: number,
  width: number,
  height: number,
  pad = 4,
): string {
  const coords = buildSeriesCoords(values, maxVal, width, height, pad)
  const first = coords[0]
  const last = coords[coords.length - 1]
  if (!first || !last) return ''
  const baseline = (height - pad).toFixed(1)
  if (coords.length === 1) {
    return `M ${pad.toFixed(1)} ${baseline} L ${pad.toFixed(1)} ${first.y.toFixed(1)} L ${first.x.toFixed(1)} ${first.y.toFixed(1)} L ${first.x.toFixed(1)} ${baseline} Z`
  }
  const line = coords.map((pt, i) => `${i === 0 ? 'M' : 'L'} ${pt.x.toFixed(1)} ${pt.y.toFixed(1)}`).join(' ')
  return `${line} L ${last.x.toFixed(1)} ${baseline} L ${first.x.toFixed(1)} ${baseline} Z`
}

function formatRate(bytesPerSec: number): string {
  if (!Number.isFinite(bytesPerSec) || bytesPerSec <= 0) return '0 B/s'
  return `${formatBytes(bytesPerSec)}/s`
}

function formatKBps(kbps: number): string {
  if (!Number.isFinite(kbps) || kbps <= 0) return '0 KB/s'
  if (kbps >= 1024) {
    return `${(kbps / 1024).toFixed(2)} MB/s`
  }
  return `${kbps.toFixed(1)} KB/s`
}

function usedPercent(total: number, free: number): number {
  if (total <= 0) return 0
  return Math.min(100, Math.max(0, Math.round(((total - free) / total) * 100)))
}

function barColorClass(percent: number): string {
  if (percent >= 90) return 'bg-[#e11d48]'
  if (percent >= 75) return 'bg-[#f59e0b]'
  return 'bg-[#0572ec]'
}

const NAV_TABS: readonly { readonly id: MonitorTab; readonly label: string }[] = [
  { id: 'overview', label: '개요' },
  { id: 'compute', label: 'CPU / 메모리' },
  { id: 'network', label: '네트워크' },
  { id: 'storage', label: '스토리지' },
]
</script>

<template>
  <div class="flex h-full w-full overflow-hidden bg-[#f4f6f9] text-[12px] text-[#1e293b]">
    <aside class="hidden w-44 shrink-0 flex-col justify-between border-r border-[#dce2ea] bg-[#f5f7fa] p-2 sm:flex">
      <div class="flex flex-col gap-1">
        <button
          v-for="tab in NAV_TABS"
          :key="tab.id"
          type="button"
          class="flex items-center gap-2 rounded-[4px] px-2.5 py-2 text-left text-[12px] transition"
          :class="
            activeTab === tab.id
              ? 'bg-[#e6f0ff] font-semibold text-[#0058cc]'
              : 'text-[#334155] hover:bg-[#eaeef4]'
          "
          @click="activeTab = tab.id"
        >
          <Activity v-if="tab.id === 'overview'" class="h-4 w-4 shrink-0" />
          <Cpu v-else-if="tab.id === 'compute'" class="h-4 w-4 shrink-0" />
          <Network v-else-if="tab.id === 'network'" class="h-4 w-4 shrink-0" />
          <HardDrive v-else class="h-4 w-4 shrink-0" />
          <span>{{ tab.label }}</span>
        </button>
      </div>

      <div class="space-y-1 border-t border-[#dce2ea] px-2 pt-2.5 text-[11px]">
        <div
          v-if="wsStatus === 'live'"
          class="flex items-center gap-1.5 font-semibold text-[#059669]"
        >
          <span class="h-2 w-2 rounded-full bg-[#10b981]" />
          실시간 스트리밍
        </div>
        <div
          v-else-if="wsStatus === 'connecting'"
          class="flex items-center gap-1.5 text-[#64748b]"
        >
          <span class="h-2 w-2 animate-pulse rounded-full bg-[#0572ec]" />
          연결 중...
        </div>
        <div
          v-else
          class="flex items-center gap-1.5 font-medium text-[#d97706]"
        >
          <span class="h-2 w-2 animate-pulse rounded-full bg-[#f59e0b]" />
          재연결 대기 중
        </div>
        <div v-if="snapshot" class="truncate text-[10px] text-[#64748b]" :title="snapshot.hostname">
          {{ snapshot.hostname || snapshot.os }}
        </div>
      </div>
    </aside>

    <div class="flex min-w-0 flex-1 flex-col overflow-y-auto p-4">
      <div class="mb-3 flex flex-wrap items-center gap-1.5 sm:hidden">
        <button
          v-for="tab in NAV_TABS"
          :key="tab.id"
          type="button"
          class="dsm-btn"
          :class="activeTab === tab.id ? '!border-[#0572ec] !bg-[#e6f0ff] !text-[#0058cc] font-semibold' : ''"
          @click="activeTab = tab.id"
        >
          {{ tab.label }}
        </button>
      </div>

      <div
        v-if="!snapshot"
        class="flex flex-1 items-center justify-center rounded-[6px] border border-[#dce2ea] bg-white py-12 text-[#64748b]"
      >
        실시간 시스템 리소스 데이터를 수집하는 중...
      </div>

      <div v-else-if="activeTab === 'overview'" class="space-y-3.5">
        <div class="flex flex-nowrap items-center justify-between gap-3 overflow-x-auto rounded-[6px] border border-[#dce2ea] bg-white px-3.5 py-2.5 shadow-2xs">
          <div class="flex shrink-0 flex-nowrap items-center gap-4 text-[11.5px] text-[#475569]">
            <span class="inline-flex shrink-0 items-center gap-1.5 whitespace-nowrap font-semibold text-[#1e293b]">
              <Server class="h-3.5 w-3.5 text-[#0572ec]" />
              {{ snapshot.hostname || 'Personal Server' }}
            </span>
            <span class="whitespace-nowrap">{{ snapshot.platform || snapshot.os }}</span>
            <span class="whitespace-nowrap tabular-nums">{{ snapshot.cpuPerCore.length }} Cores</span>
          </div>
          <div class="inline-flex shrink-0 items-center gap-1.5 whitespace-nowrap text-[11px] tabular-nums text-[#64748b]">
            <Clock class="h-3.5 w-3.5 text-[#0572ec]" />
            가동 시간 {{ formatDuration(snapshot.uptimeSeconds) }}
          </div>
        </div>

        <div class="grid grid-cols-1 gap-3 lg:grid-cols-3">
          <section class="flex flex-col rounded-[6px] border border-[#dce2ea] bg-white p-3.5 shadow-2xs">
            <div class="flex items-center justify-between">
              <span class="text-[12px] font-semibold text-[#1e293b]">CPU 사용률</span>
              <span class="text-[18px] font-bold tabular-nums text-[#0572ec]">
                {{ clampPercent(snapshot.cpuTotalPercent).toFixed(1) }}%
              </span>
            </div>
            <div class="mt-2 h-24 w-full overflow-hidden rounded-[4px] border border-[#e2e8f0] bg-[#f8fafc]">
              <svg viewBox="0 0 240 88" preserveAspectRatio="none" class="h-full w-full">
                <line x1="0" y1="22" x2="240" y2="22" stroke="#e2e8f0" stroke-width="1" stroke-dasharray="3 3" />
                <line x1="0" y1="44" x2="240" y2="44" stroke="#e2e8f0" stroke-width="1" stroke-dasharray="3 3" />
                <line x1="0" y1="66" x2="240" y2="66" stroke="#e2e8f0" stroke-width="1" stroke-dasharray="3 3" />
                <path :d="buildAreaPath(cpuSeries, 100, 240, 88, 3)" fill="#0572ec" fill-opacity="0.14" />
                <polyline
                  :points="buildLinePoints(cpuSeries, 100, 240, 88, 3)"
                  fill="none"
                  stroke="#0572ec"
                  stroke-width="1.8"
                  stroke-linejoin="round"
                  stroke-linecap="round"
                />
              </svg>
            </div>
            <div class="mt-2 flex items-center justify-between text-[11px] text-[#64748b]">
              <span>최근 60초</span>
              <span class="tabular-nums">{{ snapshot.cpuPerCore.length }} 논리 코어</span>
            </div>
          </section>

          <section class="flex flex-col rounded-[6px] border border-[#dce2ea] bg-white p-3.5 shadow-2xs">
            <div class="flex items-center justify-between">
              <span class="text-[12px] font-semibold text-[#1e293b]">메모리 사용률</span>
              <span class="text-[18px] font-bold tabular-nums text-[#0d9488]">
                {{ clampPercent(snapshot.memUsedPercent).toFixed(1) }}%
              </span>
            </div>
            <div class="mt-2 h-24 w-full overflow-hidden rounded-[4px] border border-[#e2e8f0] bg-[#f8fafc]">
              <svg viewBox="0 0 240 88" preserveAspectRatio="none" class="h-full w-full">
                <line x1="0" y1="22" x2="240" y2="22" stroke="#e2e8f0" stroke-width="1" stroke-dasharray="3 3" />
                <line x1="0" y1="44" x2="240" y2="44" stroke="#e2e8f0" stroke-width="1" stroke-dasharray="3 3" />
                <line x1="0" y1="66" x2="240" y2="66" stroke="#e2e8f0" stroke-width="1" stroke-dasharray="3 3" />
                <path :d="buildAreaPath(memSeries, 100, 240, 88, 3)" fill="#0d9488" fill-opacity="0.14" />
                <polyline
                  :points="buildLinePoints(memSeries, 100, 240, 88, 3)"
                  fill="none"
                  stroke="#0d9488"
                  stroke-width="1.8"
                  stroke-linejoin="round"
                  stroke-linecap="round"
                />
              </svg>
            </div>
            <div class="mt-2 flex items-center justify-between text-[11px] tabular-nums text-[#64748b]">
              <span>사용 {{ formatBytes(snapshot.memUsedBytes) }}</span>
              <span>전체 {{ formatBytes(snapshot.memTotalBytes) }}</span>
            </div>
          </section>

          <section class="flex flex-col rounded-[6px] border border-[#dce2ea] bg-white p-3.5 shadow-2xs">
            <div class="flex items-center justify-between">
              <span class="text-[12px] font-semibold text-[#1e293b]">네트워크</span>
              <span class="text-[11px] font-medium tabular-nums text-[#475569]">
                최대 {{ formatKBps(netMaxKBps) }}
              </span>
            </div>
            <div class="mt-2 h-24 w-full overflow-hidden rounded-[4px] border border-[#e2e8f0] bg-[#f8fafc]">
              <svg viewBox="0 0 240 88" preserveAspectRatio="none" class="h-full w-full">
                <line x1="0" y1="22" x2="240" y2="22" stroke="#e2e8f0" stroke-width="1" stroke-dasharray="3 3" />
                <line x1="0" y1="44" x2="240" y2="44" stroke="#e2e8f0" stroke-width="1" stroke-dasharray="3 3" />
                <line x1="0" y1="66" x2="240" y2="66" stroke="#e2e8f0" stroke-width="1" stroke-dasharray="3 3" />
                <path :d="buildAreaPath(netRxSeries, netMaxKBps, 240, 88, 3)" fill="#0572ec" fill-opacity="0.10" />
                <polyline
                  :points="buildLinePoints(netRxSeries, netMaxKBps, 240, 88, 3)"
                  fill="none"
                  stroke="#0572ec"
                  stroke-width="1.7"
                  stroke-linejoin="round"
                  stroke-linecap="round"
                />
                <polyline
                  :points="buildLinePoints(netTxSeries, netMaxKBps, 240, 88, 3)"
                  fill="none"
                  stroke="#0d9488"
                  stroke-width="1.7"
                  stroke-linejoin="round"
                  stroke-linecap="round"
                />
              </svg>
            </div>
            <div class="mt-2 flex items-center justify-between text-[11px] tabular-nums">
              <span class="font-medium text-[#0572ec]">↓ {{ formatRate(snapshot.netRxBytesPerSec) }}</span>
              <span class="font-medium text-[#0d9488]">↑ {{ formatRate(snapshot.netTxBytesPerSec) }}</span>
            </div>
          </section>
        </div>

        <section class="rounded-[6px] border border-[#dce2ea] bg-white p-4 shadow-2xs">
          <div class="mb-3 flex items-center justify-between">
            <h3 class="text-[13px] font-semibold text-[#1e293b]">볼륨 사용량 요약</h3>
            <span class="text-[11px] text-[#64748b]">마운트 볼륨 {{ snapshot.disks.length }}개</span>
          </div>

          <div v-if="snapshot.disks.length === 0" class="py-6 text-center text-[#64748b]">
            표시할 스토리지 볼륨이 없습니다.
          </div>

          <div v-else class="space-y-3">
            <div v-for="disk in snapshot.disks" :key="disk.id" class="space-y-1.5">
              <div class="flex items-center justify-between gap-2">
                <span class="flex items-center gap-1.5 font-medium text-[#1e293b]">
                  <HardDrive class="h-3.5 w-3.5 text-[#0572ec]" />
                  {{ disk.name }}
                </span>
                <span class="tabular-nums text-[11px] text-[#475569]">
                  사용 {{ formatBytes(disk.totalBytes - disk.freeBytes) }} / {{ formatBytes(disk.totalBytes) }}
                  <strong class="ml-1 font-semibold text-[#1e293b]">
                    {{ usedPercent(disk.totalBytes, disk.freeBytes) }}%
                  </strong>
                </span>
              </div>
              <div class="h-1.5 w-full overflow-hidden rounded-full bg-[#e2e8f0]">
                <div
                  class="h-full rounded-full transition-all"
                  :class="barColorClass(usedPercent(disk.totalBytes, disk.freeBytes))"
                  :style="{ width: `${usedPercent(disk.totalBytes, disk.freeBytes)}%` }"
                />
              </div>
            </div>
          </div>
        </section>
      </div>

      <div v-else-if="activeTab === 'compute'" class="space-y-3.5">
        <section class="rounded-[6px] border border-[#dce2ea] bg-white p-4 shadow-2xs">
          <div class="mb-2.5 flex items-center justify-between">
            <div>
              <h3 class="text-[13px] font-semibold text-[#1e293b]">전체 CPU 사용률</h3>
              <p class="text-[11px] text-[#64748b]">최근 60초 실시간 추이 (2초 간격)</p>
            </div>
            <span class="text-[20px] font-bold tabular-nums text-[#0572ec]">
              {{ clampPercent(snapshot.cpuTotalPercent).toFixed(1) }}%
            </span>
          </div>

          <div class="h-40 w-full overflow-hidden rounded-[4px] border border-[#dce2ea] bg-[#f8fafc]">
            <svg viewBox="0 0 600 150" preserveAspectRatio="none" class="h-full w-full">
              <line x1="0" y1="37.5" x2="600" y2="37.5" stroke="#e2e8f0" stroke-width="1" stroke-dasharray="4 4" />
              <line x1="0" y1="75" x2="600" y2="75" stroke="#e2e8f0" stroke-width="1" stroke-dasharray="4 4" />
              <line x1="0" y1="112.5" x2="600" y2="112.5" stroke="#e2e8f0" stroke-width="1" stroke-dasharray="4 4" />
              <path :d="buildAreaPath(cpuSeries, 100, 600, 150, 4)" fill="#0572ec" fill-opacity="0.16" />
              <polyline
                :points="buildLinePoints(cpuSeries, 100, 600, 150, 4)"
                fill="none"
                stroke="#0572ec"
                stroke-width="2"
                stroke-linejoin="round"
                stroke-linecap="round"
              />
            </svg>
          </div>

          <div v-if="snapshot.cpuPerCore.length > 0" class="mt-4">
            <div class="mb-2 text-[11.5px] font-semibold text-[#334155]">
              코어별 사용률 ({{ snapshot.cpuPerCore.length }} Cores)
            </div>
            <div class="grid grid-cols-1 gap-x-4 gap-y-2 sm:grid-cols-2">
              <div
                v-for="(corePct, idx) in snapshot.cpuPerCore"
                :key="idx"
                class="flex items-center gap-2.5"
              >
                <span class="w-12 shrink-0 font-mono text-[11px] text-[#475569]">Core {{ idx }}</span>
                <div class="h-2 flex-1 overflow-hidden rounded-full bg-[#e2e8f0]">
                  <div
                    class="h-full rounded-full transition-all duration-300"
                    :class="barColorClass(clampPercent(corePct))"
                    :style="{ width: `${clampPercent(corePct)}%` }"
                  />
                </div>
                <span class="w-11 shrink-0 text-right font-mono text-[11px] tabular-nums text-[#1e293b]">
                  {{ clampPercent(corePct).toFixed(1) }}%
                </span>
              </div>
            </div>
          </div>
        </section>

        <section class="rounded-[6px] border border-[#dce2ea] bg-white p-4 shadow-2xs">
          <div class="mb-2.5 flex items-center justify-between">
            <div>
              <h3 class="text-[13px] font-semibold text-[#1e293b]">물리 메모리 &amp; 스왑</h3>
              <p class="text-[11px] tabular-nums text-[#64748b]">
                RAM {{ formatBytes(snapshot.memUsedBytes) }} / {{ formatBytes(snapshot.memTotalBytes) }}
              </p>
            </div>
            <span class="text-[20px] font-bold tabular-nums text-[#0d9488]">
              {{ clampPercent(snapshot.memUsedPercent).toFixed(1) }}%
            </span>
          </div>

          <div class="h-32 w-full overflow-hidden rounded-[4px] border border-[#dce2ea] bg-[#f8fafc]">
            <svg viewBox="0 0 600 120" preserveAspectRatio="none" class="h-full w-full">
              <line x1="0" y1="30" x2="600" y2="30" stroke="#e2e8f0" stroke-width="1" stroke-dasharray="4 4" />
              <line x1="0" y1="60" x2="600" y2="60" stroke="#e2e8f0" stroke-width="1" stroke-dasharray="4 4" />
              <line x1="0" y1="90" x2="600" y2="90" stroke="#e2e8f0" stroke-width="1" stroke-dasharray="4 4" />
              <path :d="buildAreaPath(memSeries, 100, 600, 120, 4)" fill="#0d9488" fill-opacity="0.16" />
              <polyline
                :points="buildLinePoints(memSeries, 100, 600, 120, 4)"
                fill="none"
                stroke="#0d9488"
                stroke-width="2"
                stroke-linejoin="round"
                stroke-linecap="round"
              />
            </svg>
          </div>

          <div class="mt-3.5 grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div class="rounded-[4px] border border-[#e2e8f0] bg-[#f8fafc] p-3">
              <div class="flex items-center justify-between text-[11.5px]">
                <span class="font-semibold text-[#1e293b]">물리 메모리 (RAM)</span>
                <span class="font-mono font-semibold tabular-nums text-[#0d9488]">
                  {{ clampPercent(snapshot.memUsedPercent).toFixed(1) }}%
                </span>
              </div>
              <div class="mt-2 h-2 w-full overflow-hidden rounded-full bg-[#e2e8f0]">
                <div
                  class="h-full rounded-full bg-[#0d9488] transition-all duration-300"
                  :style="{ width: `${clampPercent(snapshot.memUsedPercent)}%` }"
                />
              </div>
              <div class="mt-1.5 flex justify-between text-[11px] tabular-nums text-[#475569]">
                <span>사용 {{ formatBytes(snapshot.memUsedBytes) }}</span>
                <span>여유 {{ formatBytes(Math.max(0, snapshot.memTotalBytes - snapshot.memUsedBytes)) }}</span>
              </div>
            </div>

            <div class="rounded-[4px] border border-[#e2e8f0] bg-[#f8fafc] p-3">
              <div class="flex items-center justify-between text-[11.5px]">
                <span class="font-semibold text-[#1e293b]">스왑 메모리 (Swap)</span>
                <span class="font-mono font-semibold tabular-nums text-[#0572ec]">
                  {{ swapUsedPercent.toFixed(1) }}%
                </span>
              </div>
              <div class="mt-2 h-2 w-full overflow-hidden rounded-full bg-[#e2e8f0]">
                <div
                  class="h-full rounded-full bg-[#0572ec] transition-all duration-300"
                  :style="{ width: `${swapUsedPercent}%` }"
                />
              </div>
              <div class="mt-1.5 flex justify-between text-[11px] tabular-nums text-[#475569]">
                <span>사용 {{ formatBytes(snapshot.swapUsedBytes) }}</span>
                <span>전체 {{ formatBytes(snapshot.swapTotalBytes) }}</span>
              </div>
            </div>
          </div>
        </section>
      </div>

      <div v-else-if="activeTab === 'network'" class="space-y-3.5">
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div class="rounded-[6px] border border-[#dce2ea] bg-white p-3.5 shadow-2xs">
            <div class="flex items-center justify-between text-[11.5px] text-[#64748b]">
              <span class="flex items-center gap-1.5 font-semibold text-[#1e293b]">
                <span class="h-2.5 w-2.5 rounded-full bg-[#0572ec]" />
                수신 (RX / Download)
              </span>
              <span class="tabular-nums">최대 {{ formatKBps(netPeakRxKBps) }}</span>
            </div>
            <div class="mt-1.5 text-[20px] font-bold tabular-nums text-[#0572ec]">
              {{ formatRate(snapshot.netRxBytesPerSec) }}
            </div>
          </div>

          <div class="rounded-[6px] border border-[#dce2ea] bg-white p-3.5 shadow-2xs">
            <div class="flex items-center justify-between text-[11.5px] text-[#64748b]">
              <span class="flex items-center gap-1.5 font-semibold text-[#1e293b]">
                <span class="h-2.5 w-2.5 rounded-full bg-[#0d9488]" />
                송신 (TX / Upload)
              </span>
              <span class="tabular-nums">최대 {{ formatKBps(netPeakTxKBps) }}</span>
            </div>
            <div class="mt-1.5 text-[20px] font-bold tabular-nums text-[#0d9488]">
              {{ formatRate(snapshot.netTxBytesPerSec) }}
            </div>
          </div>
        </div>

        <section class="rounded-[6px] border border-[#dce2ea] bg-white p-4 shadow-2xs">
          <div class="mb-2.5 flex flex-wrap items-center justify-between gap-2">
            <div>
              <h3 class="text-[13px] font-semibold text-[#1e293b]">실시간 네트워크 트래픽</h3>
              <p class="text-[11px] text-[#64748b]">스케일 상한 {{ formatKBps(netMaxKBps) }} · 최근 60초</p>
            </div>
            <div class="flex items-center gap-4 text-[11px] font-medium">
              <span class="inline-flex items-center gap-1.5 text-[#0572ec]">
                <span class="h-2 w-4 rounded-full bg-[#0572ec]" />
                수신 (RX)
              </span>
              <span class="inline-flex items-center gap-1.5 text-[#0d9488]">
                <span class="h-2 w-4 rounded-full bg-[#0d9488]" />
                송신 (TX)
              </span>
            </div>
          </div>

          <div class="h-52 w-full overflow-hidden rounded-[4px] border border-[#dce2ea] bg-[#f8fafc]">
            <svg viewBox="0 0 600 190" preserveAspectRatio="none" class="h-full w-full">
              <line x1="0" y1="47.5" x2="600" y2="47.5" stroke="#e2e8f0" stroke-width="1" stroke-dasharray="4 4" />
              <line x1="0" y1="95" x2="600" y2="95" stroke="#e2e8f0" stroke-width="1" stroke-dasharray="4 4" />
              <line x1="0" y1="142.5" x2="600" y2="142.5" stroke="#e2e8f0" stroke-width="1" stroke-dasharray="4 4" />
              <path :d="buildAreaPath(netRxSeries, netMaxKBps, 600, 190, 4)" fill="#0572ec" fill-opacity="0.14" />
              <path :d="buildAreaPath(netTxSeries, netMaxKBps, 600, 190, 4)" fill="#0d9488" fill-opacity="0.12" />
              <polyline
                :points="buildLinePoints(netRxSeries, netMaxKBps, 600, 190, 4)"
                fill="none"
                stroke="#0572ec"
                stroke-width="2"
                stroke-linejoin="round"
                stroke-linecap="round"
              />
              <polyline
                :points="buildLinePoints(netTxSeries, netMaxKBps, 600, 190, 4)"
                fill="none"
                stroke="#0d9488"
                stroke-width="2"
                stroke-linejoin="round"
                stroke-linecap="round"
              />
            </svg>
          </div>
        </section>
      </div>

      <div v-else class="rounded-[6px] border border-[#dce2ea] bg-white shadow-2xs">
        <div class="border-b border-[#dce2ea] bg-[#f8fafc] px-4 py-2.5">
          <h3 class="text-[13px] font-semibold text-[#1e293b]">마운트 볼륨 디스크 현황</h3>
        </div>

        <div v-if="snapshot.disks.length === 0" class="py-10 text-center text-[#64748b]">
          연결된 마운트 볼륨이 없습니다.
        </div>

        <div v-else class="divide-y divide-[#eef2f6]">
          <div v-for="disk in snapshot.disks" :key="disk.id" class="space-y-2 p-4">
            <div class="flex flex-wrap items-center justify-between gap-2">
              <div class="flex items-center gap-2">
                <HardDrive class="h-4 w-4 text-[#0d9488]" />
                <span class="text-[13px] font-semibold text-[#1e293b]">{{ disk.name }}</span>
                <span class="rounded-[3px] border border-[#dce2ea] bg-[#f8fafc] px-1.5 py-0.5 font-mono text-[10px] text-[#64748b]">
                  {{ disk.id }}
                </span>
              </div>
              <span class="font-mono text-[12px] font-semibold tabular-nums text-[#0572ec]">
                {{ usedPercent(disk.totalBytes, disk.freeBytes) }}% 사용 중
              </span>
            </div>

            <div class="h-2 w-full overflow-hidden rounded-full bg-[#e2e8f0]">
              <div
                class="h-full rounded-full transition-all"
                :class="barColorClass(usedPercent(disk.totalBytes, disk.freeBytes))"
                :style="{ width: `${usedPercent(disk.totalBytes, disk.freeBytes)}%` }"
              />
            </div>

            <div class="flex flex-wrap items-center gap-4 text-[11px] tabular-nums text-[#475569]">
              <span>사용 용량: <strong class="font-medium text-[#1e293b]">{{ formatBytes(disk.totalBytes - disk.freeBytes) }}</strong></span>
              <span>여유 공간: <strong class="font-medium text-[#0d9488]">{{ formatBytes(disk.freeBytes) }}</strong></span>
              <span>전체 용량: <strong class="font-medium text-[#1e293b]">{{ formatBytes(disk.totalBytes) }}</strong></span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
