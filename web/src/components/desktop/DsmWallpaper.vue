<script setup lang="ts">
export type DsmWallpaperTheme = 'ocean' | 'mist' | 'slate'

withDefaults(
  defineProps<{
    readonly theme?: DsmWallpaperTheme
    readonly customUrl?: string | null
  }>(),
  {
    theme: 'ocean',
    customUrl: null,
  },
)
</script>

<template>
  <div class="pointer-events-none absolute inset-0 overflow-hidden select-none">
    <template v-if="customUrl">
      <img :src="customUrl" alt="" class="h-full w-full object-cover" />
      <div class="absolute inset-0 bg-black/15" />
    </template>

    <!-- Theme 1: Iconic Synology DSM 7.2 Ocean Silk Wave -->
    <svg
      v-else-if="theme === 'ocean'"
      viewBox="0 0 1600 900"
      preserveAspectRatio="xMidYMid slice"
      class="h-full w-full"
    >
      <defs>
        <linearGradient id="dsm-ocean-base" x1="0%" y1="0%" x2="100%" y2="100%">
          <stop offset="0%" stop-color="#0B3B6E" />
          <stop offset="48%" stop-color="#115E9C" />
          <stop offset="100%" stop-color="#1982A8" />
        </linearGradient>
        <linearGradient id="dsm-wave-1" x1="0%" y1="0%" x2="100%" y2="80%">
          <stop offset="0%" stop-color="#2596BE" stop-opacity="0.55" />
          <stop offset="100%" stop-color="#0D4B82" stop-opacity="0.1" />
        </linearGradient>
        <linearGradient id="dsm-wave-2" x1="15%" y1="0%" x2="85%" y2="100%">
          <stop offset="0%" stop-color="#5CC3E2" stop-opacity="0.38" />
          <stop offset="100%" stop-color="#1565A6" stop-opacity="0.22" />
        </linearGradient>
        <linearGradient id="dsm-wave-3" x1="0%" y1="100%" x2="100%" y2="0%">
          <stop offset="0%" stop-color="#07284F" stop-opacity="0.65" />
          <stop offset="100%" stop-color="#1D79B2" stop-opacity="0.18" />
        </linearGradient>
      </defs>
      <rect width="1600" height="900" fill="url(#dsm-ocean-base)" />
      <path d="M0,180 C380,60 760,360 1600,110 L1600,900 L0,900 Z" fill="url(#dsm-wave-1)" />
      <path d="M0,430 C460,260 980,590 1600,320 L1600,900 L0,900 Z" fill="url(#dsm-wave-2)" />
      <path d="M0,620 C520,470 1080,760 1600,520 L1600,900 L0,900 Z" fill="url(#dsm-wave-3)" />
    </svg>

    <!-- Theme 2: DSM Nordic Mist (Teal-Slate Daylight) -->
    <svg
      v-else-if="theme === 'mist'"
      viewBox="0 0 1600 900"
      preserveAspectRatio="xMidYMid slice"
      class="h-full w-full"
    >
      <defs>
        <linearGradient id="dsm-mist-base" x1="0%" y1="0%" x2="100%" y2="100%">
          <stop offset="0%" stop-color="#1E4E68" />
          <stop offset="55%" stop-color="#2D6F84" />
          <stop offset="100%" stop-color="#4A8F98" />
        </linearGradient>
        <linearGradient id="dsm-mist-w1" x1="0%" y1="0%" x2="100%" y2="100%">
          <stop offset="0%" stop-color="#89C4C8" stop-opacity="0.34" />
          <stop offset="100%" stop-color="#1A425A" stop-opacity="0.15" />
        </linearGradient>
      </defs>
      <rect width="1600" height="900" fill="url(#dsm-mist-base)" />
      <path d="M0,260 C520,120 960,440 1600,210 L1600,900 L0,900 Z" fill="url(#dsm-mist-w1)" />
      <path d="M0,560 C600,410 1050,690 1600,480 L1600,900 L0,900 Z" fill="rgba(14, 38, 54, 0.38)" />
    </svg>

    <!-- Theme 3: DSM Deep Graphite -->
    <svg
      v-else
      viewBox="0 0 1600 900"
      preserveAspectRatio="xMidYMid slice"
      class="h-full w-full"
    >
      <defs>
        <linearGradient id="dsm-slate-base" x1="0%" y1="0%" x2="100%" y2="100%">
          <stop offset="0%" stop-color="#162231" />
          <stop offset="55%" stop-color="#1F3349" />
          <stop offset="100%" stop-color="#27445E" />
        </linearGradient>
      </defs>
      <rect width="1600" height="900" fill="url(#dsm-slate-base)" />
      <path d="M0,330 C480,190 980,490 1600,270 L1600,900 L0,900 Z" fill="rgba(56, 128, 196, 0.14)" />
      <path d="M0,610 C540,480 1100,720 1600,540 L1600,900 L0,900 Z" fill="rgba(10, 18, 28, 0.48)" />
    </svg>
  </div>
</template>
