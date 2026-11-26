<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import {
  HardDrive,
  Folder,
  Film,
  FileCode2,
  FileText,
  Image as ImageIcon,
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

// Inline dialog states (avoids browser prompt/confirm blocking)
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
    const stillExists = roots.value.some((r) => r.id === (preferredRoot ?? currentRootId.value))
    if (stillExists && (preferredRoot ?? currentRootId.value)) {
      await loadDirectory(preferredRoot ?? currentRootId.value, preferredPath ?? currentRelPath.value)
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
      title: `Video Player - ${entry.name}`,
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
      title: `Code Editor - ${entry.name}`,
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
    title: `Code Editor - ${entry.name}`,
    payload: {
      kind: 'code-editor',
      rootId: currentRootId.value,
      relPath: entry.relPath,
      fileName: entry.name,
    },
  })
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
        title: `Code Editor - ${d.value.trim()}`,
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
      return ImageIcon
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
      return 'text-amber-400'
    case 'video':
      return 'text-rose-400'
    case 'code':
      return 'text-emerald-400'
    case 'image':
      return 'text-purple-400'
    case 'audio':
      return 'text-cyan-400'
    case 'archive':
      return 'text-orange-400'
    case 'file':
      return 'text-slate-400'
    default: {
      const _exhaustive: never = kind
      return _exhaustive
    }
  }
}
</script>

<template>
  <div
    class="flex h-full w-full text-xs text-slate-200"
    @dragover.prevent="dragOver = true"
    @dragleave.prevent="dragOver = false"
    @drop.prevent="handleDrop"
  >
    <!-- Left Sidebar: Mounted Roots -->
    <aside class="flex w-52 shrink-0 flex-col border-r border-white/10 bg-slate-950/50 p-2.5">
      <div class="mb-2 px-2 text-[11px] font-semibold tracking-wider text-slate-400">마운트 폴더</div>
      <nav class="min-h-0 flex-1 space-y-1 overflow-y-auto">
        <button
          v-for="root in roots"
          :key="root.id"
          type="button"
          class="flex w-full items-center justify-between gap-2 rounded-lg px-2.5 py-2 text-left transition"
          :class="
            currentRootId === root.id
              ? 'bg-sky-500/20 font-semibold text-sky-300'
              : 'text-slate-300 hover:bg-white/5'
          "
          @click="loadDirectory(root.id, '')"
        >
          <span class="flex min-w-0 items-center gap-2">
            <HardDrive class="h-4 w-4 shrink-0" />
            <span class="truncate">{{ root.name }}</span>
          </span>
          <span
            v-if="root.readOnly"
            class="shrink-0 rounded bg-amber-500/20 px-1 py-0.5 text-[9px] font-semibold text-amber-300"
          >
            RO
          </span>
        </button>
      </nav>

      <div v-if="authStore.accessTier === 'private'" class="mt-2 border-t border-white/10 pt-2">
        <button
          type="button"
          class="flex w-full items-center justify-center gap-1.5 rounded-lg border border-sky-400/25 bg-sky-500/10 px-2.5 py-2 text-[11px] font-semibold text-sky-300 transition hover:bg-sky-500/20"
          @click="openMountManager"
        >
          <Settings2 class="h-3.5 w-3.5" />
          + 폴더 마운트 관리
        </button>
      </div>
    </aside>

    <!-- Main Content -->
    <div class="relative flex min-w-0 flex-1 flex-col">
      <!-- Toolbar -->
      <div class="flex flex-wrap items-center justify-between gap-2 border-b border-white/10 bg-slate-900/80 px-3 py-2">
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
            class="inline-flex items-center gap-1.5 rounded-lg bg-sky-600 px-2.5 py-1.5 font-semibold text-white hover:bg-sky-500"
            @click="fileInputRef?.click()"
          >
            <Upload class="h-3.5 w-3.5" />
            업로드
          </button>

          <button
            v-if="!currentReadOnly"
            type="button"
            class="inline-flex items-center gap-1.5 rounded-lg bg-white/5 px-2.5 py-1.5 hover:bg-white/10"
            @click="dialog = { kind: 'mkdir', value: '' }"
          >
            <FolderPlus class="h-3.5 w-3.5 text-amber-300" />
            새 폴더
          </button>

          <button
            v-if="!currentReadOnly"
            type="button"
            class="inline-flex items-center gap-1.5 rounded-lg bg-white/5 px-2.5 py-1.5 hover:bg-white/10"
            @click="dialog = { kind: 'new-file', value: '' }"
          >
            <FilePlus2 class="h-3.5 w-3.5 text-emerald-300" />
            새 파일
          </button>

          <span
            v-if="currentReadOnly"
            class="inline-flex items-center gap-1 rounded-lg border border-amber-400/30 bg-amber-500/15 px-2.5 py-1.5 text-[11px] font-semibold text-amber-300"
          >
            <Lock class="h-3 w-3" />
            읽기 전용 폴더
          </span>

          <button
            v-if="selectedEntry && !selectedEntry.isDir"
            type="button"
            class="inline-flex items-center gap-1.5 rounded-lg bg-white/5 px-2.5 py-1.5 hover:bg-white/10"
            @click="openSelectedInEditor"
          >
            <FileCode2 class="h-3.5 w-3.5 text-emerald-300" />
            편집
          </button>

          <button
            type="button"
            class="inline-flex items-center gap-1.5 rounded-lg bg-white/5 px-2.5 py-1.5 hover:bg-white/10"
            @click="downloadSelectionOrFolder"
          >
            <Download class="h-3.5 w-3.5 text-sky-300" />
            {{ selectedEntry ? (selectedEntry.isDir ? 'ZIP 다운로드' : '다운로드') : '현재 폴더 ZIP' }}
          </button>

          <button
            type="button"
            class="inline-flex items-center gap-1.5 rounded-lg bg-white/5 px-2.5 py-1.5 hover:bg-white/10"
            @click="openShareForSelection"
          >
            <Share2 class="h-3.5 w-3.5 text-indigo-300" />
            공유 링크
          </button>
        </div>

        <div class="flex items-center gap-1">
          <button
            v-if="selectedEntry && !currentReadOnly"
            type="button"
            class="rounded-lg p-1.5 text-slate-300 hover:bg-white/10"
            title="이름 변경"
            @click="dialog = { kind: 'rename', entry: selectedEntry, value: selectedEntry.name }"
          >
            <Pencil class="h-3.5 w-3.5" />
          </button>
          <button
            v-if="selectedEntry && !currentReadOnly"
            type="button"
            class="rounded-lg p-1.5 text-rose-300 hover:bg-rose-500/20"
            title="삭제"
            @click="dialog = { kind: 'delete', entry: selectedEntry }"
          >
            <Trash2 class="h-3.5 w-3.5" />
          </button>
          <button
            type="button"
            class="rounded-lg p-1.5 text-slate-300 hover:bg-white/10"
            title="새로고침"
            @click="loadDirectory(currentRootId, currentRelPath)"
          >
            <RefreshCw class="h-3.5 w-3.5" :class="{ 'animate-spin': loading }" />
          </button>
        </div>
      </div>

      <!-- Breadcrumb Bar -->
      <div class="flex items-center gap-1 overflow-x-auto border-b border-white/5 bg-slate-950/40 px-3 py-1.5 text-xs">
        <button
          type="button"
          class="rounded px-1.5 py-0.5 font-medium text-sky-300 hover:bg-white/5"
          @click="loadDirectory(currentRootId, '')"
        >
          {{ roots.find((r) => r.id === currentRootId)?.name ?? currentRootId }}
        </button>
        <template v-for="crumb in breadcrumbs" :key="crumb.relPath">
          <ChevronRight class="h-3.5 w-3.5 shrink-0 text-slate-500" />
          <button
            type="button"
            class="rounded px-1.5 py-0.5 text-slate-200 hover:bg-white/5"
            @click="loadDirectory(currentRootId, crumb.relPath)"
          >
            {{ crumb.name }}
          </button>
        </template>
      </div>

      <!-- Upload Progress Banner -->
      <div
        v-if="uploadProgress.active"
        class="flex items-center gap-3 border-b border-sky-500/20 bg-sky-500/10 px-3 py-2 text-xs"
      >
        <span class="truncate font-medium text-sky-200">업로드 중: {{ uploadProgress.fileName }}</span>
        <div class="h-1.5 flex-1 overflow-hidden rounded-full bg-slate-800">
          <div class="h-full bg-sky-400 transition-all" :style="{ width: `${uploadProgress.percent}%` }" />
        </div>
        <span class="tabular-nums text-sky-300">{{ uploadProgress.percent }}%</span>
      </div>

      <!-- Error Banner -->
      <div
        v-if="errorMessage"
        class="border-b border-rose-500/30 bg-rose-500/15 px-3 py-2 text-xs text-rose-200"
      >
        {{ errorMessage }}
      </div>

      <!-- File Table -->
      <div class="min-h-0 flex-1 overflow-y-auto" @click.self="selectedRelPath = null">
        <table class="w-full border-collapse text-left">
          <thead class="sticky top-0 z-10 bg-slate-950/90 text-[11px] text-slate-400 backdrop-blur">
            <tr class="border-b border-white/10">
              <th class="px-3 py-2 font-medium">이름</th>
              <th class="w-28 px-3 py-2 font-medium">크기</th>
              <th class="hidden w-40 px-3 py-2 font-medium sm:table-cell">수정한 날짜</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="!loading && entries.length === 0">
              <td colspan="3" class="px-4 py-14 text-center text-slate-500">
                폴더가 비어 있습니다. 파일을 드래그해서 놓거나 상단의 업로드 버튼을 누르세요.
              </td>
            </tr>
            <tr
              v-for="entry in entries"
              :key="entry.relPath"
              class="cursor-pointer select-none border-b border-white/5 transition"
              :class="
                selectedRelPath === entry.relPath
                  ? 'bg-sky-500/20 text-white'
                  : 'hover:bg-white/5'
              "
              @click="selectedRelPath = entry.relPath"
              @dblclick="handleOpenEntry(entry)"
            >
              <td class="px-3 py-2">
                <div class="flex items-center gap-2.5">
                  <component
                    :is="iconForKind(entry.kind)"
                    class="h-4 w-4 shrink-0"
                    :class="iconColorForKind(entry.kind)"
                  />
                  <span class="truncate font-medium">{{ entry.name }}</span>
                </div>
              </td>
              <td class="px-3 py-2 tabular-nums text-slate-400">
                {{ entry.isDir ? '-' : formatBytes(entry.size) }}
              </td>
              <td class="hidden px-3 py-2 tabular-nums text-slate-400 sm:table-cell">
                {{ formatDateTime(entry.modTime) }}
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Footer Status Bar -->
      <div class="flex items-center justify-between border-t border-white/10 bg-slate-950/60 px-3 py-1.5 text-[11px] text-slate-400">
        <span>총 {{ entries.length }}개 항목</span>
        <span v-if="selectedEntry">더블클릭하여 열기 · {{ selectedEntry.name }}</span>
        <span v-else>파일을 창으로 드래그하여 청크 업로드 가능</span>
      </div>

      <!-- Drag Overlay -->
      <div
        v-if="dragOver"
        class="pointer-events-none absolute inset-0 flex items-center justify-center border-2 border-dashed border-sky-400 bg-sky-950/60 backdrop-blur-xs"
      >
        <div class="rounded-xl bg-slate-900 px-5 py-3 text-sm font-semibold text-sky-300 shadow-xl">
          여기에 놓아 현재 폴더에 업로드
        </div>
      </div>

      <!-- Inline Action Dialog -->
      <div
        v-if="dialog.kind !== 'none'"
        class="absolute inset-0 z-30 flex items-center justify-center bg-black/55 p-4"
        @click.self="dialog = { kind: 'none' }"
      >
        <form
          class="w-full max-w-sm rounded-2xl border border-white/15 bg-slate-900 p-4 shadow-2xl"
          @submit.prevent="submitDialog"
        >
          <h4 class="mb-3 text-sm font-semibold text-white">
            <template v-if="dialog.kind === 'mkdir'">새 폴더 만들기</template>
            <template v-else-if="dialog.kind === 'new-file'">새 텍스트 파일 만들기</template>
            <template v-else-if="dialog.kind === 'rename'">이름 변경</template>
            <template v-else>항목 삭제 확인</template>
          </h4>

          <input
            v-if="dialog.kind === 'mkdir' || dialog.kind === 'new-file' || dialog.kind === 'rename'"
            v-model="dialog.value"
            type="text"
            autofocus
            class="mb-3 w-full rounded-xl border border-white/10 bg-slate-950 px-3 py-2 text-xs text-white focus:border-sky-400 focus:outline-none"
            :placeholder="dialog.kind === 'new-file' ? 'example.yaml' : '이름 입력'"
          />
          <p v-else class="mb-4 text-xs text-slate-300">
            정말 <strong class="text-white">{{ dialog.entry.name }}</strong> 항목을 영구 삭제할까요?
          </p>

          <div class="flex justify-end gap-2">
            <button
              type="button"
              class="rounded-lg px-3 py-1.5 text-xs text-slate-400 hover:bg-white/5"
              @click="dialog = { kind: 'none' }"
            >
              취소
            </button>
            <button
              type="submit"
              class="rounded-lg px-3 py-1.5 text-xs font-semibold text-white"
              :class="dialog.kind === 'delete' ? 'bg-rose-600 hover:bg-rose-500' : 'bg-sky-600 hover:bg-sky-500'"
            >
              {{ dialog.kind === 'delete' ? '삭제' : '확인' }}
            </button>
          </div>
        </form>
      </div>
    </div>

    <!-- Share Modal -->
    <ShareModal
      v-if="shareTarget"
      :target-root-id="shareTarget.rootId"
      :target-rel-path="shareTarget.relPath"
      :target-name="shareTarget.name"
      @close="shareTarget = null"
    />
  </div>
</template>
