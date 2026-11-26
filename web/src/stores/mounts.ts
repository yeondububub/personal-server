import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useMountsStore = defineStore('mounts', () => {
  const version = ref(0)

  function notifyChanged(): void {
    version.value += 1
  }

  return {
    version,
    notifyChanged,
  }
})
