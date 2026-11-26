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
    // Clipboard API unavailable on non-HTTPS origins
  }
}

onMounted(() => {
  void refreshShares()
})
</script>

<template>
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/45 p-4"
    @click.self="emit('close')"
  >
    <div class="w-full max-w-lg overflow-hidden rounded-[6px] border border-[#cbd5e1] bg-white text-[#1e293b] shadow-xl">
      <div class="flex items-center justify-between border-b border-[#dce2ea] bg-[#f4f6f9] px-4 py-2.5">
        <div class="flex min-w-0 items-center gap-2">
          <Link2 class="h-4 w-4 shrink-0 text-[#0572ec]" />
          <div class="min-w-0">
            <h3 class="text-[13px] font-semibold text-[#1e293b]">공유 링크 관리</h3>
            <p class="truncate text-[11px] text-[#64748b]">{{ targetName }}</p>
          </div>
        </div>
        <button
          type="button"
          class="flex h-6 w-6 shrink-0 cursor-pointer items-center justify-center rounded-[4px] text-[#64748b] hover:bg-black/6 hover:text-[#1e293b]"
          @click="emit('close')"
        >
          <X class="h-3.5 w-3.5" />
        </button>
      </div>

      <div class="space-y-4 p-4">
        <form class="space-y-3 rounded-[4px] border border-[#dce2ea] bg-[#f8fafc] p-3.5" @submit.prevent="handleCreate">
          <div class="grid grid-cols-2 gap-3">
            <div>
              <label class="mb-1 block text-[11.5px] font-medium text-[#475569]">유효 기간</label>
              <select
                v-model.number="expiresHours"
                class="dsm-select w-full"
              >
                <option :value="3">3시간</option>
                <option :value="24">24시간 (1일)</option>
                <option :value="72">72시간 (3일)</option>
                <option :value="168">7일</option>
                <option :value="720">30일</option>
              </select>
            </div>

            <div>
              <label class="mb-1 block text-[11.5px] font-medium text-[#475569]">비밀번호 보호 (선택)</label>
              <input
                v-model="password"
                type="password"
                placeholder="비워두면 공개 링크"
                class="dsm-input w-full"
              />
            </div>
          </div>

          <div class="flex flex-wrap items-center justify-between gap-2 pt-0.5">
            <label class="inline-flex cursor-pointer items-center gap-2 text-[12px] text-[#334155]">
              <input
                v-model="allowDownload"
                type="checkbox"
                class="h-3.5 w-3.5 rounded border-[#cbd5e1] accent-[#0572ec]"
              />
              <span>파일 다운로드 허용 (해제 시 영상 스트리밍만 제공)</span>
            </label>

            <button
              type="submit"
              :disabled="creating"
              class="dsm-btn dsm-btn-primary"
            >
              {{ creating ? '생성 중...' : '공유 링크 생성' }}
            </button>
          </div>
        </form>

        <p v-if="errorMessage" class="text-[12px] font-medium text-[#c81e3a]">{{ errorMessage }}</p>

        <div>
          <div class="mb-1.5 text-[11.5px] font-semibold text-[#475569]">활성 공유 링크</div>
          <div class="max-h-52 space-y-2 overflow-y-auto">
            <div
              v-if="filteredShares.length === 0"
              class="rounded-[4px] border border-[#e2e8f0] bg-[#f8fafc] py-5 text-center text-[12px] text-[#64748b]"
            >
              이 항목에 생성된 공유 링크가 없습니다.
            </div>

            <div
              v-for="item in filteredShares"
              :key="item.token"
              class="flex items-center justify-between gap-2 rounded-[4px] border border-[#dce2ea] bg-white px-3 py-2"
            >
              <div class="min-w-0 flex-1">
                <div class="flex items-center gap-1.5 text-[12px] font-medium text-[#0058cc]">
                  <Lock v-if="item.hasPassword" class="h-3 w-3 shrink-0 text-[#d97706]" />
                  <span class="truncate">{{ buildShareUrl(item.token) }}</span>
                </div>
                <div class="mt-0.5 text-[11px] text-[#64748b]">
                  만료 {{ formatDateTime(item.expiresAt) }} ·
                  {{ item.allowDownload ? '다운로드 허용' : '스트리밍 전용' }}
                </div>
              </div>

              <div class="flex shrink-0 items-center gap-1.5">
                <button
                  type="button"
                  class="dsm-btn"
                  @click="copyLink(item.token)"
                >
                  <Check v-if="copiedToken === item.token" class="h-3.5 w-3.5 text-[#10b981]" />
                  <Copy v-else class="h-3.5 w-3.5 text-[#475569]" />
                  <span>{{ copiedToken === item.token ? '복사됨' : '링크 복사' }}</span>
                </button>
                <button
                  type="button"
                  class="dsm-btn dsm-btn-danger !px-2"
                  title="링크 삭제"
                  @click="handleRevoke(item.token)"
                >
                  <Trash2 class="h-3.5 w-3.5" />
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div class="flex items-center justify-end border-t border-[#dce2ea] bg-[#f8fafc] px-4 py-2.5">
        <button
          type="button"
          class="dsm-btn"
          @click="emit('close')"
        >
          닫기
        </button>
      </div>
    </div>
  </div>
</template>
