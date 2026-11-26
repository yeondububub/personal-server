<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ArrowUp, Check, Folder, RefreshCw, X } from 'lucide-vue-next'
import { api } from '../../api/client'
import type { HostBrowseResult } from '../../types/api'

const props = defineProps<{
  readonly initialPath?: string
}>()

const emit = defineEmits<{
  select: [path: string]
  close: []
}>()

const result = ref<HostBrowseResult | null>(null)
const pathInput = ref(props.initialPath ?? '')
const showHidden = ref(false)
const loading = ref(false)
const errorMessage = ref('')

async function browse(targetPath?: string): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    const res = await api.browseHostDir({
      path: targetPath,
      showHidden: showHidden.value,
    })
    result.value = res
    pathInput.value = res.currentPath
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '디렉토리 탐색에 실패했습니다.'
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  void browse(props.initialPath || undefined)
})
</script>

<template>
  <div
    class="fixed inset-0 z-[60] flex items-center justify-center bg-slate-900/35 p-4"
    @click.self="emit('close')"
  >
    <div class="flex h-[500px] w-full max-w-2xl flex-col overflow-hidden rounded-[6px] border border-[#cbd5e1] bg-white text-[12px] text-[#1e293b] shadow-2xl">
      <div class="flex items-center justify-between border-b border-[#dce2ea] bg-[#eef1f5] px-4 py-2.5">
        <div>
          <h3 class="text-[13px] font-semibold text-[#1e293b]">서버 로컬 디렉토리 선택</h3>
          <p class="text-[11px] text-[#64748b]">마운트할 폴더로 이동한 뒤 하단의 선택 버튼을 누르세요.</p>
        </div>
        <button
          type="button"
          class="rounded-[4px] p-1 text-[#475569] hover:bg-[#dce2ea] hover:text-[#1e293b]"
          @click="emit('close')"
        >
          <X class="h-4 w-4" />
        </button>
      </div>

      <div class="space-y-2 border-b border-[#dce2ea] bg-[#f8fafc] px-4 py-2.5">
        <div v-if="result && result.quickPaths.length > 0" class="flex flex-wrap items-center gap-1.5">
          <button
            v-for="q in result.quickPaths"
            :key="q.path"
            type="button"
            class="dsm-btn"
            :class="
              result.currentPath === q.path
                ? '!border-[#0572ec] !bg-[#e6f0ff] !text-[#0058cc] font-semibold'
                : ''
            "
            @click="browse(q.path)"
          >
            {{ q.label }}
          </button>
        </div>

        <form class="flex items-center gap-2" @submit.prevent="browse(pathInput)">
          <button
            type="button"
            :disabled="!result || !result.parentPath"
            class="dsm-btn w-8 shrink-0 !px-0"
            title="상위 폴더"
            @click="result?.parentPath && browse(result.parentPath)"
          >
            <ArrowUp class="h-3.5 w-3.5" />
          </button>

          <input
            v-model="pathInput"
            type="text"
            class="dsm-input flex-1 font-mono"
            placeholder="/path/to/directory"
          />

          <button type="submit" class="dsm-btn shrink-0">
            이동
          </button>

          <label class="inline-flex shrink-0 items-center gap-1.5 text-[11px] text-[#475569]">
            <input
              v-model="showHidden"
              type="checkbox"
              class="accent-[#0572ec]"
              @change="browse(result?.currentPath)"
            />
            숨김 폴더
          </label>
        </form>
      </div>

      <div
        v-if="errorMessage"
        class="border-b border-[#fecdd3] bg-[#fff1f2] px-4 py-2 text-[#be123c]"
      >
        {{ errorMessage }}
      </div>

      <div class="min-h-0 flex-1 overflow-y-auto bg-white p-2">
        <div v-if="loading" class="flex items-center justify-center gap-2 py-12 text-[#64748b]">
          <RefreshCw class="h-4 w-4 animate-spin text-[#0572ec]" />
          불러오는 중...
        </div>

        <div
          v-else-if="result && result.directories.length === 0"
          class="py-12 text-center text-[#64748b]"
        >
          하위 폴더가 없습니다. 현재 폴더를 그대로 마운트할 수 있습니다.
        </div>

        <div v-else-if="result" class="grid grid-cols-1 gap-0.5 sm:grid-cols-2">
          <button
            v-for="dir in result.directories"
            :key="dir.path"
            type="button"
            class="flex items-center gap-2 rounded-[4px] px-2.5 py-1.5 text-left text-[#1e293b] transition hover:bg-[#f1f5f9]"
            @click="browse(dir.path)"
          >
            <Folder class="h-4 w-4 shrink-0 fill-[#fbbf24] text-[#d97706]" />
            <span class="truncate">{{ dir.name }}</span>
          </button>
        </div>
      </div>

      <div class="flex items-center justify-between gap-3 border-t border-[#dce2ea] bg-[#f8fafc] px-4 py-2.5">
        <div class="min-w-0 truncate font-mono text-[11px] text-[#64748b]">
          선택 경로: <span class="font-medium text-[#0058cc]">{{ result?.currentPath || '-' }}</span>
        </div>

        <div class="flex shrink-0 items-center gap-2">
          <button type="button" class="dsm-btn" @click="emit('close')">
            취소
          </button>
          <button
            type="button"
            :disabled="!result"
            class="dsm-btn dsm-btn-primary"
            @click="result && emit('select', result.currentPath)"
          >
            <Check class="h-3.5 w-3.5" />
            현재 폴더 선택
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
