<script setup lang="ts">
import { onMounted, ref } from 'vue'
import {
  HardDrive,
  FolderSearch,
  Plus,
  Trash2,
  Pencil,
  ShieldCheck,
  Globe,
  Lock,
  AlertTriangle,
  RefreshCw,
} from 'lucide-vue-next'
import { api } from '../../api/client'
import { useMountsStore } from '../../stores/mounts'
import type { MountInfo } from '../../types/api'
import HostDirPickerModal from './HostDirPickerModal.vue'

const mountsStore = useMountsStore()

const mounts = ref<readonly MountInfo[]>([])
const loading = ref(false)
const errorMessage = ref('')

// Create form state
const newHostPath = ref('')
const newName = ref('')
const newReadOnly = ref(false)
const newPrivateOnly = ref(false)
const creating = ref(false)
const pickerOpen = ref(false)

// Edit modal state
const editing = ref<{
  readonly id: string
  readonly hostPath: string
  name: string
  readOnly: boolean
  privateOnly: boolean
} | null>(null)

// Unmount confirm state
const unmounting = ref<MountInfo | null>(null)

async function refreshMounts(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    mounts.value = await api.listMounts()
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '마운트 목록을 불러오지 못했습니다.'
  } finally {
    loading.value = false
  }
}

function handlePickedPath(selectedPath: string): void {
  newHostPath.value = selectedPath
  if (!newName.value.trim()) {
    const segs = selectedPath.split('/').filter(Boolean)
    const leaf = segs[segs.length - 1]
    newName.value = leaf ?? 'Root'
  }
  pickerOpen.value = false
}

async function handleCreateMount(): Promise<void> {
  if (!newHostPath.value.trim()) {
    errorMessage.value = '마운트할 호스트 디렉토리 경로를 선택하거나 입력하세요.'
    return
  }
  creating.value = true
  errorMessage.value = ''
  try {
    await api.createMount({
      name: newName.value.trim(),
      hostPath: newHostPath.value.trim(),
      readOnly: newReadOnly.value,
      privateOnly: newPrivateOnly.value,
    })
    newHostPath.value = ''
    newName.value = ''
    newReadOnly.value = false
    newPrivateOnly.value = false
    await refreshMounts()
    mountsStore.notifyChanged()
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '마운트 등록에 실패했습니다.'
  } finally {
    creating.value = false
  }
}

function openEdit(item: MountInfo): void {
  editing.value = {
    id: item.id,
    hostPath: item.hostPath,
    name: item.name,
    readOnly: item.readOnly,
    privateOnly: item.privateOnly,
  }
}

async function saveEdit(): Promise<void> {
  const target = editing.value
  if (!target) return
  errorMessage.value = ''
  try {
    await api.updateMount({
      id: target.id,
      name: target.name,
      readOnly: target.readOnly,
      privateOnly: target.privateOnly,
    })
    editing.value = null
    await refreshMounts()
    mountsStore.notifyChanged()
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '마운트 수정에 실패했습니다.'
  }
}

async function confirmUnmount(): Promise<void> {
  const target = unmounting.value
  if (!target) return
  errorMessage.value = ''
  try {
    await api.deleteMount(target.id)
    unmounting.value = null
    await refreshMounts()
    mountsStore.notifyChanged()
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '마운트 해제에 실패했습니다.'
  }
}

onMounted(() => {
  void refreshMounts()
})
</script>

<template>
  <div class="flex h-full w-full flex-col overflow-y-auto p-5 text-xs text-slate-200">
    <!-- Add Mount Form -->
    <section class="mb-5 rounded-2xl border border-white/10 bg-slate-950/60 p-4">
      <div class="mb-3 flex items-center justify-between">
        <div>
          <h3 class="text-sm font-semibold text-white">서버 로컬 디렉토리 마운트 추가</h3>
          <p class="text-[11px] text-slate-400">
            서버 컴퓨터의 기존 폴더를 File Station에 연결합니다. 재시작 없이 즉시 반영됩니다.
          </p>
        </div>
        <button
          type="button"
          class="inline-flex items-center gap-1.5 rounded-lg border border-sky-400/30 bg-sky-500/15 px-3 py-1.5 font-semibold text-sky-300 hover:bg-sky-500/25"
          @click="pickerOpen = true"
        >
          <FolderSearch class="h-4 w-4" />
          서버 폴더 찾아보기
        </button>
      </div>

      <form class="space-y-3" @submit.prevent="handleCreateMount">
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div>
            <label class="mb-1 block text-[11px] text-slate-400">호스트 실제 경로</label>
            <input
              v-model="newHostPath"
              type="text"
              placeholder="예: /mnt/hdd/movies 또는 ~/Movies"
              class="w-full rounded-xl border border-white/10 bg-slate-900 px-3 py-2 font-mono text-xs text-white focus:border-sky-400 focus:outline-none"
            />
          </div>

          <div>
            <label class="mb-1 block text-[11px] text-slate-400">표시 이름 (비워두면 폴더명 사용)</label>
            <input
              v-model="newName"
              type="text"
              placeholder="예: 외장하드 영화"
              class="w-full rounded-xl border border-white/10 bg-slate-900 px-3 py-2 text-xs text-white focus:border-sky-400 focus:outline-none"
            />
          </div>
        </div>

        <div class="flex flex-wrap items-center justify-between gap-3 pt-1">
          <div class="flex flex-wrap items-center gap-4">
            <label class="inline-flex items-center gap-2 text-slate-300">
              <input v-model="newReadOnly" type="checkbox" class="rounded border-white/20 bg-slate-900" />
              읽기 전용 (업로드·수정·삭제 차단)
            </label>
            <label class="inline-flex items-center gap-2 text-slate-300">
              <input v-model="newPrivateOnly" type="checkbox" class="rounded border-white/20 bg-slate-900" />
              Tailscale 내부망 전용 (외부 NPM에서 숨김)
            </label>
          </div>

          <button
            type="submit"
            :disabled="creating"
            class="inline-flex items-center gap-1.5 rounded-xl bg-sky-600 px-4 py-2 font-semibold text-white hover:bg-sky-500 disabled:opacity-50"
          >
            <Plus class="h-4 w-4" />
            {{ creating ? '연결 중...' : '마운트 연결' }}
          </button>
        </div>
      </form>
    </section>

    <div
      v-if="errorMessage"
      class="mb-4 rounded-xl border border-rose-500/30 bg-rose-500/15 px-3.5 py-2.5 text-rose-200"
    >
      {{ errorMessage }}
    </div>

    <!-- Active Mounts List -->
    <section class="flex-1 rounded-2xl border border-white/10 bg-slate-950/60 p-4">
      <div class="mb-3 flex items-center justify-between">
        <h3 class="text-sm font-semibold text-white">연결된 마운트 목록 ({{ mounts.length }})</h3>
        <button
          type="button"
          class="rounded-lg p-1.5 text-slate-400 hover:bg-white/10 hover:text-white"
          title="새로고침"
          @click="refreshMounts"
        >
          <RefreshCw class="h-3.5 w-3.5" :class="{ 'animate-spin': loading }" />
        </button>
      </div>

      <div v-if="mounts.length === 0" class="py-10 text-center text-slate-500">
        연결된 마운트가 없습니다. 위에서 서버 디렉토리를 추가하세요.
      </div>

      <div v-else class="space-y-2">
        <div
          v-for="item in mounts"
          :key="item.id"
          class="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-white/10 bg-slate-900/80 px-3.5 py-3"
        >
          <div class="min-w-0 flex-1">
            <div class="flex flex-wrap items-center gap-2">
              <HardDrive class="h-4 w-4 shrink-0 text-sky-400" />
              <span class="font-semibold text-white">{{ item.name }}</span>

              <span
                v-if="item.readOnly"
                class="inline-flex items-center gap-1 rounded-md bg-amber-500/15 px-2 py-0.5 text-[10px] font-semibold text-amber-300"
              >
                <Lock class="h-3 w-3" />
                읽기 전용
              </span>
              <span
                v-else
                class="rounded-md bg-emerald-500/15 px-2 py-0.5 text-[10px] font-semibold text-emerald-300"
              >
                읽기/쓰기
              </span>

              <span
                v-if="item.privateOnly"
                class="inline-flex items-center gap-1 rounded-md bg-violet-500/15 px-2 py-0.5 text-[10px] font-semibold text-violet-300"
              >
                <ShieldCheck class="h-3 w-3" />
                Tailscale 전용
              </span>
              <span
                v-else
                class="inline-flex items-center gap-1 rounded-md bg-sky-500/15 px-2 py-0.5 text-[10px] font-semibold text-sky-300"
              >
                <Globe class="h-3 w-3" />
                Public 허용
              </span>

              <span
                v-if="!item.exists"
                class="inline-flex items-center gap-1 rounded-md bg-rose-500/20 px-2 py-0.5 text-[10px] font-semibold text-rose-300"
              >
                <AlertTriangle class="h-3 w-3" />
                경로 없음
              </span>
            </div>

            <div class="mt-1 truncate font-mono text-[11px] text-slate-400">
              {{ item.hostPath }}
            </div>
          </div>

          <div class="flex shrink-0 items-center gap-1.5">
            <button
              type="button"
              class="inline-flex items-center gap-1 rounded-lg bg-white/5 px-2.5 py-1.5 text-slate-200 hover:bg-white/10"
              @click="openEdit(item)"
            >
              <Pencil class="h-3.5 w-3.5" />
              설정
            </button>
            <button
              type="button"
              class="inline-flex items-center gap-1 rounded-lg bg-rose-500/10 px-2.5 py-1.5 text-rose-300 hover:bg-rose-500/20"
              @click="unmounting = item"
            >
              <Trash2 class="h-3.5 w-3.5" />
              해제
            </button>
          </div>
        </div>
      </div>
    </section>

    <!-- Host Directory Picker Modal -->
    <HostDirPickerModal
      v-if="pickerOpen"
      :initial-path="newHostPath"
      @select="handlePickedPath"
      @close="pickerOpen = false"
    />

    <!-- Edit Mount Modal -->
    <div
      v-if="editing"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
      @click.self="editing = null"
    >
      <form
        class="w-full max-w-md space-y-3 rounded-2xl border border-white/15 bg-slate-900 p-5 shadow-2xl"
        @submit.prevent="saveEdit"
      >
        <h4 class="text-sm font-semibold text-white">마운트 설정 수정</h4>
        <div class="truncate font-mono text-[11px] text-slate-400">{{ editing.hostPath }}</div>

        <div>
          <label class="mb-1 block text-[11px] text-slate-400">표시 이름</label>
          <input
            v-model="editing.name"
            type="text"
            class="w-full rounded-xl border border-white/10 bg-slate-950 px-3 py-2 text-xs text-white focus:border-sky-400 focus:outline-none"
          />
        </div>

        <label class="flex items-center gap-2 text-slate-300">
          <input v-model="editing.readOnly" type="checkbox" class="rounded border-white/20 bg-slate-950" />
          읽기 전용 (수정·업로드·삭제 차단)
        </label>

        <label class="flex items-center gap-2 text-slate-300">
          <input v-model="editing.privateOnly" type="checkbox" class="rounded border-white/20 bg-slate-950" />
          Tailscale 내부망 전용 (외부 NPM에서 숨김)
        </label>

        <div class="flex justify-end gap-2 pt-2">
          <button
            type="button"
            class="rounded-lg px-3 py-1.5 text-slate-400 hover:bg-white/5"
            @click="editing = null"
          >
            취소
          </button>
          <button
            type="submit"
            class="rounded-lg bg-sky-600 px-3.5 py-1.5 font-semibold text-white hover:bg-sky-500"
          >
            저장
          </button>
        </div>
      </form>
    </div>

    <!-- Unmount Confirmation Modal -->
    <div
      v-if="unmounting"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
      @click.self="unmounting = null"
    >
      <div class="w-full max-w-sm rounded-2xl border border-white/15 bg-slate-900 p-5 shadow-2xl">
        <h4 class="mb-2 text-sm font-semibold text-white">마운트 연결 해제</h4>
        <p class="mb-4 text-xs leading-relaxed text-slate-300">
          <strong class="text-white">{{ unmounting.name }}</strong> 마운트 연결을 해제할까요?<br />
          서버 디스크의 실제 파일(<span class="font-mono text-slate-400">{{ unmounting.hostPath }}</span>)은 삭제되지 않습니다.
        </p>
        <div class="flex justify-end gap-2">
          <button
            type="button"
            class="rounded-lg px-3 py-1.5 text-slate-400 hover:bg-white/5"
            @click="unmounting = null"
          >
            취소
          </button>
          <button
            type="button"
            class="rounded-lg bg-rose-600 px-3.5 py-1.5 font-semibold text-white hover:bg-rose-500"
            @click="confirmUnmount"
          >
            연결 해제
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
