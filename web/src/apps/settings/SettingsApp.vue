<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Check, Clock, Globe, Image as ImageIcon, RotateCcw, Save, Trash2, Upload } from 'lucide-vue-next'
import { useSettingsStore } from '../../stores/settings'
import type { ClockFormat, WallpaperPreset } from '../../types/api'
import DsmWallpaper from '../../components/desktop/DsmWallpaper.vue'

type SettingsSection = 'branding' | 'wallpaper' | 'clock'

const settingsStore = useSettingsStore()

const activeSection = ref<SettingsSection>('branding')
const siteTitleInput = ref(settingsStore.siteTitle)
const busy = ref(false)
const statusMessage = ref('')
const errorMessage = ref('')

const faviconInputRef = ref<HTMLInputElement | null>(null)
const wallpaperInputRef = ref<HTMLInputElement | null>(null)

const WALLPAPER_PRESETS: readonly { readonly id: WallpaperPreset; readonly label: string; readonly desc: string }[] = [
  { id: 'ocean', label: '오션 블루', desc: '기본 웨이브 블루 테마' },
  { id: 'mist', label: '노르딕 미스트', desc: '부드러운 청록색 데이라이트' },
  { id: 'slate', label: '딥 슬레이트', desc: '차분한 다크 네이비' },
]

watch(
  () => settingsStore.siteTitle,
  (next) => {
    siteTitleInput.value = next
  },
)

function formatDatePrefix(): string {
  const now = new Date()
  const y = now.getFullYear()
  const m = String(now.getMonth() + 1).padStart(2, '0')
  const d = String(now.getDate()).padStart(2, '0')
  return `${y}-${m}-${d}`
}

const clockPreview24 = computed(() => {
  const timeStr = new Date().toLocaleTimeString('ko-KR', {
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  })
  return `${formatDatePrefix()} ${timeStr}`
})

const clockPreview12 = computed(() => {
  const timeStr = new Date().toLocaleTimeString('ko-KR', {
    hour: '2-digit',
    minute: '2-digit',
    hour12: true,
  })
  return `${formatDatePrefix()} ${timeStr}`
})

function showSuccess(msg: string): void {
  errorMessage.value = ''
  statusMessage.value = msg
  setTimeout(() => {
    if (statusMessage.value === msg) {
      statusMessage.value = ''
    }
  }, 2600)
}

async function handleSaveTitle(): Promise<void> {
  if (busy.value) return
  busy.value = true
  errorMessage.value = ''
  try {
    await settingsStore.save({ siteTitle: siteTitleInput.value.trim() || 'Personal Server' })
    showSuccess('브라우저 탭 이름이 저장되었습니다.')
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '탭 이름 저장에 실패했습니다.'
  } finally {
    busy.value = false
  }
}

async function handleFaviconPicked(ev: Event): Promise<void> {
  if (!(ev.target instanceof HTMLInputElement)) return
  const file = ev.target.files?.[0]
  ev.target.value = ''
  if (!file || busy.value) return

  busy.value = true
  errorMessage.value = ''
  try {
    await settingsStore.uploadFavicon(file)
    showSuccess('파비콘이 변경되었습니다.')
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '파비콘 업로드에 실패했습니다.'
  } finally {
    busy.value = false
  }
}

async function handleResetFavicon(): Promise<void> {
  if (busy.value) return
  busy.value = true
  errorMessage.value = ''
  try {
    await settingsStore.resetFavicon()
    showSuccess('기본 파비콘으로 복원되었습니다.')
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '파비콘 복원에 실패했습니다.'
  } finally {
    busy.value = false
  }
}

async function handleSelectPreset(preset: WallpaperPreset): Promise<void> {
  if (busy.value) return
  busy.value = true
  errorMessage.value = ''
  try {
    await settingsStore.save({ wallpaperMode: 'preset', wallpaperPreset: preset })
    showSuccess('배경화면 테마가 적용되었습니다.')
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '배경화면 변경에 실패했습니다.'
  } finally {
    busy.value = false
  }
}

async function handleUseCustomWallpaper(): Promise<void> {
  if (busy.value || !settingsStore.hasCustomWallpaper) return
  busy.value = true
  errorMessage.value = ''
  try {
    await settingsStore.save({ wallpaperMode: 'custom' })
    showSuccess('업로드한 배경화면이 적용되었습니다.')
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '배경화면 변경에 실패했습니다.'
  } finally {
    busy.value = false
  }
}

async function handleWallpaperPicked(ev: Event): Promise<void> {
  if (!(ev.target instanceof HTMLInputElement)) return
  const file = ev.target.files?.[0]
  ev.target.value = ''
  if (!file || busy.value) return

  busy.value = true
  errorMessage.value = ''
  try {
    await settingsStore.uploadWallpaper(file)
    showSuccess('새 배경화면이 업로드되어 적용되었습니다.')
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '배경화면 업로드에 실패했습니다.'
  } finally {
    busy.value = false
  }
}

async function handleDeleteWallpaper(): Promise<void> {
  if (busy.value) return
  busy.value = true
  errorMessage.value = ''
  try {
    await settingsStore.resetWallpaper()
    showSuccess('사용자 배경화면이 삭제되었습니다.')
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '배경화면 삭제에 실패했습니다.'
  } finally {
    busy.value = false
  }
}

async function handleSelectClockFormat(format: ClockFormat): Promise<void> {
  if (busy.value || settingsStore.clockFormat === format) return
  busy.value = true
  errorMessage.value = ''
  try {
    await settingsStore.save({ clockFormat: format })
    showSuccess(format === '12h' ? '12시간제로 변경되었습니다.' : '24시간제로 변경되었습니다.')
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '시간 형식 변경에 실패했습니다.'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="flex h-full w-full overflow-hidden bg-[#f4f6f9] text-[12px] text-[#1e293b]">
    <aside class="flex w-44 shrink-0 flex-col border-r border-[#dce2ea] bg-[#f5f7fa] p-2">
      <div class="mb-1.5 px-2.5 pt-1 text-[11px] font-semibold text-[#64748b]">개인화 및 표시</div>
      <nav class="space-y-1">
        <button
          type="button"
          class="flex w-full cursor-pointer items-center gap-2 rounded-[4px] px-2.5 py-2 text-left text-[12px] transition"
          :class="
            activeSection === 'branding'
              ? 'bg-[#e6f0ff] font-semibold text-[#0058cc]'
              : 'text-[#334155] hover:bg-[#eaeef4]'
          "
          @click="activeSection = 'branding'"
        >
          <Globe class="h-4 w-4 shrink-0" />
          <span class="truncate">탭 이름 · 파비콘</span>
        </button>

        <button
          type="button"
          class="flex w-full cursor-pointer items-center gap-2 rounded-[4px] px-2.5 py-2 text-left text-[12px] transition"
          :class="
            activeSection === 'wallpaper'
              ? 'bg-[#e6f0ff] font-semibold text-[#0058cc]'
              : 'text-[#334155] hover:bg-[#eaeef4]'
          "
          @click="activeSection = 'wallpaper'"
        >
          <ImageIcon class="h-4 w-4 shrink-0" />
          <span class="truncate">배경화면</span>
        </button>

        <button
          type="button"
          class="flex w-full cursor-pointer items-center gap-2 rounded-[4px] px-2.5 py-2 text-left text-[12px] transition"
          :class="
            activeSection === 'clock'
              ? 'bg-[#e6f0ff] font-semibold text-[#0058cc]'
              : 'text-[#334155] hover:bg-[#eaeef4]'
          "
          @click="activeSection = 'clock'"
        >
          <Clock class="h-4 w-4 shrink-0" />
          <span class="truncate">시계 형식</span>
        </button>
      </nav>
    </aside>

    <div class="min-w-0 flex-1 overflow-y-auto p-4">
      <div
        v-if="errorMessage"
        class="mb-3 rounded-[4px] border border-[#fecdd3] bg-[#fff1f2] px-3 py-2 text-[12px] font-medium text-[#be123c]"
      >
        {{ errorMessage }}
      </div>
      <div
        v-if="statusMessage"
        class="mb-3 flex items-center gap-1.5 rounded-[4px] border border-[#bbf7d0] bg-[#f0fdf4] px-3 py-2 text-[12px] font-medium text-[#15803d]"
      >
        <Check class="h-3.5 w-3.5 shrink-0" />
        <span>{{ statusMessage }}</span>
      </div>

      <div v-if="activeSection === 'branding'" class="space-y-4">
        <section class="rounded-[6px] border border-[#dce2ea] bg-white p-4 shadow-2xs">
          <h3 class="text-[13px] font-semibold text-[#1e293b]">브라우저 탭 이름</h3>
          <p class="mt-0.5 text-[11.5px] text-[#64748b]">
            크롬 브라우저 탭과 로그인 화면 제목에 표시되는 서버 이름입니다.
          </p>

          <form class="mt-3 flex flex-nowrap items-center gap-2" @submit.prevent="handleSaveTitle">
            <input
              v-model="siteTitleInput"
              type="text"
              maxlength="60"
              placeholder="Personal Server"
              class="dsm-input max-w-sm flex-1"
            />
            <button type="submit" class="dsm-btn dsm-btn-primary shrink-0" :disabled="busy">
              <Save class="h-3.5 w-3.5" />
              <span>저장</span>
            </button>
          </form>

          <div class="mt-3 inline-flex items-center gap-2 rounded-t-[6px] border border-[#dce2ea] bg-[#f8fafc] px-3 py-1.5 text-[11.5px] text-[#334155]">
            <img
              v-if="settingsStore.customFaviconUrl"
              :src="settingsStore.customFaviconUrl"
              alt=""
              class="h-3.5 w-3.5 rounded-[2px] object-contain"
            />
            <Globe v-else class="h-3.5 w-3.5 text-[#0572ec]" />
            <span class="max-w-[200px] truncate font-medium">{{ siteTitleInput.trim() || 'Personal Server' }}</span>
          </div>
        </section>

        <section class="rounded-[6px] border border-[#dce2ea] bg-white p-4 shadow-2xs">
          <h3 class="text-[13px] font-semibold text-[#1e293b]">파비콘 (Favicon)</h3>
          <p class="mt-0.5 text-[11.5px] text-[#64748b]">
            브라우저 탭 왼쪽에 표시되는 아이콘을 직접 업로드할 수 있습니다. (.ico, .png, .svg, .jpg, .webp / 최대 2MB)
          </p>

          <input
            ref="faviconInputRef"
            type="file"
            accept=".ico,.png,.svg,.jpg,.jpeg,.webp,image/*"
            class="hidden"
            @change="handleFaviconPicked"
          />

          <div class="mt-3 flex flex-nowrap items-center gap-3">
            <div class="flex h-12 w-12 shrink-0 items-center justify-center overflow-hidden rounded-[6px] border border-[#dce2ea] bg-[#f8fafc]">
              <img
                v-if="settingsStore.customFaviconUrl"
                :src="settingsStore.customFaviconUrl"
                alt="현재 파비콘"
                class="h-8 w-8 object-contain"
              />
              <Globe v-else class="h-6 w-6 text-[#0572ec]" />
            </div>

            <div class="flex flex-nowrap items-center gap-2">
              <button
                type="button"
                class="dsm-btn dsm-btn-primary"
                :disabled="busy"
                @click="faviconInputRef?.click()"
              >
                <Upload class="h-3.5 w-3.5" />
                <span>파비콘 업로드</span>
              </button>

              <button
                v-if="settingsStore.hasCustomFavicon"
                type="button"
                class="dsm-btn"
                :disabled="busy"
                @click="handleResetFavicon"
              >
                <RotateCcw class="h-3.5 w-3.5" />
                <span>기본값 복원</span>
              </button>
            </div>
          </div>
        </section>
      </div>

      <div v-else-if="activeSection === 'wallpaper'" class="space-y-4">
        <section class="rounded-[6px] border border-[#dce2ea] bg-white p-4 shadow-2xs">
          <h3 class="text-[13px] font-semibold text-[#1e293b]">기본 배경화면 테마</h3>
          <p class="mt-0.5 text-[11.5px] text-[#64748b]">
            로그인 화면과 바탕화면에 즉시 반영됩니다.
          </p>

          <div class="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-3">
            <button
              v-for="preset in WALLPAPER_PRESETS"
              :key="preset.id"
              type="button"
              class="group flex cursor-pointer flex-col overflow-hidden rounded-[6px] border text-left transition"
              :class="
                settingsStore.wallpaperMode === 'preset' && settingsStore.wallpaperPreset === preset.id
                  ? 'border-[#0572ec] ring-2 ring-[#0572ec]/25'
                  : 'border-[#dce2ea] hover:border-[#94a3b8]'
              "
              :disabled="busy"
              @click="handleSelectPreset(preset.id)"
            >
              <div class="relative h-24 w-full overflow-hidden">
                <DsmWallpaper :theme="preset.id" />
                <span
                  v-if="settingsStore.wallpaperMode === 'preset' && settingsStore.wallpaperPreset === preset.id"
                  class="absolute top-2 right-2 flex h-5 w-5 items-center justify-center rounded-full bg-[#0572ec] text-white shadow"
                >
                  <Check class="h-3 w-3" />
                </span>
              </div>
              <div class="bg-white px-2.5 py-2">
                <div class="text-[12px] font-semibold text-[#1e293b]">{{ preset.label }}</div>
                <div class="text-[10.5px] text-[#64748b]">{{ preset.desc }}</div>
              </div>
            </button>
          </div>
        </section>

        <section class="rounded-[6px] border border-[#dce2ea] bg-white p-4 shadow-2xs">
          <div class="flex flex-nowrap items-center justify-between gap-2">
            <div>
              <h3 class="text-[13px] font-semibold text-[#1e293b]">사용자 지정 배경화면</h3>
              <p class="mt-0.5 text-[11.5px] text-[#64748b]">
                원하는 이미지를 업로드해 배경화면으로 사용합니다. (.jpg, .png, .webp / 최대 15MB)
              </p>
            </div>

            <input
              ref="wallpaperInputRef"
              type="file"
              accept=".jpg,.jpeg,.png,.webp,image/jpeg,image/png,image/webp"
              class="hidden"
              @change="handleWallpaperPicked"
            />

            <button
              type="button"
              class="dsm-btn dsm-btn-primary shrink-0"
              :disabled="busy"
              @click="wallpaperInputRef?.click()"
            >
              <Upload class="h-3.5 w-3.5" />
              <span>이미지 업로드</span>
            </button>
          </div>

          <div
            v-if="settingsStore.customWallpaperUrl"
            class="mt-3 flex flex-col gap-2.5 sm:flex-row sm:items-center"
          >
            <div
              class="relative h-28 w-48 shrink-0 overflow-hidden rounded-[6px] border"
              :class="
                settingsStore.wallpaperMode === 'custom'
                  ? 'border-[#0572ec] ring-2 ring-[#0572ec]/25'
                  : 'border-[#dce2ea]'
              "
            >
              <img :src="settingsStore.customWallpaperUrl" alt="사용자 배경화면" class="h-full w-full object-cover" />
              <span
                v-if="settingsStore.wallpaperMode === 'custom'"
                class="absolute top-2 right-2 flex h-5 w-5 items-center justify-center rounded-full bg-[#0572ec] text-white shadow"
              >
                <Check class="h-3 w-3" />
              </span>
            </div>

            <div class="flex flex-nowrap items-center gap-2">
              <button
                v-if="settingsStore.wallpaperMode !== 'custom'"
                type="button"
                class="dsm-btn"
                :disabled="busy"
                @click="handleUseCustomWallpaper"
              >
                <Check class="h-3.5 w-3.5 text-[#0572ec]" />
                <span>이 이미지 적용</span>
              </button>

              <button
                type="button"
                class="dsm-btn dsm-btn-danger"
                :disabled="busy"
                @click="handleDeleteWallpaper"
              >
                <Trash2 class="h-3.5 w-3.5" />
                <span>삭제</span>
              </button>
            </div>
          </div>
        </section>
      </div>

      <div v-else class="space-y-4">
        <section class="rounded-[6px] border border-[#dce2ea] bg-white p-4 shadow-2xs">
          <h3 class="text-[13px] font-semibold text-[#1e293b]">상단 바 시계 표시 형식</h3>
          <p class="mt-0.5 text-[11.5px] text-[#64748b]">
            바탕화면 상단 메뉴바 오른쪽 시계의 표기 방식을 선택합니다.
          </p>

          <div class="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2">
            <button
              type="button"
              class="flex cursor-pointer items-center justify-between rounded-[6px] border px-4 py-3 text-left transition"
              :class="
                settingsStore.clockFormat === '24h'
                  ? 'border-[#0572ec] bg-[#e6f0ff]/60 ring-2 ring-[#0572ec]/20'
                  : 'border-[#dce2ea] bg-white hover:bg-[#f8fafc]'
              "
              :disabled="busy"
              @click="handleSelectClockFormat('24h')"
            >
              <div>
                <div class="text-[12.5px] font-semibold text-[#1e293b]">24시간제</div>
                <div class="mt-0.5 font-mono text-[13px] tabular-nums text-[#475569]">{{ clockPreview24 }}</div>
              </div>
              <span
                v-if="settingsStore.clockFormat === '24h'"
                class="flex h-5 w-5 items-center justify-center rounded-full bg-[#0572ec] text-white"
              >
                <Check class="h-3 w-3" />
              </span>
            </button>

            <button
              type="button"
              class="flex cursor-pointer items-center justify-between rounded-[6px] border px-4 py-3 text-left transition"
              :class="
                settingsStore.clockFormat === '12h'
                  ? 'border-[#0572ec] bg-[#e6f0ff]/60 ring-2 ring-[#0572ec]/20'
                  : 'border-[#dce2ea] bg-white hover:bg-[#f8fafc]'
              "
              :disabled="busy"
              @click="handleSelectClockFormat('12h')"
            >
              <div>
                <div class="text-[12.5px] font-semibold text-[#1e293b]">12시간제 (오전/오후)</div>
                <div class="mt-0.5 font-mono text-[13px] tabular-nums text-[#475569]">{{ clockPreview12 }}</div>
              </div>
              <span
                v-if="settingsStore.clockFormat === '12h'"
                class="flex h-5 w-5 items-center justify-center rounded-full bg-[#0572ec] text-white"
              >
                <Check class="h-3 w-3" />
              </span>
            </button>
          </div>
        </section>
      </div>
    </div>
  </div>
</template>
