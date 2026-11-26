<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Folder, ArrowUp, Check, X, RefreshCw } from 'lucide-vue-next'
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
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/65 p-4"
    @click.self="emit('close')"
  >
    <div class="flex h-[520px] w-full max-w-2xl flex-col overflow-hidden rounded-2xl border border-white/15 bg-slate-900 text-xs text-slate-100 shadow-2xl">
      <!-- Header -->
      <div class="flex items-center justify-between border-b border-white/10 bg-slate-950/70 px-4 py-3">
        <div>
          <h3 class="text-sm font-semibold text-white">서버 로컬 디렉토리 선택</h3>
          <p class="text-[11px] text-slate-400">마운트할 폴더로 이동한 뒤 하단의 선택 버튼을 누르세요.</p>
        </div>
        <button
          type="button"
          class="rounded-lg p-1 text-slate-400 hover:bg-white/10 hover:text-white"
          @click="emit('close')"
        >
          <X class="h-4 w-4" />
        </button>
      </div>

      <!-- Quick Paths & Manual Path Form -->
      <div class="space-y-2 border-b border-white/10 bg-slate-950/40 px-4 py-2.5">
        <div v-if="result && result.quickPaths.length > 0" class="flex flex-wrap items-center gap-1.5">
          <button
            v-for="q in result.quickPaths"
            :key="q.path"
            type="button"
            class="rounded-lg border px-2 py-1 text-[11px] transition"
            :class="
              result.currentPath === q.path
                ? 'border-sky-400/40 bg-sky-500/20 text-sky-200'
                : 'border-white/10 bg-white/5 text-slate-300 hover:bg-white/10'
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
            class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-white/10 bg-white/5 text-slate-300 hover:bg-white/10 disabled:opacity-40"
            title="상위 폴더"
            @click="result?.parentPath && browse(result.parentPath)"
          >
            <ArrowUp class="h-3.5 w-3.5" />
          </button>

          <input
            v-model="pathInput"
            type="text"
            class="flex-1 rounded-lg border border-white/10 bg-slate-950 px-3 py-1.5 font-mono text-xs text-white focus:border-sky-400 focus:outline-none"
            placeholder="/path/to/directory"
          />

          <button
            type="submit"
            class="rounded-lg bg-white/10 px-3 py-1.5 font-medium text-white hover:bg-white/15"
          >
            이동
          </button>

          <label class="inline-flex shrink-0 items-center gap-1.5 text-[11px] text-slate-400">
            <input
              v-model="showHidden"
              type="checkbox"
              class="rounded border-white/20 bg-slate-950"
              @change="browse(result?.currentPath)"
            />
            숨김 폴더
          </label>
        </form>
      </div>

      <div v-if="errorMessage" class="border-b border-rose-500/30 bg-rose-500/15 px-4 py-2 text-rose-200">
        {{ errorMessage }}
      </div>

      <!-- Directory List -->
      <div class="min-h-0 flex-1 overflow-y-auto p-2">
        <div v-if="loading" class="flex items-center justify-center gap-2 py-12 text-slate-400">
          <RefreshCw class="h-4 w-4 animate-spin" />
          불러오는 중...
        </div>

        <div
          v-else-if="result && result.directories.length === 0"
          class="py-12 text-center text-slate-500"
        >
          하위 폴더가 없습니다. 이 폴더를 그대로 마운트할 수 있습니다.
        </div>

        <div v-else-if="result" class="grid grid-cols-1 gap-1 sm:grid-cols-2">
          <button
            v-for="dir in result.directories"
            :key="dir.path"
            type="button"
            class="flex items-center gap-2.5 rounded-lg px-3 py-2 text-left transition hover:bg-white/10"
            @click="browse(dir.path)"
          >
            <Folder class="h-4 w-4 shrink-0 text-amber-400" />
            <span class="truncate text-slate-200">{{ dir.name }}</span>
          </button>
        </div>
      </div>

      <!-- Footer -->
      <div class="flex items-center justify-between gap-3 border-t border-white/10 bg-slate-950/70 px-4 py-3">
        <div class="min-w-0 truncate font-mono text-[11px] text-slate-400">
          선택 경로: <span class="text-sky-300">{{ result?.currentPath || '-' }}</span>
        </div>

        <div class="flex shrink-0 items-center gap-2">
          <button
            type="button"
            class="rounded-lg px-3 py-1.5 text-slate-400 hover:bg-white/5"
            @click="emit('close')"
          >
            취소
          </button>
          <button
            type="button"
            :disabled="!result"
            class="inline-flex items-center gap-1.5 rounded-lg bg-sky-600 px-3.5 py-1.5 font-semibold text-white hover:bg-sky-500 disabled:opacity-40"
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
