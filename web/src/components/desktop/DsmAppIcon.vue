<script setup lang="ts">
import type { AppId } from '../../stores/desktop'

withDefaults(
  defineProps<{
    readonly appId: AppId
    readonly size?: 'xs' | 'sm' | 'md' | 'lg'
  }>(),
  {
    size: 'md',
  },
)

function sizeClass(size: 'xs' | 'sm' | 'md' | 'lg'): string {
  switch (size) {
    case 'xs':
      return 'h-4 w-4'
    case 'sm':
      return 'h-5 w-5'
    case 'md':
      return 'h-10 w-10'
    case 'lg':
      return 'h-14 w-14'
    default: {
      const _exhaustive: never = size
      return _exhaustive
    }
  }
}
</script>

<template>
  <div class=" inline-flex shrink-0 select-none items-center justify-center" :class="sizeClass(size)">
    <!-- File Station: Synology golden-amber folder + crisp white/blue inner sheet -->
    <svg v-if="appId === 'file-station'" viewBox="0 0 64 64" class="h-full w-full drop-shadow-sm">
      <defs>
        <linearGradient id="fs-bg" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stop-color="#FFE082" />
          <stop offset="100%" stop-color="#F59E0B" />
        </linearGradient>
        <linearGradient id="fs-front" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stop-color="#FCD34D" />
          <stop offset="100%" stop-color="#D97706" />
        </linearGradient>
      </defs>
      <rect x="4" y="4" width="56" height="56" rx="14" fill="#F8FAFC" stroke="#CBD5E1" stroke-width="1" />
      <path d="M13 20a4 4 0 0 1 4-4h10l4 4h16a4 4 0 0 1 4 4v22a4 4 0 0 1-4 4H17a4 4 0 0 1-4-4V20z" fill="url(#fs-bg)" />
      <rect x="17" y="21" width="30" height="14" rx="2.5" fill="#FFFFFF" />
      <rect x="21" y="25" width="14" height="2.2" rx="1" fill="#0572EC" />
      <rect x="21" y="29.5" width="20" height="2" rx="1" fill="#94A3B8" />
      <path d="M13 28.5A3.5 3.5 0 0 1 16.5 25h31A3.5 3.5 0 0 1 51 28.5V46a4 4 0 0 1-4 4H17a4 4 0 0 1-4-4V28.5z" fill="url(#fs-front)" />
    </svg>

    <!-- Control Panel / Shared Folder Mounts: DSM Control Panel silver-blue sliders & storage bay -->
    <svg v-else-if="appId === 'mounts'" viewBox="0 0 64 64" class="h-full w-full drop-shadow-sm">
      <defs>
        <linearGradient id="cp-bg" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stop-color="#F1F5F9" />
          <stop offset="100%" stop-color="#CBD5E1" />
        </linearGradient>
      </defs>
      <rect x="4" y="4" width="56" height="56" rx="14" fill="url(#cp-bg)" stroke="#94A3B8" stroke-width="1" />
      <rect x="14" y="16" width="36" height="12" rx="3" fill="#334155" />
      <circle cx="20" cy="22" r="2.2" fill="#10B981" />
      <rect x="26" y="20.8" width="18" height="2.4" rx="1.2" fill="#94A3B8" />
      <rect x="14" y="32" width="36" height="16" rx="3" fill="#1E293B" />
      <circle cx="20" cy="40" r="2.2" fill="#0572EC" />
      <rect x="26" y="38.8" width="18" height="2.4" rx="1.2" fill="#64748B" />
      <circle cx="45" cy="45" r="9" fill="#0572EC" />
      <path d="M45 40.5v9M40.5 45h9" stroke="#FFFFFF" stroke-width="2.2" stroke-linecap="round" />
    </svg>

    <!-- Wargame / Container Manager: Synology Container Manager stacked isometric blue blocks -->
    <svg v-else-if="appId === 'wargame'" viewBox="0 0 64 64" class="h-full w-full drop-shadow-sm">
      <defs>
        <linearGradient id="wg-bg" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0%" stop-color="#1E88E5" />
          <stop offset="100%" stop-color="#0052CC" />
        </linearGradient>
      </defs>
      <rect x="4" y="4" width="56" height="56" rx="14" fill="url(#wg-bg)" />
      <g fill="#FFFFFF">
        <rect x="15" y="33" width="9.5" height="9.5" rx="1.8" opacity="0.95" />
        <rect x="27" y="33" width="9.5" height="9.5" rx="1.8" opacity="0.95" />
        <rect x="39" y="33" width="9.5" height="9.5" rx="1.8" opacity="0.95" />
        <rect x="21" y="21.5" width="9.5" height="9.5" rx="1.8" opacity="0.88" />
        <rect x="33" y="21.5" width="9.5" height="9.5" rx="1.8" opacity="0.88" />
      </g>
      <path d="M13 46.5h38" stroke="#93C5FD" stroke-width="2.5" stroke-linecap="round" />
    </svg>

    <!-- Resource Monitor: Synology teal telemetry dial & pulse -->
    <svg v-else-if="appId === 'monitor'" viewBox="0 0 64 64" class="h-full w-full drop-shadow-sm">
      <defs>
        <linearGradient id="rm-bg" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stop-color="#14B8A6" />
          <stop offset="100%" stop-color="#0F766E" />
        </linearGradient>
      </defs>
      <rect x="4" y="4" width="56" height="56" rx="14" fill="url(#rm-bg)" />
      <circle cx="32" cy="34" r="16" fill="none" stroke="rgba(255,255,255,0.28)" stroke-width="4" />
      <path d="M18.5 42A16 16 0 1 1 46 26" fill="none" stroke="#FFFFFF" stroke-width="4" stroke-linecap="round" />
      <path d="M32 34L41 25" stroke="#FEF08A" stroke-width="3" stroke-linecap="round" />
      <circle cx="32" cy="34" r="3.2" fill="#FFFFFF" />
    </svg>

    <!-- Web Terminal: Crisp dark console squircle -->
    <svg v-else-if="appId === 'terminal'" viewBox="0 0 64 64" class="h-full w-full drop-shadow-sm">
      <defs>
        <linearGradient id="tm-bg" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stop-color="#334155" />
          <stop offset="100%" stop-color="#0F172A" />
        </linearGradient>
      </defs>
      <rect x="4" y="4" width="56" height="56" rx="14" fill="url(#tm-bg)" stroke="#475569" stroke-width="1" />
      <circle cx="14" cy="14" r="2" fill="#EF4444" />
      <circle cx="20.5" cy="14" r="2" fill="#F59E0B" />
      <circle cx="27" cy="14" r="2" fill="#10B981" />
      <path d="M17 27l9 7-9 7" fill="none" stroke="#38BDF8" stroke-width="3.2" stroke-linecap="round" stroke-linejoin="round" />
      <path d="M30 41h14" stroke="#E2E8F0" stroke-width="3.2" stroke-linecap="round" />
    </svg>

    <!-- Video Station / Media Player -->
    <svg v-else-if="appId === 'video-player'" viewBox="0 0 64 64" class="h-full w-full drop-shadow-sm">
      <defs>
        <linearGradient id="vp-bg" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0%" stop-color="#FB7185" />
          <stop offset="100%" stop-color="#E11D48" />
        </linearGradient>
      </defs>
      <rect x="4" y="4" width="56" height="56" rx="14" fill="url(#vp-bg)" />
      <circle cx="32" cy="32" r="16" fill="rgba(255,255,255,0.2)" />
      <path d="M27.5 23.5L42 32l-14.5 8.5V23.5z" fill="#FFFFFF" />
    </svg>

    <!-- Text / Code Editor: Synology Text Editor sheet + blue header -->
    <svg v-else viewBox="0 0 64 64" class="h-full w-full drop-shadow-sm">
      <rect x="4" y="4" width="56" height="56" rx="14" fill="#FFFFFF" stroke="#CBD5E1" stroke-width="1" />
      <path d="M4 18A14 14 0 0 1 18 4h28a14 14 0 0 1 14 14v2H4v-2z" fill="#0572EC" />
      <rect x="15" y="27" width="24" height="3" rx="1.5" fill="#334155" />
      <rect x="15" y="35" width="34" height="3" rx="1.5" fill="#94A3B8" />
      <rect x="15" y="43" width="28" height="3" rx="1.5" fill="#94A3B8" />
    </svg>
  </div>
</template>
