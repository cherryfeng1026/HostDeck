import { computed, reactive } from 'vue'
import type { ThemeMode } from './types'

const THEME_STORAGE_KEY = 'hostdeck.theme.mode'

const state = reactive({
  mode: detectInitialThemeMode(),
})

function detectInitialThemeMode(): ThemeMode {
  if (typeof window === 'undefined') {
    return 'dark'
  }
  const storedMode = window.localStorage.getItem(THEME_STORAGE_KEY)
  return storedMode === 'light' || storedMode === 'dark' ? storedMode : 'dark'
}

function syncThemeMode(mode: ThemeMode) {
  if (typeof document !== 'undefined') {
    document.documentElement.dataset.theme = mode
    document.documentElement.style.colorScheme = mode
  }
  if (typeof window !== 'undefined') {
    window.localStorage.setItem(THEME_STORAGE_KEY, mode)
  }
}

function logThemeEvent(event: string, details: Record<string, unknown> = {}) {
  console.info('[HostDeck][Theme]', {
    event,
    mode: state.mode,
    timestamp: new Date().toISOString(),
    ...details,
  })
}

syncThemeMode(state.mode)

export const currentThemeMode = computed(() => state.mode)

export function setThemeMode(mode: ThemeMode) {
  if (state.mode === mode) {
    syncThemeMode(mode)
    return
  }
  state.mode = mode
  syncThemeMode(mode)
  logThemeEvent('mode-changed')
}

export function toggleThemeMode() {
  setThemeMode(state.mode === 'dark' ? 'light' : 'dark')
}

export function useThemeState() {
  return {
    state,
    currentThemeMode,
    setThemeMode,
    toggleThemeMode,
  }
}
