import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { api } from '../api/client'
import type { AppSettings, ClockFormat, WallpaperMode, WallpaperPreset } from '../types/api'

const DEFAULT_FAVICON_SVG =
  'data:image/svg+xml,' +
  encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64"><defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop offset="0%" stop-color="#0ea5e9"/><stop offset="100%" stop-color="#0572ec"/></linearGradient></defs><rect x="4" y="4" width="56" height="56" rx="14" fill="url(#g)"/><rect x="16" y="20" width="32" height="9" rx="3" fill="#fff" opacity="0.95"/><rect x="16" y="35" width="32" height="9" rx="3" fill="#fff" opacity="0.82"/><circle cx="22" cy="24.5" r="2" fill="#0572ec"/><circle cx="22" cy="39.5" r="2" fill="#0572ec"/></svg>',
  )

const DEFAULT_SETTINGS: AppSettings = {
  siteTitle: 'Personal Server',
  clockFormat: '24h',
  wallpaperMode: 'preset',
  wallpaperPreset: 'ocean',
  hasCustomWallpaper: false,
  hasCustomFavicon: false,
  updatedAt: 0,
}

function applyDocumentBranding(cfg: AppSettings): void {
  document.title = cfg.siteTitle || 'Personal Server'

  let link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
  if (!link) {
    link = document.createElement('link')
    link.rel = 'icon'
    document.head.appendChild(link)
  }
  link.href = cfg.hasCustomFavicon
    ? api.customFaviconUrl(cfg.updatedAt)
    : DEFAULT_FAVICON_SVG
}

export const useSettingsStore = defineStore('settings', () => {
  const data = ref<AppSettings>(DEFAULT_SETTINGS)
  const loaded = ref(false)
  const assetVersion = ref(0)

  const siteTitle = computed(() => data.value.siteTitle)
  const clockFormat = computed<ClockFormat>(() => data.value.clockFormat)
  const wallpaperMode = computed<WallpaperMode>(() => data.value.wallpaperMode)
  const wallpaperPreset = computed<WallpaperPreset>(() => data.value.wallpaperPreset)
  const hasCustomWallpaper = computed(() => data.value.hasCustomWallpaper)
  const hasCustomFavicon = computed(() => data.value.hasCustomFavicon)

  const customWallpaperUrl = computed<string | null>(() => {
    if (!data.value.hasCustomWallpaper) return null
    return api.customWallpaperUrl(assetVersion.value || data.value.updatedAt)
  })

  const customFaviconUrl = computed<string | null>(() => {
    if (!data.value.hasCustomFavicon) return null
    return api.customFaviconUrl(assetVersion.value || data.value.updatedAt)
  })

  function applySettings(next: AppSettings, bumpAsset = false): void {
    data.value = next
    if (bumpAsset) {
      assetVersion.value = Date.now()
    } else if (next.updatedAt > 0) {
      assetVersion.value = next.updatedAt
    }
    applyDocumentBranding({
      ...next,
      updatedAt: assetVersion.value || next.updatedAt,
    })
  }

  async function loadPublic(): Promise<void> {
    try {
      const next = await api.fetchPublicSettings()
      applySettings(next)
    } catch {
      applyDocumentBranding(data.value)
    } finally {
      loaded.value = true
    }
  }

  async function save(params: {
    readonly siteTitle?: string
    readonly clockFormat?: ClockFormat
    readonly wallpaperMode?: WallpaperMode
    readonly wallpaperPreset?: WallpaperPreset
  }): Promise<AppSettings> {
    const next = await api.updateSettings(params)
    applySettings(next)
    return next
  }

  async function uploadFavicon(file: File): Promise<AppSettings> {
    const next = await api.uploadFavicon(file)
    applySettings(next, true)
    return next
  }

  async function resetFavicon(): Promise<AppSettings> {
    const next = await api.deleteFavicon()
    applySettings(next, true)
    return next
  }

  async function uploadWallpaper(file: File): Promise<AppSettings> {
    const next = await api.uploadWallpaper(file)
    applySettings(next, true)
    return next
  }

  async function resetWallpaper(): Promise<AppSettings> {
    const next = await api.deleteWallpaper()
    applySettings(next, true)
    return next
  }

  return {
    data,
    loaded,
    siteTitle,
    clockFormat,
    wallpaperMode,
    wallpaperPreset,
    hasCustomWallpaper,
    hasCustomFavicon,
    customWallpaperUrl,
    customFaviconUrl,
    loadPublic,
    save,
    uploadFavicon,
    resetFavicon,
    uploadWallpaper,
    resetWallpaper,
  }
})
