import { onMounted, onUnmounted, unref } from 'vue'
import type { Ref } from 'vue'

type RefreshCallback = () => void | Promise<void>

export function useAutoRefresh(callback: RefreshCallback, intervalMs: number | Ref<number>) {
  let timer: number | null = null
  let running = false

  async function run() {
    if (running || isDocumentHidden()) {
      return
    }
    running = true
    try {
      await callback()
    } finally {
      running = false
    }
  }

  function stop() {
    if (timer !== null) {
      window.clearInterval(timer)
      timer = null
    }
  }

  function start() {
    stop()
    const interval = unref(intervalMs)
    if (interval <= 0 || isDocumentHidden()) {
      return
    }
    timer = window.setInterval(() => {
      void run()
    }, interval)
  }

  function handleVisibilityChange() {
    if (isDocumentHidden()) {
      stop()
      return
    }
    void run()
    start()
  }

  onMounted(() => {
    if (typeof document === 'undefined') {
      return
    }
    document.addEventListener('visibilitychange', handleVisibilityChange)
    start()
  })

  onUnmounted(() => {
    if (typeof document !== 'undefined') {
      document.removeEventListener('visibilitychange', handleVisibilityChange)
    }
    stop()
  })

  return { start, stop, refreshNow: run }
}

function isDocumentHidden() {
  return typeof document !== 'undefined' && document.hidden
}
