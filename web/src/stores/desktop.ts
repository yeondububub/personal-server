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

export interface WargameLaunchSource {
  readonly rootId: string
  readonly relPath: string
  readonly name: string
  readonly nonce: number
}

export interface TerminalLaunchTarget {
  readonly kind: 'host' | 'docker'
  readonly container?: string
  readonly label?: string
  readonly nonce: number
}

export type WindowPayload =
  | { readonly kind: 'file-station'; readonly initialRootId?: string; readonly initialPath?: string }
  | { readonly kind: 'video-player'; readonly rootId: string; readonly relPath: string; readonly fileName: string }
  | { readonly kind: 'code-editor'; readonly rootId: string; readonly relPath: string; readonly fileName: string }
  | { readonly kind: 'mounts' }
  | { readonly kind: 'wargame'; readonly launchSource?: WargameLaunchSource }
  | { readonly kind: 'terminal'; readonly initialTarget?: TerminalLaunchTarget }
  | { readonly kind: 'monitor' }

export interface DesktopAppDefinition {
  readonly id: AppId
  readonly title: string
  readonly subtitle: string
  readonly requiresPrivate: boolean
  readonly showOnDesktop: boolean
  readonly defaultWidth: number
  readonly defaultHeight: number
}

const FILE_STATION_APP: DesktopAppDefinition = {
  id: 'file-station',
  title: 'File Station',
  subtitle: '공유 폴더 및 파일 관리',
  requiresPrivate: false,
  showOnDesktop: true,
  defaultWidth: 960,
  defaultHeight: 620,
}

export const DESKTOP_APPS: readonly DesktopAppDefinition[] = [
  FILE_STATION_APP,
  {
    id: 'video-player',
    title: 'Media Player',
    subtitle: '영상 스트리밍 플레이어',
    requiresPrivate: false,
    showOnDesktop: false,
    defaultWidth: 900,
    defaultHeight: 560,
  },
  {
    id: 'code-editor',
    title: '텍스트 편집기',
    subtitle: '코드 및 설정 파일 편집',
    requiresPrivate: false,
    showOnDesktop: false,
    defaultWidth: 880,
    defaultHeight: 600,
  },
  {
    id: 'mounts',
    title: '마운트 관리',
    subtitle: '서버 로컬 폴더 마운트 및 권한',
    requiresPrivate: true,
    showOnDesktop: true,
    defaultWidth: 820,
    defaultHeight: 560,
  },
  {
    id: 'wargame',
    title: 'Wargame Runner',
    subtitle: 'Docker 워게임 자동 실행기',
    requiresPrivate: true,
    showOnDesktop: true,
    defaultWidth: 860,
    defaultHeight: 560,
  },
  {
    id: 'terminal',
    title: '터미널',
    subtitle: '호스트 및 컨테이너 쉘',
    requiresPrivate: true,
    showOnDesktop: true,
    defaultWidth: 820,
    defaultHeight: 520,
  },
  {
    id: 'monitor',
    title: '리소스 모니터',
    subtitle: '실시간 CPU, 메모리, 네트워크 및 볼륨',
    requiresPrivate: false,
    showOnDesktop: true,
    defaultWidth: 840,
    defaultHeight: 560,
  },
]

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
    return found ?? FILE_STATION_APP
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
      if (params.payload.kind === 'wargame' && params.payload.launchSource) {
        existing.payload = params.payload
      } else if (params.payload.kind === 'terminal' && params.payload.initialTarget) {
        existing.payload = params.payload
      }
      focusWindow(existing.id)
      return existing.id
    }

    const offset = (windows.value.length % 6) * 28
    const maxW = Math.max(360, window.innerWidth - 40)
    const maxH = Math.max(320, window.innerHeight - 76)
    const width = Math.min(def.defaultWidth, maxW)
    const height = Math.min(def.defaultHeight, maxH)
    const x = Math.max(16, Math.min(96 + offset, window.innerWidth - width - 16))
    const y = Math.max(44, Math.min(56 + offset, window.innerHeight - height - 20))

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
