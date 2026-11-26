<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { Minus, Square, Minimize2, X } from 'lucide-vue-next'
import { useDesktopStore, type WindowInstance } from '../../stores/desktop'

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
      top: '44px',
      width: '100vw',
      height: 'calc(100vh - 44px)',
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
    const ny = Math.max(44, Math.min(window.innerHeight - 48, origY + (moveEv.clientY - startY)))
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

function startResize(ev: PointerEvent): void {
  if (isMobile.value || props.win.maximized) return
  if (ev.button !== 0) return
  ev.stopPropagation()
  desktopStore.focusWindow(props.win.id)

  const startX = ev.clientX
  const startY = ev.clientY
  const origW = props.win.width
  const origH = props.win.height
  const x = props.win.x
  const y = props.win.y

  const onMove = (moveEv: PointerEvent) => {
    const nw = Math.max(380, Math.min(window.innerWidth - x, origW + (moveEv.clientX - startX)))
    const nh = Math.max(280, Math.min(window.innerHeight - y, origH + (moveEv.clientY - startY)))
    desktopStore.updateBounds({
      windowId: props.win.id,
      x,
      y,
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
    class="fixed flex flex-col overflow-hidden border bg-slate-900/95 shadow-2xl backdrop-blur-xl transition-shadow"
    :class="[
      isMobile || win.maximized ? 'rounded-none border-white/10' : 'rounded-xl',
      isActive ? 'border-sky-400/40 ring-1 ring-sky-400/20' : 'border-white/10',
    ]"
    @pointerdown="desktopStore.focusWindow(win.id)"
  >
    <!-- Window Titlebar -->
    <div
      class="flex h-10 shrink-0 select-none items-center justify-between border-b border-white/10 bg-slate-950/75 px-3"
      @pointerdown="startDrag"
      @dblclick="desktopStore.toggleMaximize(win.id)"
    >
      <div class="truncate pr-4 text-xs font-semibold text-slate-100">{{ win.title }}</div>

      <div class="flex items-center gap-1" @pointerdown.stop>
        <button
          type="button"
          class="flex h-6 w-6 items-center justify-center rounded-md text-slate-400 hover:bg-white/10 hover:text-white"
          title="최소화"
          @click="desktopStore.minimizeWindow(win.id)"
        >
          <Minus class="h-3.5 w-3.5" />
        </button>
        <button
          v-if="!isMobile"
          type="button"
          class="flex h-6 w-6 items-center justify-center rounded-md text-slate-400 hover:bg-white/10 hover:text-white"
          :title="win.maximized ? '이전 크기로' : '최대화'"
          @click="desktopStore.toggleMaximize(win.id)"
        >
          <Minimize2 v-if="win.maximized" class="h-3.5 w-3.5" />
          <Square v-else class="h-3 w-3" />
        </button>
        <button
          type="button"
          class="flex h-6 w-6 items-center justify-center rounded-md text-slate-400 hover:bg-rose-500/80 hover:text-white"
          title="닫기"
          @click="desktopStore.closeWindow(win.id)"
        >
          <X class="h-3.5 w-3.5" />
        </button>
      </div>
    </div>

    <!-- Window Body -->
    <div class="relative min-h-0 flex-1 overflow-hidden bg-slate-900">
      <slot />
    </div>

    <!-- Corner Resize Handle -->
    <div
      v-if="!isMobile && !win.maximized"
      class="absolute right-0 bottom-0 h-4 w-4 cursor-nwse-resize"
      @pointerdown="startResize"
    >
      <svg class="h-full w-full text-slate-500/60" viewBox="0 0 16 16">
        <path d="M14 14H10M14 10V14M6 14H14V6" stroke="currentColor" stroke-width="1.5" fill="none" />
      </svg>
    </div>
  </section>
</template>
