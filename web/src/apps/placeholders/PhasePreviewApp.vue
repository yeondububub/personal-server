<script setup lang="ts">
import { CheckCircle2 } from 'lucide-vue-next'
import DsmAppIcon from '../../components/desktop/DsmAppIcon.vue'
</script>

<template>
  <div class="flex h-full w-full flex-col items-center justify-center bg-[#f4f6f9] p-6 text-center text-[#334155]">
    <div class="flex w-full max-w-md flex-col items-center rounded-[6px] border border-[#dce2ea] bg-white p-6 shadow-2xs">
      <DsmAppIcon app-id="terminal" size="lg" class="mb-3.5" />

      <span
        class="mb-3 inline-flex items-center gap-1.5 rounded-[4px] border border-[#a7f3d0] bg-[#ecfdf5] px-2.5 py-1 text-[11px] font-semibold text-[#047857]"
      >
        <CheckCircle2 class="h-3.5 w-3.5" />
        Tailscale Private 리스너 연결 확인됨
      </span>

      <h3 class="mb-1.5 text-[14px] font-semibold text-[#1e293b]">
        Web Terminal (Phase 3 구현 예정)
      </h3>

      <p class="text-[12px] leading-relaxed text-[#475569]">
        xterm.js와 Go PTY를 연결해 우분투 호스트 쉘 및 실행 중인 워게임 컨테이너(docker exec)에
        브라우저에서 바로 접속하는 터미널 모듈입니다.
      </p>
    </div>
  </div>
</template>
