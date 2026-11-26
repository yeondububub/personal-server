<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import {
  Folder,
  Film,
  FileCode2,
  FileText,
  Image,
  Music,
  FolderArchive,
  Upload,
  FolderPlus,
  RefreshCw,
  Download,
  Share2,
  Pencil,
  Trash2,
  ChevronRight,
  FilePlus2,
  Settings2,
  Lock,
  Cpu,
  X,
} from 'lucide-vue-next'
import { api } from '../../api/client'
import { useAuthStore } from '../../stores/auth'
import { useDesktopStore } from '../../stores/desktop'
import { useMountsStore } from '../../stores/mounts'
import type { FileEntry, FileKind, MountRoot } from '../../types/api'
import { formatBytes, formatDateTime } from '../../utils/format'
import ShareModal from './ShareModal.vue'

const props = defineProps<{
  readonly windowId: string
  readonly initialRootId?: string
  readonly initialPath?: string
}>()

const authStore = useAuthStore()
const desktopStore = useDesktopStore()
const mountsStore = useMountsStore()

const roots = ref<readonly MountRoot[]>([])
const currentRootId = ref('')
const currentRelPath = ref('')
const currentReadOnly = ref(false)
const entries = ref<readonly FileEntry[]>([])
const selectedRelPath = ref<string | null>(null)
const loading = ref(false)
const errorMessage = ref('')
const dragOver = ref(false)

const uploadProgress = ref<{
  readonly active: boolean
  readonly fileName: string
  readonly percent: number
}>({ active: false, fileName: '', percent: 0 })

const fileInputRef = ref<HTMLInputElement | null>(null)

type DialogState =
  | { readonly kind: 'none' }
  | { readonly kind: 'mkdir'; value: string }
  | { readonly kind: 'new-file'; value: string }
  | { readonly kind: 'rename'; readonly entry: FileEntry; value: string }
  | { readonly kind: 'delete'; readonly entry: FileEntry }

const dialog = ref<DialogState>({ kind: 'none' })
const shareTarget = ref<{ readonly rootId: string; readonly relPath: string; readonly name: string } | null>(null)

const selectedEntry = computed<FileEntry | null>(() => {
  if (!selectedRelPath.value) return null
  return entries.value.find((e) => e.relPath === selectedRelPath.value) ?? null
})

const breadcrumbs = computed(() => {
  if (!currentRelPath.value) return []
  const parts = currentRelPath.value.split('/').filter(Boolean)
  return parts.map((segment, idx) => ({
    name: segment,
    relPath: parts.slice(0, idx + 1).join('/'),
  }))
})

async function loadDirectory(rootId: string, relPath: string): Promise<void> {
  if (!rootId) return
  loading.value = true
  errorMessage.value = ''
  try {
    const res = await api.listFiles({ rootId, relPath })
    currentRootId.value = res.rootId
    currentRelPath.value = res.relPath
    currentReadOnly.value = res.readOnly
    entries.value = res.entries
    selectedRelPath.value = null
    const rootName = roots.value.find((r) => r.id === res.rootId)?.name ?? res.rootId
    const suffix = res.relPath ? ` / ${res.relPath}` : ''
    desktopStore.setWindowTitle({
      windowId: props.windowId,
      title: `File Station - ${rootName}${suffix}`,
    })
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '디렉토리 조회 실패'
  } finally {
    loading.value = false
  }
}

async function refreshRoots(preferredRoot?: string, preferredPath?: string): Promise<void> {
  try {
    roots.value = await api.fetchRoots()
    const targetRoot = preferredRoot ?? currentRootId.value
    const stillExists = roots.value.some((r) => r.id === targetRoot)
    if (stillExists && targetRoot) {
      await loadDirectory(targetRoot, preferredPath ?? currentRelPath.value)
      return
    }
    const firstRoot = roots.value[0]
    if (firstRoot) {
      await loadDirectory(firstRoot.id, '')
    } else {
      currentRootId.value = ''
      currentRelPath.value = ''
      currentReadOnly.value = false
      entries.value = []
    }
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '마운트 목록을 불러오지 못했습니다.'
  }
}

onMounted(() => {
  void refreshRoots(props.initialRootId, props.initialPath)
})

watch(
  () => mountsStore.version,
  () => {
    void refreshRoots()
  },
)

function openMountManager(): void {
  desktopStore.openWindow({
    title: '마운트 관리',
    payload: { kind: 'mounts' },
  })
}

function handleOpenEntry(entry: FileEntry): void {
  if (entry.isDir) {
    void loadDirectory(currentRootId.value, entry.relPath)
    return
  }
  if (entry.kind === 'video') {
    desktopStore.openWindow({
      title: `Media Player - ${entry.name}`,
      payload: {
        kind: 'video-player',
        rootId: currentRootId.value,
        relPath: entry.relPath,
        fileName: entry.name,
      },
    })
    return
  }
  if (entry.kind === 'code') {
    desktopStore.openWindow({
      title: `텍스트 편집기 - ${entry.name}`,
      payload: {
        kind: 'code-editor',
        rootId: currentRootId.value,
        relPath: entry.relPath,
        fileName: entry.name,
      },
    })
    return
  }
  window.open(api.downloadUrl({ rootId: currentRootId.value, relPath: entry.relPath }), '_blank')
}

function openSelectedInEditor(): void {
  const entry = selectedEntry.value
  if (!entry || entry.isDir) return
  desktopStore.openWindow({
    title: `텍스트 편집기 - ${entry.name}`,
    payload: {
      kind: 'code-editor',
      rootId: currentRootId.value,
      relPath: entry.relPath,
      fileName: entry.name,
    },
  })
}

function launchSelectedInWargame(): void {
  const entry = selectedEntry.value
  if (!entry || (!entry.isDir && entry.kind !== 'archive') || !currentRootId.value) return
  desktopStore.openWindow({
    title: 'Wargame Runner',
    payload: {
      kind: 'wargame',
      launchSource: {
        rootId: currentRootId.value,
        relPath: entry.relPath,
        name: entry.name,
        nonce: Date.now(),
      },
    },
  })
}

function openRenameDialog(): void {
  const entry = selectedEntry.value
  if (!entry || currentReadOnly.value) return
  dialog.value = { kind: 'rename', entry, value: entry.name }
}

function openDeleteDialog(): void {
  const entry = selectedEntry.value
  if (!entry || currentReadOnly.value) return
  dialog.value = { kind: 'delete', entry }
}

async function uploadFiles(fileList: FileList | readonly File[]): Promise<void> {
  const arr = Array.from(fileList)
  if (arr.length === 0 || !currentRootId.value) return
  errorMessage.value = ''

  for (const file of arr) {
    uploadProgress.value = { active: true, fileName: file.name, percent: 0 }
    try {
      await api.uploadFileChunked({
        rootId: currentRootId.value,
        dirPath: currentRelPath.value,
        file,
        onProgress: (uploaded, total) => {
          const pct = total > 0 ? Math.round((uploaded / total) * 100) : 100
          uploadProgress.value = { active: true, fileName: file.name, percent: pct }
        },
      })
    } catch (err) {
      errorMessage.value = err instanceof Error ? err.message : `${file.name} 업로드 실패`
      break
    }
  }

  uploadProgress.value = { active: false, fileName: '', percent: 0 }
  await loadDirectory(currentRootId.value, currentRelPath.value)
}

function handleFileInputChange(ev: Event): void {
  const input = ev.target
  if (input instanceof HTMLInputElement && input.files) {
    void uploadFiles(input.files)
    input.value = ''
  }
}

function handleDrop(ev: DragEvent): void {
  dragOver.value = false
  if (currentReadOnly.value) {
    errorMessage.value = '읽기 전용으로 마운트된 폴더에는 업로드할 수 없습니다.'
    return
  }
  if (ev.dataTransfer?.files && ev.dataTransfer.files.length > 0) {
    void uploadFiles(ev.dataTransfer.files)
  }
}

function joinRelPath(base: string, leaf: string): string {
  const cleanLeaf = leaf.trim().replace(/^\/+|\/+$/g, '')
  return base ? `${base}/${cleanLeaf}` : cleanLeaf
}

async function submitDialog(): Promise<void> {
  const d = dialog.value
  if (d.kind === 'none') return
  errorMessage.value = ''

  try {
    if (d.kind === 'mkdir') {
      if (!d.value.trim()) return
      await api.mkdir({
        rootId: currentRootId.value,
        relPath: joinRelPath(currentRelPath.value, d.value),
      })
    } else if (d.kind === 'new-file') {
      if (!d.value.trim()) return
      const relPath = joinRelPath(currentRelPath.value, d.value)
      await api.writeTextFile({
        rootId: currentRootId.value,
        relPath,
        content: '',
      })
      dialog.value = { kind: 'none' }
      await loadDirectory(currentRootId.value, currentRelPath.value)
      desktopStore.openWindow({
        title: `텍스트 편집기 - ${d.value.trim()}`,
        payload: {
          kind: 'code-editor',
          rootId: currentRootId.value,
          relPath,
          fileName: d.value.trim(),
        },
      })
      return
    } else if (d.kind === 'rename') {
      if (!d.value.trim() || d.value.trim() === d.entry.name) {
        dialog.value = { kind: 'none' }
        return
      }
      const parentParts = d.entry.relPath.split('/')
      parentParts.pop()
      const parentRel = parentParts.join('/')
      await api.renameEntry({
        rootId: currentRootId.value,
        fromRel: d.entry.relPath,
        toRel: joinRelPath(parentRel, d.value),
      })
    } else if (d.kind === 'delete') {
      await api.deleteEntry({
        rootId: currentRootId.value,
        relPath: d.entry.relPath,
      })
    }

    dialog.value = { kind: 'none' }
    await loadDirectory(currentRootId.value, currentRelPath.value)
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '작업 처리 실패'
  }
}

function openShareForSelection(): void {
  if (!currentRootId.value) return
  const entry = selectedEntry.value
  if (entry) {
    shareTarget.value = {
      rootId: currentRootId.value,
      relPath: entry.relPath,
      name: entry.name,
    }
    return
  }
  const rootName = roots.value.find((r) => r.id === currentRootId.value)?.name ?? currentRootId.value
  shareTarget.value = {
    rootId: currentRootId.value,
    relPath: currentRelPath.value,
    name: currentRelPath.value || rootName,
  }
}

function downloadSelectionOrFolder(): void {
  if (!currentRootId.value) return
  const targetRel = selectedEntry.value ? selectedEntry.value.relPath : currentRelPath.value
  window.open(api.downloadUrl({ rootId: currentRootId.value, relPath: targetRel }), '_blank')
}

function iconForKind(kind: FileKind) {
  switch (kind) {
    case 'dir':
      return Folder
    case 'video':
      return Film
    case 'code':
      return FileCode2
    case 'image':
      return Image
    case 'audio':
      return Music
    case 'archive':
      return FolderArchive
    case 'file':
      return FileText
    default: {
      const _exhaustive: never = kind
      return _exhaustive
    }
  }
}

function iconColorForKind(kind: FileKind): string {
  switch (kind) {
    case 'dir':
      return 'text-[#f59e0b] fill-[#fde68a]'
    case 'video':
      return 'text-[#e11d48]'
    case 'code':
      return 'text-[#0572ec]'
    case 'image':
      return 'text-[#0284c7]'
    case 'audio':
      return 'text-[#2563eb]'
    case 'archive':
      return 'text-[#64748b]'
    case 'file':
      return 'text-[#64748b]'
    default: {
      const _exhaustive: never = kind
      return _exhaustive
    }
  }
}
</script>

<template>
  <div
    class="relative flex h-full w-full flex-col bg-white text-[12px] text-[#1e293b]"
    @dragover.prevent="dragOver = true"
    @dragleave.prevent="dragOver = false"
    @drop.prevent="handleDrop"
  >
    <div class="flex flex-wrap items-center justify-between gap-2 border-b border-[#dce2ea] bg-[#f8fafc] px-2.5 py-1.5">
      <div class="flex flex-wrap items-center gap-1.5">
        <input
          ref="fileInputRef"
          type="file"
          multiple
          class="hidden"
          @change="handleFileInputChange"
        />

        <button
          v-if="!currentReadOnly"
          type="button"
          class="dsm-btn dsm-btn-primary"
          :disabled="!currentRootId"
          @click="fileInputRef?.click()"
        >
          <Upload class="h-3.5 w-3.5" />
          <span>업로드</span>
        </button>

        <button
          v-if="!currentReadOnly"
          type="button"
          class="dsm-btn"
          :disabled="!currentRootId"
          @click="dialog = { kind: 'mkdir', value: '' }"
        >
          <FolderPlus class="h-3.5 w-3.5 text-[#f59e0b]" />
          <span>새 폴더 생성</span>
        </button>

        <button
          v-if="!currentReadOnly"
          type="button"
          class="dsm-btn"
          :disabled="!currentRootId"
          @click="dialog = { kind: 'new-file', value: '' }"
        >
          <FilePlus2 class="h-3.5 w-3.5 text-[#0572ec]" />
          <span>새 파일</span>
        </button>

        <span
          v-if="currentReadOnly"
          class="inline-flex h-7 items-center gap-1 rounded-[4px] border border-amber-300 bg-amber-50 px-2 text-[11px] font-medium text-amber-800"
        >
          <Lock class="h-3 w-3" />
          <span>읽기 전용</span>
        </span>

        <button
          type="button"
          class="dsm-btn"
          :disabled="!currentRootId"
          @click="downloadSelectionOrFolder"
        >
          <Download class="h-3.5 w-3.5 text-[#475569]" />
          <span>다운로드</span>
        </button>

        <button
          type="button"
          class="dsm-btn"
          :disabled="!selectedEntry || selectedEntry.isDir"
          @click="openSelectedInEditor"
        >
          <FileCode2 class="h-3.5 w-3.5 text-[#0572ec]" />
          <span>편집</span>
        </button>

        <button
          type="button"
          class="dsm-btn"
          :disabled="!currentRootId"
          @click="openShareForSelection"
        >
          <Share2 class="h-3.5 w-3.5 text-[#475569]" />
          <span>공유 링크</span>
        </button>

        <button
          v-if="
            authStore.accessTier === 'private' &&
            selectedEntry &&
            (selectedEntry.isDir || selectedEntry.kind === 'archive')
          "
          type="button"
          class="dsm-btn !border-[#b8d4fe] !text-[#0572ec]"
          @click="launchSelectedInWargame"
        >
          <Cpu class="h-3.5 w-3.5 text-[#0572ec]" />
          <span>워게임 실행</span>
        </button>
      </div>

      <div class="flex items-center gap-1.5">
        <button
          v-if="!currentReadOnly"
          type="button"
          class="dsm-btn"
          :disabled="!selectedEntry"
          title="이름 변경"
          @click="openRenameDialog"
        >
          <Pencil class="h-3.5 w-3.5 text-[#475569]" />
          <span>이름 변경</span>
        </button>

        <button
          v-if="!currentReadOnly"
          type="button"
          class="dsm-btn dsm-btn-danger"
          :disabled="!selectedEntry"
          title="삭제"
          @click="openDeleteDialog"
        >
          <Trash2 class="h-3.5 w-3.5" />
          <span>삭제</span>
        </button>

        <button
          type="button"
          class="dsm-btn"
          :disabled="!currentRootId"
          title="새로고침"
          @click="loadDirectory(currentRootId, currentRelPath)"
        >
          <RefreshCw class="h-3.5 w-3.5 text-[#475569]" :class="{ 'animate-spin': loading }" />
        </button>
      </div>
    </div>

    <div class="border-b border-[#e2e8f0] bg-white px-2.5 py-1.5">
      <div class="flex h-7 items-center gap-0.5 overflow-x-auto rounded-[4px] border border-[#cbd5e1] bg-[#f8fafc] px-2 text-[12px] text-[#334155]">
        <Folder class="mr-1 h-3.5 w-3.5 shrink-0 fill-[#fde68a] text-[#f59e0b]" />
        <button
          v-if="currentRootId"
          type="button"
          class="cursor-pointer rounded-[3px] px-1.5 py-0.5 font-medium text-[#1e293b] hover:bg-[#e2e8f0]/70"
          @click="loadDirectory(currentRootId, '')"
        >
          {{ roots.find((r) => r.id === currentRootId)?.name ?? currentRootId }}
        </button>
        <span v-else class="text-[#94a3b8]">공유 폴더 없음</span>

        <template v-for="crumb in breadcrumbs" :key="crumb.relPath">
          <ChevronRight class="h-3.5 w-3.5 shrink-0 text-[#94a3b8]" />
          <button
            type="button"
            class="cursor-pointer rounded-[3px] px-1.5 py-0.5 text-[#334155] hover:bg-[#e2e8f0]/70"
            @click="loadDirectory(currentRootId, crumb.relPath)"
          >
            {{ crumb.name }}
          </button>
        </template>
      </div>
    </div>

    <div class="flex min-h-0 flex-1 overflow-hidden">
      <aside class="flex w-52 shrink-0 flex-col border-r border-[#dce2ea] bg-[#f5f7fa] py-2">
        <div class="mb-1 px-3 text-[11px] font-semibold text-[#64748b]">공유 폴더</div>

        <nav class="min-h-0 flex-1 overflow-y-auto">
          <button
            v-for="root in roots"
            :key="root.id"
            type="button"
            class="flex w-full cursor-pointer items-center justify-between gap-2 px-3 py-1.5 text-left text-[12px] transition-colors"
            :class="
              currentRootId === root.id
                ? 'border-l-[3px] border-[#0572ec] bg-[#e6f0ff] font-semibold text-[#0058cc]'
                : 'border-l-[3px] border-transparent text-[#334155] hover:bg-[#eaeff5]'
            "
            @click="loadDirectory(root.id, '')"
          >
            <span class="flex min-w-0 items-center gap-2">
              <Folder class="h-4 w-4 shrink-0 fill-[#fde68a] text-[#f59e0b]" />
              <span class="truncate">{{ root.name }}</span>
            </span>
            <span
              v-if="root.readOnly"
              class="shrink-0 rounded-[3px] border border-amber-300 bg-amber-50 px-1 text-[10px] font-medium text-amber-700"
            >
              RO
            </span>
          </button>
        </nav>

        <div v-if="authStore.accessTier === 'private'" class="mt-2 border-t border-[#dce2ea] px-2.5 pt-2">
          <button
            type="button"
            class="dsm-btn w-full"
            @click="openMountManager"
          >
            <Settings2 class="h-3.5 w-3.5 text-[#475569]" />
            <span>+ 공유 폴더 마운트 설정</span>
          </button>
        </div>
      </aside>

      <div class="relative flex min-w-0 flex-1 flex-col bg-white">
        <div
          v-if="uploadProgress.active"
          class="flex items-center gap-3 border-b border-[#b8d4fe] bg-[#e6f0ff] px-3 py-1.5 text-[12px]"
        >
          <span class="truncate font-medium text-[#004eb3]">업로드 중: {{ uploadProgress.fileName }}</span>
          <div class="h-1.5 flex-1 overflow-hidden rounded-full bg-[#cbd5e1]">
            <div class="h-full bg-[#0572ec] transition-all" :style="{ width: `${uploadProgress.percent}%` }" />
          </div>
          <span class="tabular-nums font-medium text-[#004eb3]">{{ uploadProgress.percent }}%</span>
        </div>

        <div
          v-if="errorMessage"
          class="border-b border-[#f5c2c7] bg-[#fff5f5] px-3 py-1.5 text-[12px] font-medium text-[#c81e3a]"
        >
          {{ errorMessage }}
        </div>

        <div class="min-h-0 flex-1 overflow-y-auto bg-white" @click.self="selectedRelPath = null">
          <table class="w-full border-collapse bg-white text-left text-[12px]">
            <thead class="sticky top-0 z-10 h-8 border-b border-[#dce2ea] bg-[#f8fafc] text-[#475569]">
              <tr>
                <th class="border-r border-[#e2e8f0] px-3 font-medium">이름</th>
                <th class="w-28 border-r border-[#e2e8f0] px-3 font-medium">크기</th>
                <th class="hidden w-44 px-3 font-medium sm:table-cell">수정한 날짜</th>
              </tr>
            </thead>
            <tbody>
              <tr v-if="!loading && entries.length === 0">
                <td colspan="3" class="px-4 py-14 text-center text-[#64748b]">
                  이 폴더는 비어 있습니다. 파일을 끌어다 놓거나 상단의 업로드 버튼을 클릭하세요.
                </td>
              </tr>
              <tr
                v-for="entry in entries"
                :key="entry.relPath"
                class="h-[34px] cursor-pointer select-none border-b border-[#f1f5f9] transition-colors"
                :class="
                  selectedRelPath === entry.relPath
                    ? 'bg-[#e6f0ff] text-[#004eb3]'
                    : 'text-[#1e293b] hover:bg-[#f5f8fc]'
                "
                @click="selectedRelPath = entry.relPath"
                @dblclick="handleOpenEntry(entry)"
              >
                <td class="px-3">
                  <div class="flex items-center gap-2">
                    <component
                      :is="iconForKind(entry.kind)"
                      class="h-4 w-4 shrink-0"
                      :class="iconColorForKind(entry.kind)"
                    />
                    <span class="truncate font-medium">{{ entry.name }}</span>
                  </div>
                </td>
                <td
                  class="px-3 tabular-nums"
                  :class="selectedRelPath === entry.relPath ? 'text-[#004eb3]' : 'text-[#64748b]'"
                >
                  {{ entry.isDir ? '-' : formatBytes(entry.size) }}
                </td>
                <td
                  class="hidden px-3 tabular-nums sm:table-cell"
                  :class="selectedRelPath === entry.relPath ? 'text-[#004eb3]' : 'text-[#64748b]'"
                >
                  {{ formatDateTime(entry.modTime) }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <div class="flex h-7 shrink-0 items-center justify-between border-t border-[#dce2ea] bg-[#f8fafc] px-3 text-[11px] text-[#64748b]">
          <span>총 {{ entries.length }}개 항목</span>
          <span v-if="selectedEntry">선택됨: {{ selectedEntry.name }} (더블클릭하여 열기)</span>
          <span v-else>파일을 창으로 드래그하여 업로드</span>
        </div>

        <div
          v-if="dragOver"
          class="pointer-events-none absolute inset-0 flex items-center justify-center border-2 border-dashed border-[#0572ec] bg-[#e6f0ff]/75"
        >
          <div class="rounded-[6px] border border-[#b8d4fe] bg-white px-4 py-2.5 text-[12.5px] font-semibold text-[#0058cc] shadow-md">
            여기에 놓아 현재 폴더에 업로드
          </div>
        </div>
      </div>
    </div>

    <div
      v-if="dialog.kind !== 'none'"
      class="absolute inset-0 z-30 flex items-center justify-center bg-black/35 p-4"
      @click.self="dialog = { kind: 'none' }"
    >
      <form
        class="w-full max-w-sm overflow-hidden rounded-[6px] border border-[#cbd5e1] bg-white text-[#1e293b] shadow-xl"
        @submit.prevent="submitDialog"
      >
        <div class="flex items-center justify-between border-b border-[#dce2ea] bg-[#f4f6f9] px-4 py-2.5 text-[13px] font-semibold">
          <span>
            <template v-if="dialog.kind === 'mkdir'">새 폴더 생성</template>
            <template v-else-if="dialog.kind === 'new-file'">새 텍스트 파일 생성</template>
            <template v-else-if="dialog.kind === 'rename'">이름 변경</template>
            <template v-else>삭제 확인</template>
          </span>
          <button
            type="button"
            class="flex h-6 w-6 cursor-pointer items-center justify-center rounded-[4px] text-[#64748b] hover:bg-black/6 hover:text-[#1e293b]"
            @click="dialog = { kind: 'none' }"
          >
            <X class="h-3.5 w-3.5" />
          </button>
        </div>

        <div class="p-4">
          <template v-if="dialog.kind === 'mkdir' || dialog.kind === 'new-file' || dialog.kind === 'rename'">
            <label class="mb-1.5 block text-[12px] font-medium text-[#475569]">이름</label>
            <input
              v-model="dialog.value"
              type="text"
              autofocus
              class="dsm-input w-full"
              :placeholder="dialog.kind === 'new-file' ? 'example.yaml' : '이름 입력'"
            />
          </template>
          <p v-else class="text-[12.5px] leading-relaxed text-[#334155]">
            선택한 <strong class="font-semibold text-[#1e293b]">{{ dialog.entry.name }}</strong> 항목을 영구 삭제하시겠습니까?
          </p>
        </div>

        <div class="flex items-center justify-end gap-2 border-t border-[#dce2ea] bg-[#f8fafc] px-4 py-2.5">
          <button
            type="button"
            class="dsm-btn"
            @click="dialog = { kind: 'none' }"
          >
            취소
          </button>
          <button
            type="submit"
            :class="dialog.kind === 'delete' ? 'dsm-btn dsm-btn-danger-solid' : 'dsm-btn dsm-btn-primary'"
          >
            {{ dialog.kind === 'delete' ? '삭제' : '확인' }}
          </button>
        </div>
      </form>
    </div>

    <ShareModal
      v-if="shareTarget"
      :target-root-id="shareTarget.rootId"
      :target-rel-path="shareTarget.relPath"
      :target-name="shareTarget.name"
      @close="shareTarget = null"
    />
  </div>
</template>
