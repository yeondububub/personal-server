<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { Activity, Clock, Cpu, Database, HardDrive, Server, ShieldCheck } from 'lucide-vue-next'
import { api } from '../../api/client'
import type { SystemSummary } from '../../types/api'
import { formatBytes, formatDuration } from '../../utils/format'

type MonitorTab = 'overview' | 'storage'

const summary = ref<SystemSummary | null>(null)
const activeTab = ref<MonitorTab>('overview')
let timer: ReturnType<typeof setInterval> | null = null

async function load(): Promise<void> {
  try {
    summary.value = await api.fetchSystemSummary()
  } catch {
    return
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

function barColorClass(percent: number): string {
  if (percent >= 90) return 'bg-[#e11d48]'
  if (percent >= 75) return 'bg-[#0d9488]'
  return 'bg-[#0572ec]'
}
</script>

<template>
  <div class="flex h-full w-full overflow-hidden bg-[#f4f6f9] text-[12px] text-[#1e293b]">
    <aside class="hidden w-40 shrink-0 flex-col gap-1 border-r border-[#dce2ea] bg-[#f5f7fa] p-2 sm:flex">
      <button
        type="button"
        class="flex items-center gap-2 rounded-[4px] px-2.5 py-2 text-left text-[12px] transition"
        :class="
          activeTab === 'overview'
            ? 'bg-[#e6f0ff] font-semibold text-[#0058cc]'
            : 'text-[#334155] hover:bg-[#eaeef4]'
        "
        @click="activeTab = 'overview'"
      >
        <Activity class="h-4 w-4 shrink-0" />
        <span>개요</span>
      </button>

      <button
        type="button"
        class="flex items-center gap-2 rounded-[4px] px-2.5 py-2 text-left text-[12px] transition"
        :class="
          activeTab === 'storage'
            ? 'bg-[#e6f0ff] font-semibold text-[#0058cc]'
            : 'text-[#334155] hover:bg-[#eaeef4]'
        "
        @click="activeTab = 'storage'"
      >
        <HardDrive class="h-4 w-4 shrink-0" />
        <span>스토리지</span>
      </button>
    </aside>

    <div class="flex min-w-0 flex-1 flex-col overflow-y-auto p-4">
      <div class="mb-3 flex items-center gap-1.5 sm:hidden">
        <button
          type="button"
          class="dsm-btn"
          :class="activeTab === 'overview' ? '!border-[#0572ec] !bg-[#e6f0ff] !text-[#0058cc] font-semibold' : ''"
          @click="activeTab = 'overview'"
        >
          개요
        </button>
        <button
          type="button"
          class="dsm-btn"
          :class="activeTab === 'storage' ? '!border-[#0572ec] !bg-[#e6f0ff] !text-[#0058cc] font-semibold' : ''"
          @click="activeTab = 'storage'"
        >
          스토리지
        </button>
      </div>

      <div
        v-if="!summary"
        class="flex flex-1 items-center justify-center rounded-[6px] border border-[#dce2ea] bg-white py-12 text-[#64748b]"
      >
        시스템 리소스 정보를 수집하는 중...
      </div>

      <div v-else-if="activeTab === 'overview'" class="space-y-3.5">
        <div class="grid grid-cols-2 gap-3 lg:grid-cols-5">
          <div class="rounded-[6px] border border-[#dce2ea] bg-white p-3 shadow-2xs">
            <div class="flex items-center gap-1.5 text-[11px] text-[#64748b]">
              <Server class="h-3.5 w-3.5 text-[#0572ec]" />
              플랫폼
            </div>
            <div class="mt-1.5 text-[13px] font-semibold text-[#1e293b]">
              {{ summary.goOS }} ({{ summary.goArch }})
            </div>
          </div>

          <div class="rounded-[6px] border border-[#dce2ea] bg-white p-3 shadow-2xs">
            <div class="flex items-center gap-1.5 text-[11px] text-[#64748b]">
              <Cpu class="h-3.5 w-3.5 text-[#0d9488]" />
              논리 코어 수
            </div>
            <div class="mt-1.5 text-[13px] font-semibold tabular-nums text-[#1e293b]">
              {{ summary.cpuCores }} Cores
            </div>
          </div>

          <div class="rounded-[6px] border border-[#dce2ea] bg-white p-3 shadow-2xs">
            <div class="flex items-center gap-1.5 text-[11px] text-[#64748b]">
              <Database class="h-3.5 w-3.5 text-[#0572ec]" />
              앱 메모리
            </div>
            <div class="mt-1.5 text-[13px] font-semibold tabular-nums text-[#1e293b]">
              {{ formatBytes(summary.appMemoryBytes) }}
            </div>
          </div>

          <div class="rounded-[6px] border border-[#dce2ea] bg-white p-3 shadow-2xs">
            <div class="flex items-center gap-1.5 text-[11px] text-[#64748b]">
              <ShieldCheck class="h-3.5 w-3.5 text-[#0d9488]" />
              접속 리스너
            </div>
            <div class="mt-1.5 text-[13px] font-semibold text-[#1e293b]">
              {{ summary.accessTier === 'private' ? 'Tailscale Private' : 'Public (NPM)' }}
            </div>
          </div>

          <div class="col-span-2 rounded-[6px] border border-[#dce2ea] bg-white p-3 shadow-2xs lg:col-span-1">
            <div class="flex items-center gap-1.5 text-[11px] text-[#64748b]">
              <Clock class="h-3.5 w-3.5 text-[#0572ec]" />
              가동 시간
            </div>
            <div class="mt-1.5 text-[13px] font-semibold tabular-nums text-[#1e293b]">
              {{ formatDuration(summary.uptimeSeconds) }}
            </div>
          </div>
        </div>

        <div class="rounded-[6px] border border-[#dce2ea] bg-white p-4 shadow-2xs">
          <div class="mb-3 flex items-center justify-between">
            <h3 class="text-[13px] font-semibold text-[#1e293b]">볼륨 사용량 요약</h3>
            <span class="text-[11px] text-[#64748b]">마운트 볼륨 {{ summary.disks.length }}개</span>
          </div>

          <div v-if="summary.disks.length === 0" class="py-6 text-center text-[#64748b]">
            표시할 스토리지 볼륨이 없습니다.
          </div>

          <div v-else class="space-y-3">
            <div v-for="disk in summary.disks" :key="disk.id" class="space-y-1.5">
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
        </div>

        <p class="px-0.5 text-[11px] text-[#64748b]">
          코어별 실시간 CPU 그래프, 호스트 전체 RAM/스왑 및 네트워크 트래픽 WebSocket 스트리밍은 Phase 3에서 확장됩니다.
        </p>
      </div>

      <div v-else class="rounded-[6px] border border-[#dce2ea] bg-white shadow-2xs">
        <div class="border-b border-[#dce2ea] bg-[#f8fafc] px-4 py-2.5">
          <h3 class="text-[13px] font-semibold text-[#1e293b]">마운트 볼륨 디스크 현황</h3>
        </div>

        <div v-if="summary.disks.length === 0" class="py-10 text-center text-[#64748b]">
          연결된 마운트 볼륨이 없습니다.
        </div>

        <div v-else class="divide-y divide-[#eef2f6]">
          <div v-for="disk in summary.disks" :key="disk.id" class="space-y-2 p-4">
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
