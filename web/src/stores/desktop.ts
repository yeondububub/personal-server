import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

export type AppId =
  | 'file-station'
  | 'video-player'
  | 'code-editor'
  | 'mounts'
  | 'wargame'
  | 'terminal'
  | 'monitor'

export type WindowPayload =
  | { readonly kind: 'file-station'; readonly initialRootId?: string; readonly initialPath?: string }
  | { readonly kind: 'video-player'; readonly rootId: string; readonly relPath: string; readonly fileName: string }
  | { readonly kind: 'code-editor'; readonly rootId: string; readonly relPath: string; readonly fileName: string }
  | { readonly kind: 'mounts' }
  | { readonly kind: 'wargame' }
  | { readonly kind: 'terminal' }
  | { readonly kind: 'monitor' }

export interface DesktopAppDefinition {
  readonly id: AppId
  readonly title: string
  readonly subtitle: string
  readonly iconBg: string
  readonly requiresPrivate: boolean
  readonly showOnDesktop: boolean
  readonly defaultWidth: number
  readonly defaultHeight: number
}

export const DESKTOP_APPS = [
  {
    id: 'file-station',
    title: 'File Station',
    subtitle: '파일 탐색 · 업로드 · 공유',
    iconBg: 'from-sky-500 to-blue-600',
    requiresPrivate: false,
    showOnDesktop: true,
    defaultWidth: 960,
    defaultHeight: 620,
  },
  {
    id: 'video-player',
    title: 'Video Player',
    subtitle: 'MP4 스트리밍 · 자막 · 이어보기',
    iconBg: 'from-rose-500 to-orange-500',
    requiresPrivate: false,
    showOnDesktop: false,
    defaultWidth: 900,
    defaultHeight: 560,
  },
  {
    id: 'code-editor',
    title: 'Code Editor',
    subtitle: '코드 및 설정 편집기',
    iconBg: 'from-emerald-500 to-teal-600',
    requiresPrivate: false,
    showOnDesktop: false,
    defaultWidth: 880,
    defaultHeight: 600,
  },
  {
    id: 'mounts',
    title: '마운트 관리',
    subtitle: '서버 로컬 폴더 연결 · 권한 설정',
    iconBg: 'from-cyan-500 to-teal-600',
    requiresPrivate: true,
    showOnDesktop: true,
    defaultWidth: 820,
    defaultHeight: 560,
  },
  {
    id: 'wargame',
    title: 'Wargame Runner',
    subtitle: 'Docker 워게임 자동화 (Tailscale)',
    iconBg: 'from-violet-500 to-purple-700',
    requiresPrivate: true,
    showOnDesktop: true,
    defaultWidth: 860,
    defaultHeight: 560,
  },
  {
    id: 'terminal',
    title: 'Web Terminal',
    subtitle: '우분투 호스트 & 컨테이너 쉘',
    iconBg: 'from-slate-700 to-slate-900',
    requiresPrivate: true,
    showOnDesktop: true,
    defaultWidth: 820,
    defaultHeight: 520,
  },
  {
    id: 'monitor',
    title: 'Resource Monitor',
    subtitle: '시스템 리소스 및 디스크 상태',
    iconBg: 'from-amber-500 to-orange-600',
    requiresPrivate: false,
    showOnDesktop: true,
    defaultWidth: 740,
    defaultHeight: 500,
  },
] as const satisfies readonly DesktopAppDefinition[]

export interface WindowInstance {
  id: string
  appId: AppId
  title: string
  payload: WindowPayload
  x: number
  y: number
  width: number
  height: number
  minimized: boolean
  maximized: boolean
  zIndex: number
}

export const useDesktopStore = defineStore('desktop', () => {
  const windows = ref<WindowInstance[]>([])
  const nextZIndex = ref(20)
  const counter = ref(1)

  const activeWindowId = computed<string | null>(() => {
    let top: WindowInstance | null = null
    for (const w of windows.value) {
      if (w.minimized) continue
      if (!top || w.zIndex > top.zIndex) {
        top = w
      }
    }
    return top ? top.id : null
  })

  function findAppDef(appId: AppId): DesktopAppDefinition {
    const found = DESKTOP_APPS.find((a) => a.id === appId)
    if (!found) {
      return DESKTOP_APPS[0]
    }
    return found
  }

  function focusWindow(windowId: string): void {
    const win = windows.value.find((w) => w.id === windowId)
    if (!win) return
    win.minimized = false
    nextZIndex.value += 1
    win.zIndex = nextZIndex.value
  }

  function openWindow(params: { readonly title?: string; readonly payload: WindowPayload }): string {
    const appId = params.payload.kind
    const def = findAppDef(appId)

    // Reuse singleton window for apps like monitor/wargame/terminal or matching file path
    const existing = windows.value.find((w) => {
      if (w.payload.kind !== params.payload.kind) return false
      switch (params.payload.kind) {
        case 'mounts':
        case 'wargame':
        case 'terminal':
        case 'monitor':
          return true
        case 'video-player':
          return (
            w.payload.kind === 'video-player' &&
            w.payload.rootId === params.payload.rootId &&
            w.payload.relPath === params.payload.relPath
          )
        case 'code-editor':
          return (
            w.payload.kind === 'code-editor' &&
            w.payload.rootId === params.payload.rootId &&
            w.payload.relPath === params.payload.relPath
          )
        case 'file-station':
          return false
        default: {
          const _exhaustive: never = params.payload
          return _exhaustive
        }
      }
    })

    if (existing) {
      focusWindow(existing.id)
      return existing.id
    }

    const offset = ((windows.value.length % 6) * 28)
    const maxW = Math.max(360, window.innerWidth - 40)
    const maxH = Math.max(320, window.innerHeight - 88)
    const width = Math.min(def.defaultWidth, maxW)
    const height = Math.min(def.defaultHeight, maxH)
    const x = Math.max(16, Math.min(96 + offset, window.innerWidth - width - 16))
    const y = Math.max(52, Math.min(64 + offset, window.innerHeight - height - 24))

    nextZIndex.value += 1
    const id = `win-${counter.value++}`
    windows.value.push({
      id,
      appId,
      title: params.title ?? def.title,
      payload: params.payload,
      x,
      y,
      width,
      height,
      minimized: false,
      maximized: false,
      zIndex: nextZIndex.value,
    })
    return id
  }

  function closeWindow(windowId: string): void {
    windows.value = windows.value.filter((w) => w.id !== windowId)
  }

  function toggleMinimize(windowId: string): void {
    const win = windows.value.find((w) => w.id === windowId)
    if (!win) return
    if (win.minimized) {
      focusWindow(windowId)
    } else if (activeWindowId.value === windowId) {
      win.minimized = true
    } else {
      focusWindow(windowId)
    }
  }

  function minimizeWindow(windowId: string): void {
    const win = windows.value.find((w) => w.id === windowId)
    if (win) {
      win.minimized = true
    }
  }

  function toggleMaximize(windowId: string): void {
    const win = windows.value.find((w) => w.id === windowId)
    if (!win) return
    win.maximized = !win.maximized
    focusWindow(windowId)
  }

  function updateBounds(params: {
    readonly windowId: string
    readonly x: number
    readonly y: number
    readonly width: number
    readonly height: number
  }): void {
    const win = windows.value.find((w) => w.id === params.windowId)
    if (!win || win.maximized) return
    win.x = params.x
    win.y = params.y
    win.width = params.width
    win.height = params.height
  }

  function setWindowTitle(params: { readonly windowId: string; readonly title: string }): void {
    const win = windows.value.find((w) => w.id === params.windowId)
    if (win) {
      win.title = params.title
    }
  }

  function resetAll(): void {
    windows.value = []
  }

  return {
    windows,
    activeWindowId,
    openWindow,
    focusWindow,
    closeWindow,
    toggleMinimize,
    minimizeWindow,
    toggleMaximize,
    updateBounds,
    setWindowTitle,
    resetAll,
  }
})
