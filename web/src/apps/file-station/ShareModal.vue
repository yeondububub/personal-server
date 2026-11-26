<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { Copy, Check, Trash2, Link2, Lock, X } from 'lucide-vue-next'
import { api } from '../../api/client'
import type { ShareItem } from '../../types/api'
import { formatDateTime } from '../../utils/format'

const props = defineProps<{
  readonly targetRootId: string
  readonly targetRelPath: string
  readonly targetName: string
}>()

const emit = defineEmits<{
  close: []
}>()

const shares = ref<readonly ShareItem[]>([])
const publicBaseUrl = ref('')
const expiresHours = ref(24)
const password = ref('')
const allowDownload = ref(true)
const creating = ref(false)
const copiedToken = ref('')
const errorMessage = ref('')

const filteredShares = computed(() =>
  shares.value.filter(
    (s) => s.rootId === props.targetRootId && s.relPath === props.targetRelPath,
  ),
)

function buildShareUrl(token: string): string {
  const base = publicBaseUrl.value !== '' ? publicBaseUrl.value : window.location.origin
  return `${base.replace(/\/$/, '')}/s/${encodeURIComponent(token)}`
}

async function refreshShares(): Promise<void> {
  try {
    const res = await api.listShares()
    shares.value = res.shares
    publicBaseUrl.value = res.publicBaseUrl
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '공유 목록 조회 실패'
  }
}

async function handleCreate(): Promise<void> {
  creating.value = true
  errorMessage.value = ''
  try {
    const created = await api.createShare({
      rootId: props.targetRootId,
      relPath: props.targetRelPath,
      password: password.value,
      expiresHours: expiresHours.value,
      allowDownload: allowDownload.value,
    })
    password.value = ''
    publicBaseUrl.value = created.publicBaseUrl
    await refreshShares()
    await copyLink(created.share.token)
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '공유 링크 생성 실패'
  } finally {
    creating.value = false
  }
}

async function handleRevoke(token: string): Promise<void> {
  try {
    await api.deleteShare(token)
    await refreshShares()
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '공유 링크 삭제 실패'
  }
}

async function copyLink(token: string): Promise<void> {
  const url = buildShareUrl(token)
  try {
    await navigator.clipboard.writeText(url)
    copiedToken.value = token
    setTimeout(() => {
      if (copiedToken.value === token) copiedToken.value = ''
    }, 2000)
  } catch {
    // Clipboard API might be blocked on insecure origins
  }
}

onMounted(() => {
  void refreshShares()
})
</script>

<template>
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
    @click.self="emit('close')"
  >
    <div class="w-full max-w-lg rounded-2xl border border-white/15 bg-slate-900 p-5 text-slate-100 shadow-2xl">
      <div class="mb-4 flex items-center justify-between border-b border-white/10 pb-3">
        <div class="flex items-center gap-2.5">
          <Link2 class="h-4 w-4 text-sky-400" />
          <div>
            <h3 class="text-sm font-semibold">임시 공유 링크 관리</h3>
            <p class="text-xs text-slate-400">{{ targetName }}</p>
          </div>
        </div>
        <button
          type="button"
          class="rounded-lg p-1 text-slate-400 hover:bg-white/10 hover:text-white"
          @click="emit('close')"
        >
          <X class="h-4 w-4" />
        </button>
      </div>

      <form class="mb-5 space-y-3 rounded-xl border border-white/10 bg-slate-950/60 p-3.5" @submit.prevent="handleCreate">
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="mb-1 block text-[11px] text-slate-400">만료 시간</label>
            <select
              v-model.number="expiresHours"
              class="w-full rounded-lg border border-white/10 bg-slate-900 px-2.5 py-1.5 text-xs text-white focus:border-sky-400 focus:outline-none"
            >
              <option :value="3">3시간</option>
              <option :value="24">24시간 (1일)</option>
              <option :value="72">72시간 (3일)</option>
              <option :value="168">7일</option>
              <option :value="720">30일</option>
            </select>
          </div>

          <div>
            <label class="mb-1 block text-[11px] text-slate-400">비밀번호 (선택)</label>
            <input
              v-model="password"
              type="password"
              placeholder="비워두면 공개"
              class="w-full rounded-lg border border-white/10 bg-slate-900 px-2.5 py-1.5 text-xs text-white focus:border-sky-400 focus:outline-none"
            />
          </div>
        </div>

        <div class="flex items-center justify-between pt-1">
          <label class="inline-flex items-center gap-2 text-xs text-slate-300">
            <input v-model="allowDownload" type="checkbox" class="rounded border-white/20 bg-slate-900" />
            원본 파일 다운로드 허용 (영상은 해제해도 스트리밍 가능)
          </label>

          <button
            type="submit"
            :disabled="creating"
            class="rounded-lg bg-sky-600 px-3.5 py-1.5 text-xs font-semibold text-white hover:bg-sky-500 disabled:opacity-50"
          >
            {{ creating ? '생성 중...' : '새 링크 발급' }}
          </button>
        </div>
      </form>

      <p v-if="errorMessage" class="mb-3 text-xs text-rose-300">{{ errorMessage }}</p>

      <div class="max-h-52 space-y-2 overflow-y-auto">
        <div v-if="filteredShares.length === 0" class="py-4 text-center text-xs text-slate-500">
          이 항목에 발급된 활성 공유 링크가 없습니다.
        </div>

        <div
          v-for="item in filteredShares"
          :key="item.token"
          class="flex items-center justify-between gap-2 rounded-xl border border-white/10 bg-slate-950/50 px-3 py-2.5"
        >
          <div class="min-w-0 flex-1">
            <div class="flex items-center gap-1.5 text-xs font-medium text-sky-300">
              <Lock v-if="item.hasPassword" class="h-3 w-3 text-amber-300" />
              <span class="truncate">{{ buildShareUrl(item.token) }}</span>
            </div>
            <div class="mt-0.5 text-[11px] text-slate-400">
              만료 {{ formatDateTime(item.expiresAt) }} ·
              {{ item.allowDownload ? '다운로드 허용' : '스트리밍 전용' }}
            </div>
          </div>

          <div class="flex shrink-0 items-center gap-1">
            <button
              type="button"
              class="inline-flex items-center gap-1 rounded-lg bg-white/10 px-2.5 py-1 text-xs text-slate-200 hover:bg-white/15"
              @click="copyLink(item.token)"
            >
              <Check v-if="copiedToken === item.token" class="h-3.5 w-3.5 text-emerald-400" />
              <Copy v-else class="h-3.5 w-3.5" />
              {{ copiedToken === item.token ? '복사됨' : '복사' }}
            </button>
            <button
              type="button"
              class="rounded-lg p-1.5 text-slate-400 hover:bg-rose-500/20 hover:text-rose-300"
              title="링크 폐기"
              @click="handleRevoke(item.token)"
            >
              <Trash2 class="h-3.5 w-3.5" />
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
