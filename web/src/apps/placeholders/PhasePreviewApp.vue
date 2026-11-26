<script setup lang="ts">
import { Cpu, Terminal, CheckCircle2 } from 'lucide-vue-next'

defineProps<{
  readonly mode: 'wargame' | 'terminal'
}>()
</script>

<template>
  <div class="flex h-full w-full flex-col items-center justify-center p-8 text-center text-slate-200">
    <div
      class="mb-4 flex h-14 w-14 items-center justify-center rounded-2xl bg-gradient-to-br text-white shadow-lg"
      :class="mode === 'wargame' ? 'from-violet-500 to-purple-700' : 'from-slate-700 to-slate-900'"
    >
      <Cpu v-if="mode === 'wargame'" class="h-7 w-7" />
      <Terminal v-else class="h-7 w-7" />
    </div>

    <span
      class="mb-3 inline-flex items-center gap-1.5 rounded-full border border-emerald-400/30 bg-emerald-500/15 px-3 py-1 text-[11px] font-medium text-emerald-300"
    >
      <CheckCircle2 class="h-3.5 w-3.5" />
      Tailscale Private 리스너 연결 확인됨
    </span>

    <h3 class="mb-2 text-base font-semibold text-white">
      {{ mode === 'wargame' ? 'Wargame Runner (Phase 2 구현 예정)' : 'Web Terminal (Phase 3 구현 예정)' }}
    </h3>

    <p v-if="mode === 'wargame'" class="max-w-md text-xs leading-relaxed text-slate-400">
      문제 압축 파일(.zip, .tar.gz) 업로드 시 docker-compose, Dockerfile, 단독 ELF/웹 소스를 자동 감지하여
      격리된 컨테이너와 Tailscale 전용 포트를 자동 할당하는 모듈입니다.
    </p>
    <p v-else class="max-w-md text-xs leading-relaxed text-slate-400">
      xterm.js와 Go PTY를 연결해 우분투 호스트 쉘 및 실행 중인 워게임 컨테이너(docker exec)에 브라우저에서
      바로 접속하는 터미널 모듈입니다.
    </p>
  </div>
</template>
