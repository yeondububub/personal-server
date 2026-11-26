<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { Cpu, HardDrive, Server, ShieldCheck } from 'lucide-vue-next'
import { api } from '../../api/client'
import type { SystemSummary } from '../../types/api'
import { formatBytes, formatDuration } from '../../utils/format'

const summary = ref<SystemSummary | null>(null)
let timer: ReturnType<typeof setInterval> | null = null

async function load(): Promise<void> {
  try {
    summary.value = await api.fetchSystemSummary()
  } catch {
    // Ignore transient failures
  }
}

onMounted(() => {
  void load()
  timer = setInterval(() => {
    void load()
  }, 5000)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
})

function usedPercent(total: number, free: number): number {
  if (total <= 0) return 0
  return Math.min(100, Math.max(0, Math.round(((total - free) / total) * 100)))
}
</script>

<template>
  <div class="h-full w-full overflow-y-auto p-5 text-xs text-slate-200">
    <div v-if="!summary" class="py-12 text-center text-slate-400">시스템 정보를 수집하는 중...</div>

    <div v-else class="space-y-5">
      <div class="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <div class="rounded-xl border border-white/10 bg-slate-950/60 p-3.5">
          <div class="flex items-center gap-1.5 text-slate-400">
            <Server class="h-3.5 w-3.5 text-sky-400" />
            플랫폼
          </div>
          <div class="mt-1.5 text-sm font-semibold text-white">{{ summary.goOS }} ({{ summary.goArch }})</div>
        </div>

        <div class="rounded-xl border border-white/10 bg-slate-950/60 p-3.5">
          <div class="flex items-center gap-1.5 text-slate-400">
            <Cpu class="h-3.5 w-3.5 text-amber-400" />
            논리 코어 수
          </div>
          <div class="mt-1.5 text-sm font-semibold text-white">{{ summary.cpuCores }} Cores</div>
        </div>

        <div class="rounded-xl border border-white/10 bg-slate-950/60 p-3.5">
          <div class="flex items-center gap-1.5 text-slate-400">
            <ShieldCheck class="h-3.5 w-3.5 text-emerald-400" />
            접속 리스너
          </div>
          <div class="mt-1.5 text-sm font-semibold text-white">
            {{ summary.accessTier === 'private' ? 'Tailscale Private' : 'Public (NPM)' }}
          </div>
        </div>

        <div class="rounded-xl border border-white/10 bg-slate-950/60 p-3.5">
          <div class="text-slate-400">프로세스 가동 시간</div>
          <div class="mt-1.5 text-sm font-semibold tabular-nums text-white">
            {{ formatDuration(summary.uptimeSeconds) }}
          </div>
        </div>
      </div>

      <div class="rounded-xl border border-white/10 bg-slate-950/60 p-4">
        <h3 class="mb-3 text-xs font-semibold text-white">마운트 볼륨 디스크 현황</h3>
        <div class="space-y-3">
          <div v-for="disk in summary.disks" :key="disk.id" class="space-y-1.5">
            <div class="flex items-center justify-between">
              <span class="flex items-center gap-2 font-medium text-slate-200">
                <HardDrive class="h-4 w-4 text-sky-400" />
                {{ disk.name }}
              </span>
              <span class="text-slate-400">
                사용 {{ formatBytes(disk.totalBytes - disk.freeBytes) }} / 전체 {{ formatBytes(disk.totalBytes) }}
                ({{ usedPercent(disk.totalBytes, disk.freeBytes) }}%)
              </span>
            </div>
            <div class="h-2 w-full overflow-hidden rounded-full bg-slate-800">
              <div
                class="h-full rounded-full bg-gradient-to-r from-sky-500 to-blue-500"
                :style="{ width: `${usedPercent(disk.totalBytes, disk.freeBytes)}%` }"
              />
            </div>
          </div>
        </div>
      </div>

      <p class="text-[11px] text-slate-500">
        코어별 실시간 CPU 그래프, 호스트 전체 RAM/스왑 및 네트워크 트래픽 WebSocket 스트리밍은 Phase 3에서 확장됩니다.
      </p>
    </div>
  </div>
</template>
