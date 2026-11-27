<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  Folder,
  FolderSearch,
  Pencil,
  Plus,
  RefreshCw,
  Trash2,
  X,
} from 'lucide-vue-next'
import { api } from '../../api/client'
import { useMountsStore } from '../../stores/mounts'
import type { MountInfo } from '../../types/api'
import HostDirPickerModal from './HostDirPickerModal.vue'

type MountModalState =
  | { readonly kind: 'closed' }
  | {
      readonly kind: 'create'
      readonly hostPath: string
      readonly name: string
      readonly readOnly: boolean
      readonly privateOnly: boolean
      readonly pickerOpen: boolean
    }
  | {
      readonly kind: 'edit'
      readonly id: string
      readonly hostPath: string
      readonly name: string
      readonly readOnly: boolean
      readonly privateOnly: boolean
    }
  | { readonly kind: 'unmount'; readonly target: MountInfo }

const mountsStore = useMountsStore()

const mounts = ref<readonly MountInfo[]>([])
const selectedId = ref<string | null>(null)
const loading = ref(false)
const submitting = ref(false)
const errorMessage = ref('')
const modalState = ref<MountModalState>({ kind: 'closed' })

const selectedMount = computed<MountInfo | null>(
  () => mounts.value.find((item) => item.id === selectedId.value) ?? null,
)

async function refreshMounts(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    const nextMounts = await api.listMounts()
    mounts.value = nextMounts
    if (selectedId.value !== null && !nextMounts.some((m) => m.id === selectedId.value)) {
      selectedId.value = null
    }
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '마운트 목록을 불러오지 못했습니다.'
  } finally {
    loading.value = false
  }
}

function openCreateModal(): void {
  errorMessage.value = ''
  modalState.value = {
    kind: 'create',
    hostPath: '',
    name: '',
    readOnly: false,
    privateOnly: false,
    pickerOpen: false,
  }
}

function patchCreateModal(
  patch: Partial<Extract<MountModalState, { readonly kind: 'create' }>>,
): void {
  if (modalState.value.kind !== 'create') return
  modalState.value = {
    ...modalState.value,
    ...patch,
    kind: 'create',
  }
}

function handleCreateHostPathInput(ev: Event): void {
  if (ev.target instanceof HTMLInputElement) {
    patchCreateModal({ hostPath: ev.target.value })
  }
}

function handleCreateNameInput(ev: Event): void {
  if (ev.target instanceof HTMLInputElement) {
    patchCreateModal({ name: ev.target.value })
  }
}

function handleCreateReadOnlyChange(ev: Event): void {
  if (ev.target instanceof HTMLInputElement) {
    patchCreateModal({ readOnly: ev.target.checked })
  }
}

function handleCreatePrivateOnlyChange(ev: Event): void {
  if (ev.target instanceof HTMLInputElement) {
    patchCreateModal({ privateOnly: ev.target.checked })
  }
}

function handlePickedPath(selectedPath: string): void {
  if (modalState.value.kind !== 'create') return
  const currentName = modalState.value.name.trim()
  let nextName = modalState.value.name
  if (currentName === '') {
    const segs = selectedPath.split('/').filter(Boolean)
    const leaf = segs[segs.length - 1]
    nextName = leaf ?? 'Root'
  }
  patchCreateModal({
    hostPath: selectedPath,
    name: nextName,
    pickerOpen: false,
  })
}

async function handleCreateMount(): Promise<void> {
  if (modalState.value.kind !== 'create') return
  const form = modalState.value
  if (!form.hostPath.trim()) {
    errorMessage.value = '마운트할 호스트 디렉토리 경로를 선택하거나 입력하세요.'
    return
  }
  submitting.value = true
  errorMessage.value = ''
  try {
    const trimmedPath = form.hostPath.trim()
    await api.createMount({
      name: form.name.trim(),
      hostPath: trimmedPath,
      readOnly: form.readOnly,
      privateOnly: form.privateOnly,
    })
    modalState.value = { kind: 'closed' }
    await refreshMounts()
    const matched = mounts.value.find((m) => m.hostPath === trimmedPath)
    if (matched) {
      selectedId.value = matched.id
    }
    mountsStore.notifyChanged()
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '마운트 등록에 실패했습니다.'
  } finally {
    submitting.value = false
  }
}

function openEdit(item: MountInfo): void {
  selectedId.value = item.id
  errorMessage.value = ''
  modalState.value = {
    kind: 'edit',
    id: item.id,
    hostPath: item.hostPath,
    name: item.name,
    readOnly: item.readOnly,
    privateOnly: item.privateOnly,
  }
}

function openSelectedEdit(): void {
  if (selectedMount.value) {
    openEdit(selectedMount.value)
  }
}

function patchEditModal(
  patch: Partial<Extract<MountModalState, { readonly kind: 'edit' }>>,
): void {
  if (modalState.value.kind !== 'edit') return
  modalState.value = {
    ...modalState.value,
    ...patch,
    kind: 'edit',
  }
}

function handleEditNameInput(ev: Event): void {
  if (ev.target instanceof HTMLInputElement) {
    patchEditModal({ name: ev.target.value })
  }
}

function handleEditReadOnlyChange(ev: Event): void {
  if (ev.target instanceof HTMLInputElement) {
    patchEditModal({ readOnly: ev.target.checked })
  }
}

function handleEditPrivateOnlyChange(ev: Event): void {
  if (ev.target instanceof HTMLInputElement) {
    patchEditModal({ privateOnly: ev.target.checked })
  }
}

async function saveEdit(): Promise<void> {
  if (modalState.value.kind !== 'edit') return
  const target = modalState.value
  submitting.value = true
  errorMessage.value = ''
  try {
    await api.updateMount({
      id: target.id,
      name: target.name,
      readOnly: target.readOnly,
      privateOnly: target.privateOnly,
    })
    modalState.value = { kind: 'closed' }
    await refreshMounts()
    mountsStore.notifyChanged()
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '마운트 수정에 실패했습니다.'
  } finally {
    submitting.value = false
  }
}

function openSelectedUnmount(): void {
  if (!selectedMount.value) return
  errorMessage.value = ''
  modalState.value = { kind: 'unmount', target: selectedMount.value }
}

async function confirmUnmount(): Promise<void> {
  if (modalState.value.kind !== 'unmount') return
  const target = modalState.value.target
  submitting.value = true
  errorMessage.value = ''
  try {
    await api.deleteMount(target.id)
    modalState.value = { kind: 'closed' }
    if (selectedId.value === target.id) {
      selectedId.value = null
    }
    await refreshMounts()
    mountsStore.notifyChanged()
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '마운트 해제에 실패했습니다.'
  } finally {
    submitting.value = false
  }
}

function closeModal(): void {
  if (submitting.value) return
  errorMessage.value = ''
  modalState.value = { kind: 'closed' }
}

onMounted(() => {
  void refreshMounts()
})
</script>

<template>
  <div class="flex h-full w-full overflow-hidden bg-white text-[12px] text-[#1e293b]">
    <aside class="hidden w-44 shrink-0 border-r border-[#dce2ea] bg-[#f5f7fa] p-2 sm:flex sm:flex-col sm:justify-between">
      <div>
        <div class="px-2.5 pt-1.5 pb-1 text-[11px] font-medium text-[#64748b]">파일 공유</div>
        <div
          class="flex items-center gap-2 rounded-[4px] bg-[#e6f0ff] px-3 py-2 text-[12px] font-semibold text-[#0058cc]"
        >
          <Folder class="h-4 w-4 shrink-0 fill-[#fbbf24] text-[#d97706]" />
          <span>공유 폴더</span>
        </div>
      </div>

      <div class="border-t border-[#dce2ea] px-2.5 pt-2 text-[11px] text-[#64748b]">
        연결된 폴더 {{ mounts.length }}개
      </div>
    </aside>

    <div class="flex min-w-0 flex-1 flex-col bg-white">
      <div class="flex h-11 shrink-0 flex-nowrap items-center justify-between gap-2 overflow-x-auto border-b border-[#dce2ea] bg-[#f8fafc] px-3">
        <div class="flex shrink-0 flex-nowrap items-center gap-1.5">
          <button type="button" class="dsm-btn dsm-btn-primary" @click="openCreateModal">
            <Plus class="h-3.5 w-3.5" />
            생성 (마운트 연결)
          </button>
          <button
            type="button"
            class="dsm-btn"
            :disabled="!selectedMount"
            @click="openSelectedEdit"
          >
            <Pencil class="h-3.5 w-3.5" />
            편집
          </button>
          <button
            type="button"
            class="dsm-btn dsm-btn-danger"
            :disabled="!selectedMount"
            @click="openSelectedUnmount"
          >
            <Trash2 class="h-3.5 w-3.5" />
            마운트 해제
          </button>
        </div>

        <button type="button" class="dsm-btn" :disabled="loading" @click="refreshMounts">
          <RefreshCw class="h-3.5 w-3.5" :class="{ 'animate-spin': loading }" />
          새로고침
        </button>
      </div>

      <div
        v-if="errorMessage && modalState.kind === 'closed'"
        class="flex shrink-0 items-center justify-between gap-2 border-b border-[#fecdd3] bg-[#fff1f2] px-3.5 py-2 text-[#be123c]"
      >
        <span>{{ errorMessage }}</span>
        <button
          type="button"
          class="rounded-[4px] p-0.5 text-[#be123c] hover:bg-[#ffe4e6]"
          @click="errorMessage = ''"
        >
          <X class="h-3.5 w-3.5" />
        </button>
      </div>

      <div class="min-h-0 flex-1 overflow-auto bg-white" @click.self="selectedId = null">
        <table class="w-full border-collapse text-left text-[12px]">
          <thead class="sticky top-0 z-10 border-b border-[#dce2ea] bg-[#f8fafc] text-[11px] font-semibold text-[#475569]">
            <tr class="h-8">
              <th class="px-3 font-semibold">이름</th>
              <th class="px-3 font-semibold">호스트 경로</th>
              <th class="w-28 px-3 font-semibold">권한</th>
              <th class="w-32 px-3 font-semibold">노출 범위</th>
              <th class="w-28 px-3 font-semibold">상태</th>
            </tr>
          </thead>

          <tbody v-if="mounts.length > 0">
            <tr
              v-for="item in mounts"
              :key="item.id"
              class="h-9 cursor-pointer select-none border-b border-[#eef2f6] transition-colors"
              :class="
                selectedId === item.id
                  ? 'bg-[#e6f0ff] text-[#0058cc]'
                  : 'text-[#1e293b] hover:bg-[#f8fafc]'
              "
              @click="selectedId = item.id"
              @dblclick="openEdit(item)"
            >
              <td class="px-3">
                <div class="flex items-center gap-2">
                  <Folder class="h-4 w-4 shrink-0 fill-[#fbbf24] text-[#d97706]" />
                  <span class="truncate font-medium">{{ item.name }}</span>
                </div>
              </td>

              <td class="max-w-xs truncate px-3 font-mono text-[11px] text-[#475569]">
                {{ item.hostPath }}
              </td>

              <td class="whitespace-nowrap px-3">
                <span
                  v-if="item.readOnly"
                  class="inline-flex items-center whitespace-nowrap rounded-[3px] border border-[#fde68a] bg-[#fffbeb] px-1.5 py-0.5 text-[11px] font-medium text-[#92400e]"
                >
                  읽기 전용
                </span>
                <span
                  v-else
                  class="inline-flex items-center whitespace-nowrap rounded-[3px] border border-[#dce2ea] bg-[#f8fafc] px-1.5 py-0.5 text-[11px] font-medium text-[#334155]"
                >
                  읽기/쓰기
                </span>
              </td>

              <td class="whitespace-nowrap px-3">
                <span
                  v-if="item.privateOnly"
                  class="inline-flex items-center whitespace-nowrap rounded-[3px] border border-[#b8d4fe] bg-[#eff6ff] px-1.5 py-0.5 text-[11px] font-medium text-[#0058cc]"
                >
                  Tailscale 전용
                </span>
                <span
                  v-else
                  class="inline-flex items-center whitespace-nowrap rounded-[3px] border border-[#dce2ea] bg-[#f8fafc] px-1.5 py-0.5 text-[11px] font-medium text-[#475569]"
                >
                  Public 허용
                </span>
              </td>

              <td class="whitespace-nowrap px-3">
                <span
                  v-if="item.exists"
                  class="inline-flex items-center gap-1.5 whitespace-nowrap font-medium text-[#059669]"
                >
                  <span class="h-2 w-2 rounded-full bg-[#10b981]" />
                  정상
                </span>
                <span
                  v-else
                  class="inline-flex items-center gap-1.5 whitespace-nowrap font-medium text-[#dc2626]"
                >
                  <span class="h-2 w-2 rounded-full bg-[#ef4444]" />
                  경로 없음
                </span>
              </td>
            </tr>
          </tbody>
        </table>

        <div
          v-if="mounts.length === 0 && !loading"
          class="flex flex-col items-center justify-center py-16 text-center text-[#64748b]"
        >
          <p class="font-medium text-[#334155]">연결된 공유 폴더가 없습니다.</p>
          <p class="mt-1 text-[11px]">
            상단 툴바의 '생성 (마운트 연결)' 버튼을 눌러 서버 로컬 디렉토리를 File Station에 연결하세요.
          </p>
        </div>
      </div>

      <div class="flex h-7 shrink-0 items-center justify-between border-t border-[#dce2ea] bg-[#f8fafc] px-3 text-[11px] text-[#64748b]">
        <span>총 {{ mounts.length }}개 항목</span>
        <span v-if="selectedMount" class="truncate font-mono">
          선택됨: {{ selectedMount.name }} ({{ selectedMount.hostPath }})
        </span>
        <span v-else>행을 클릭해 선택하거나 더블클릭해 편집하세요.</span>
      </div>
    </div>

    <div
      v-if="modalState.kind === 'create'"
      class="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/35 p-4"
      @click.self="closeModal"
    >
      <form
        class="w-full max-w-lg overflow-hidden rounded-[6px] border border-[#cbd5e1] bg-white text-[#1e293b] shadow-xl"
        @submit.prevent="handleCreateMount"
      >
        <div class="flex items-center justify-between border-b border-[#dce2ea] bg-[#eef1f5] px-4 py-2.5">
          <h3 class="text-[13px] font-semibold text-[#1e293b]">공유 폴더 생성 (마운트 연결)</h3>
          <button
            type="button"
            class="rounded-[4px] p-1 text-[#475569] hover:bg-[#dce2ea] hover:text-[#1e293b]"
            @click="closeModal"
          >
            <X class="h-4 w-4" />
          </button>
        </div>

        <div class="space-y-3.5 p-4">
          <p class="text-[11px] text-[#64748b]">
            서버 컴퓨터의 로컬 디렉토리를 File Station 공유 폴더로 연결합니다. 재시작 없이 즉시 반영됩니다.
          </p>

          <div
            v-if="errorMessage"
            class="rounded-[4px] border border-[#fecdd3] bg-[#fff1f2] px-3 py-2 text-[11px] text-[#be123c]"
          >
            {{ errorMessage }}
          </div>

          <div>
            <label class="mb-1 block text-[11px] font-medium text-[#475569]">호스트 실제 경로</label>
            <div class="flex items-center gap-2">
              <input
                :value="modalState.hostPath"
                type="text"
                placeholder="예: /mnt/hdd/movies 또는 ~/Movies"
                class="dsm-input flex-1 font-mono"
                @input="handleCreateHostPathInput"
              />
              <button
                type="button"
                class="dsm-btn shrink-0"
                @click="patchCreateModal({ pickerOpen: true })"
              >
                <FolderSearch class="h-3.5 w-3.5 text-[#0572ec]" />
                서버 폴더 찾아보기
              </button>
            </div>
          </div>

          <div>
            <label class="mb-1 block text-[11px] font-medium text-[#475569]">
              표시 이름 (비워두면 폴더명 사용)
            </label>
            <input
              :value="modalState.name"
              type="text"
              placeholder="예: 외장하드 영화"
              class="dsm-input w-full"
              @input="handleCreateNameInput"
            />
          </div>

          <div class="space-y-2 rounded-[4px] border border-[#e2e8f0] bg-[#f8fafc] p-3">
            <label class="flex items-center gap-2 text-[#334155]">
              <input
                :checked="modalState.readOnly"
                type="checkbox"
                class="accent-[#0572ec]"
                @change="handleCreateReadOnlyChange"
              />
              읽기 전용 (업로드·수정·삭제 차단)
            </label>
            <label class="flex items-center gap-2 text-[#334155]">
              <input
                :checked="modalState.privateOnly"
                type="checkbox"
                class="accent-[#0572ec]"
                @change="handleCreatePrivateOnlyChange"
              />
              Tailscale 내부망 전용 (외부 NPM에서 숨김)
            </label>
          </div>
        </div>

        <div class="flex justify-end gap-2 border-t border-[#dce2ea] bg-[#f8fafc] px-4 py-2.5">
          <button type="button" class="dsm-btn" :disabled="submitting" @click="closeModal">
            취소
          </button>
          <button type="submit" class="dsm-btn dsm-btn-primary" :disabled="submitting">
            {{ submitting ? '연결 중...' : '마운트 연결' }}
          </button>
        </div>
      </form>
    </div>

    <HostDirPickerModal
      v-if="modalState.kind === 'create' && modalState.pickerOpen"
      :initial-path="modalState.hostPath"
      @select="handlePickedPath"
      @close="patchCreateModal({ pickerOpen: false })"
    />

    <div
      v-if="modalState.kind === 'edit'"
      class="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/35 p-4"
      @click.self="closeModal"
    >
      <form
        class="w-full max-w-md overflow-hidden rounded-[6px] border border-[#cbd5e1] bg-white text-[#1e293b] shadow-xl"
        @submit.prevent="saveEdit"
      >
        <div class="flex items-center justify-between border-b border-[#dce2ea] bg-[#eef1f5] px-4 py-2.5">
          <h4 class="text-[13px] font-semibold text-[#1e293b]">공유 폴더 편집</h4>
          <button
            type="button"
            class="rounded-[4px] p-1 text-[#475569] hover:bg-[#dce2ea] hover:text-[#1e293b]"
            @click="closeModal"
          >
            <X class="h-4 w-4" />
          </button>
        </div>

        <div class="space-y-3.5 p-4">
          <div
            v-if="errorMessage"
            class="rounded-[4px] border border-[#fecdd3] bg-[#fff1f2] px-3 py-2 text-[11px] text-[#be123c]"
          >
            {{ errorMessage }}
          </div>

          <div>
            <div class="mb-1 text-[11px] font-medium text-[#475569]">호스트 경로</div>
            <div class="truncate rounded-[4px] border border-[#dce2ea] bg-[#f8fafc] px-2.5 py-1.5 font-mono text-[11px] text-[#475569]">
              {{ modalState.hostPath }}
            </div>
          </div>

          <div>
            <label class="mb-1 block text-[11px] font-medium text-[#475569]">표시 이름</label>
            <input
              :value="modalState.name"
              type="text"
              class="dsm-input w-full"
              @input="handleEditNameInput"
            />
          </div>

          <div class="space-y-2 rounded-[4px] border border-[#e2e8f0] bg-[#f8fafc] p-3">
            <label class="flex items-center gap-2 text-[#334155]">
              <input
                :checked="modalState.readOnly"
                type="checkbox"
                class="accent-[#0572ec]"
                @change="handleEditReadOnlyChange"
              />
              읽기 전용 (수정·업로드·삭제 차단)
            </label>

            <label class="flex items-center gap-2 text-[#334155]">
              <input
                :checked="modalState.privateOnly"
                type="checkbox"
                class="accent-[#0572ec]"
                @change="handleEditPrivateOnlyChange"
              />
              Tailscale 내부망 전용 (외부 NPM에서 숨김)
            </label>
          </div>
        </div>

        <div class="flex justify-end gap-2 border-t border-[#dce2ea] bg-[#f8fafc] px-4 py-2.5">
          <button type="button" class="dsm-btn" :disabled="submitting" @click="closeModal">
            취소
          </button>
          <button type="submit" class="dsm-btn dsm-btn-primary" :disabled="submitting">
            {{ submitting ? '저장 중...' : '저장' }}
          </button>
        </div>
      </form>
    </div>

    <div
      v-if="modalState.kind === 'unmount'"
      class="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/35 p-4"
      @click.self="closeModal"
    >
      <div class="w-full max-w-sm overflow-hidden rounded-[6px] border border-[#cbd5e1] bg-white text-[#1e293b] shadow-xl">
        <div class="flex items-center justify-between border-b border-[#dce2ea] bg-[#eef1f5] px-4 py-2.5">
          <h4 class="text-[13px] font-semibold text-[#1e293b]">마운트 연결 해제</h4>
          <button
            type="button"
            class="rounded-[4px] p-1 text-[#475569] hover:bg-[#dce2ea] hover:text-[#1e293b]"
            @click="closeModal"
          >
            <X class="h-4 w-4" />
          </button>
        </div>

        <div class="space-y-3 p-4">
          <div
            v-if="errorMessage"
            class="rounded-[4px] border border-[#fecdd3] bg-[#fff1f2] px-3 py-2 text-[11px] text-[#be123c]"
          >
            {{ errorMessage }}
          </div>

          <p class="leading-relaxed text-[#334155]">
            <strong class="font-semibold text-[#1e293b]">{{ modalState.target.name }}</strong> 공유 폴더 마운트를 해제할까요?
          </p>
          <p class="rounded-[4px] border border-[#dce2ea] bg-[#f8fafc] px-2.5 py-2 text-[11px] text-[#475569]">
            서버 디스크의 실제 파일(<span class="font-mono text-[#1e293b]">{{ modalState.target.hostPath }}</span>)은 삭제되지 않습니다.
          </p>
        </div>

        <div class="flex justify-end gap-2 border-t border-[#dce2ea] bg-[#f8fafc] px-4 py-2.5">
          <button type="button" class="dsm-btn" :disabled="submitting" @click="closeModal">
            취소
          </button>
          <button
            type="button"
            class="dsm-btn dsm-btn-danger-solid"
            :disabled="submitting"
            @click="confirmUnmount"
          >
            {{ submitting ? '해제 중...' : '연결 해제' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
