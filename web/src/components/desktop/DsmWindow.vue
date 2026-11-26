<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { Minus, Square, Minimize2, X } from 'lucide-vue-next'
import { useDesktopStore, type WindowInstance } from '../../stores/desktop'
import DsmAppIcon from './DsmAppIcon.vue'

type ResizeDirection = 'n' | 's' | 'e' | 'w' | 'ne' | 'nw' | 'se' | 'sw'

const TOPBAR_HEIGHT = 36
const MIN_WIDTH = 380
const MIN_HEIGHT = 280

const props = defineProps<{
  readonly win: WindowInstance
  readonly isActive: boolean
}>()

const desktopStore = useDesktopStore()

const isMobile = ref(false)

function checkViewport(): void {
  isMobile.value = window.innerWidth < 768
}

onMounted(() => {
  checkViewport()
  window.addEventListener('resize', checkViewport)
})

onUnmounted(() => {
  window.removeEventListener('resize', checkViewport)
})

const windowStyle = computed(() => {
  if (isMobile.value || props.win.maximized) {
    return {
      left: '0px',
      top: `${TOPBAR_HEIGHT}px`,
      width: '100vw',
      height: `calc(100vh - ${TOPBAR_HEIGHT}px)`,
      zIndex: props.win.zIndex,
    }
  }
  return {
    left: `${props.win.x}px`,
    top: `${props.win.y}px`,
    width: `${props.win.width}px`,
    height: `${props.win.height}px`,
    zIndex: props.win.zIndex,
  }
})

function startDrag(ev: PointerEvent): void {
  if (isMobile.value || props.win.maximized) return
  if (ev.button !== 0) return
  desktopStore.focusWindow(props.win.id)

  const startX = ev.clientX
  const startY = ev.clientY
  const origX = props.win.x
  const origY = props.win.y
  const width = props.win.width
  const height = props.win.height

  const onMove = (moveEv: PointerEvent) => {
    const nx = Math.max(-width + 120, Math.min(window.innerWidth - 80, origX + (moveEv.clientX - startX)))
    const ny = Math.max(TOPBAR_HEIGHT, Math.min(window.innerHeight - TOPBAR_HEIGHT, origY + (moveEv.clientY - startY)))
    desktopStore.updateBounds({
      windowId: props.win.id,
      x: nx,
      y: ny,
      width,
      height,
    })
  }

  const onUp = () => {
    window.removeEventListener('pointermove', onMove)
    window.removeEventListener('pointerup', onUp)
  }

  window.addEventListener('pointermove', onMove)
  window.addEventListener('pointerup', onUp)
}

function startResize(ev: PointerEvent, dir: ResizeDirection): void {
  if (isMobile.value || props.win.maximized) return
  if (ev.button !== 0) return
  ev.stopPropagation()
  ev.preventDefault()
  desktopStore.focusWindow(props.win.id)

  const startX = ev.clientX
  const startY = ev.clientY
  const origX = props.win.x
  const origY = props.win.y
  const origW = props.win.width
  const origH = props.win.height
  const rightEdge = origX + origW
  const bottomEdge = origY + origH

  const resizesEast = dir === 'e' || dir === 'ne' || dir === 'se'
  const resizesWest = dir === 'w' || dir === 'nw' || dir === 'sw'
  const resizesSouth = dir === 's' || dir === 'se' || dir === 'sw'
  const resizesNorth = dir === 'n' || dir === 'ne' || dir === 'nw'

  const onMove = (moveEv: PointerEvent) => {
    const dx = moveEv.clientX - startX
    const dy = moveEv.clientY - startY

    let nx = origX
    let ny = origY
    let nw = origW
    let nh = origH

    if (resizesEast) {
      const maxW = Math.max(MIN_WIDTH, window.innerWidth - origX)
      nw = Math.max(MIN_WIDTH, Math.min(maxW, origW + dx))
    } else if (resizesWest) {
      const maxX = rightEdge - MIN_WIDTH
      nx = Math.max(0, Math.min(maxX, origX + dx))
      nw = rightEdge - nx
    }

    if (resizesSouth) {
      const maxH = Math.max(MIN_HEIGHT, window.innerHeight - origY)
      nh = Math.max(MIN_HEIGHT, Math.min(maxH, origH + dy))
    } else if (resizesNorth) {
      const maxY = bottomEdge - MIN_HEIGHT
      ny = Math.max(TOPBAR_HEIGHT, Math.min(maxY, origY + dy))
      nh = bottomEdge - ny
    }

    desktopStore.updateBounds({
      windowId: props.win.id,
      x: nx,
      y: ny,
      width: nw,
      height: nh,
    })
  }

  const onUp = () => {
    window.removeEventListener('pointermove', onMove)
    window.removeEventListener('pointerup', onUp)
  }

  window.addEventListener('pointermove', onMove)
  window.addEventListener('pointerup', onUp)
}
</script>

<template>
  <section
    v-show="!win.minimized"
    :style="windowStyle"
    class="fixed flex flex-col border bg-white text-[#1e293b] transition-shadow"
    :class="[
      isMobile || win.maximized ? 'rounded-none' : 'rounded-[7px]',
      isActive
        ? 'border-slate-500/70 shadow-[0_22px_56px_rgba(0,0,0,0.38),0_2px_10px_rgba(0,0,0,0.18)]'
        : 'border-slate-400/60 shadow-[0_18px_48px_rgba(0,0,0,0.32),0_2px_8px_rgba(0,0,0,0.14)]',
    ]"
    @pointerdown="desktopStore.focusWindow(win.id)"
  >
    <div
      class="flex h-full w-full flex-col overflow-hidden"
      :class="isMobile || win.maximized ? 'rounded-none' : 'rounded-[6px]'"
    >
      <div
        class="flex h-[36px] shrink-0 select-none items-center justify-between border-b border-[#dce2ea] px-3"
        :class="isActive ? 'bg-[#eef1f5]' : 'bg-[#f7f9fb]'"
        @pointerdown="startDrag"
        @dblclick="desktopStore.toggleMaximize(win.id)"
      >
        <div class="flex min-w-0 items-center gap-2 pr-3">
          <DsmAppIcon :app-id="win.appId" size="xs" />
          <span class="truncate text-[12.5px] font-semibold text-[#1e293b]">{{ win.title }}</span>
        </div>

        <div class="flex shrink-0 items-center gap-1" @pointerdown.stop>
          <button
            type="button"
            class="flex h-6 w-6 cursor-pointer items-center justify-center rounded-[4px] text-[#64748b] transition-colors hover:bg-black/6 hover:text-[#1e293b]"
            title="최소화"
            @click="desktopStore.minimizeWindow(win.id)"
          >
            <Minus class="h-3.5 w-3.5" />
          </button>
          <button
            v-if="!isMobile"
            type="button"
            class="flex h-6 w-6 cursor-pointer items-center justify-center rounded-[4px] text-[#64748b] transition-colors hover:bg-black/6 hover:text-[#1e293b]"
            :title="win.maximized ? '이전 크기로' : '최대화'"
            @click="desktopStore.toggleMaximize(win.id)"
          >
            <Minimize2 v-if="win.maximized" class="h-3.5 w-3.5" />
            <Square v-else class="h-3 w-3" />
          </button>
          <button
            type="button"
            class="flex h-6 w-6 cursor-pointer items-center justify-center rounded-[4px] text-[#64748b] transition-colors hover:bg-[#e11d48] hover:text-white"
            title="닫기"
            @click="desktopStore.closeWindow(win.id)"
          >
            <X class="h-3.5 w-3.5" />
          </button>
        </div>
      </div>

      <div class="relative min-h-0 flex-1 overflow-hidden bg-white text-[#1e293b]">
        <slot />
      </div>
    </div>

    <template v-if="!isMobile && !win.maximized">
      <div
        class="absolute -top-[3px] right-2.5 left-2.5 z-20 h-[6px] cursor-ns-resize"
        @pointerdown="startResize($event, 'n')"
      />
      <div
        class="absolute right-2.5 -bottom-[3px] left-2.5 z-20 h-[6px] cursor-ns-resize"
        @pointerdown="startResize($event, 's')"
      />
      <div
        class="absolute top-2.5 bottom-2.5 -left-[3px] z-20 w-[6px] cursor-ew-resize"
        @pointerdown="startResize($event, 'w')"
      />
      <div
        class="absolute top-2.5 -right-[3px] bottom-2.5 z-20 w-[6px] cursor-ew-resize"
        @pointerdown="startResize($event, 'e')"
      />
      <div
        class="absolute -top-[3px] -left-[3px] z-30 h-3 w-3 cursor-nwse-resize"
        @pointerdown="startResize($event, 'nw')"
      />
      <div
        class="absolute -top-[3px] -right-[3px] z-30 h-3 w-3 cursor-nesw-resize"
        @pointerdown="startResize($event, 'ne')"
      />
      <div
        class="absolute -bottom-[3px] -left-[3px] z-30 h-3 w-3 cursor-nesw-resize"
        @pointerdown="startResize($event, 'sw')"
      />
      <div
        class="absolute -right-[3px] -bottom-[3px] z-30 h-3.5 w-3.5 cursor-nwse-resize"
        @pointerdown="startResize($event, 'se')"
      >
        <svg class="h-full w-full text-slate-400/75" viewBox="0 0 16 16">
          <path d="M13 13H9.5M13 9.5V13M6.5 13H13V6.5" stroke="currentColor" stroke-width="1.3" fill="none" />
        </svg>
      </div>
    </template>
  </section>
</template>
