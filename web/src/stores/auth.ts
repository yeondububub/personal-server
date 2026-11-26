import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { api } from '../api/client'
import type { AccessTier, AuthStatus } from '../types/api'

export const useAuthStore = defineStore('auth', () => {
  const state = ref<AuthStatus>({ kind: 'loading' })

  const isAuthenticated = computed(() => state.value.kind === 'authenticated')
  const accessTier = computed<AccessTier>(() =>
    state.value.kind === 'loading' ? 'public' : state.value.accessTier,
  )
  const username = computed(() =>
    state.value.kind === 'authenticated' ? state.value.user.username : '',
  )

  async function refreshSession(): Promise<void> {
    try {
      state.value = await api.fetchMe()
    } catch {
      state.value = { kind: 'unauthenticated', accessTier: 'public' }
    }
  }

  async function login(params: { readonly username: string; readonly password: string }): Promise<void> {
    state.value = await api.login(params)
  }

  async function logout(): Promise<void> {
    await api.logout()
    const currentTier = accessTier.value
    state.value = { kind: 'unauthenticated', accessTier: currentTier }
  }

  return {
    state,
    isAuthenticated,
    accessTier,
    username,
    refreshSession,
    login,
    logout,
  }
})
