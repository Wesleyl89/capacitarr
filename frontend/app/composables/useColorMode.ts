/**
 * Simple color mode composable (dark/light toggle).
 * Persists preference in localStorage and applies 'dark' class to <html>.
 */
export const useAppColorMode = () => {
  const mode = useState<'light' | 'dark'>('colorMode', () => {
    if (import.meta.client) {
      const stored = localStorage.getItem('capacitarr-color-mode')
      if (stored === 'dark' || stored === 'light') return stored
      return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
    }
    return 'dark' // Default for SSR/initial
  })

  const isDark = computed(() => mode.value === 'dark')

  function toggle() {
    mode.value = mode.value === 'dark' ? 'light' : 'dark'
    apply()
  }

  function apply() {
    if (!import.meta.client) return
    document.documentElement.classList.toggle('dark', mode.value === 'dark')
    localStorage.setItem('capacitarr-color-mode', mode.value)
  }

  // Apply on first client-side load
  if (import.meta.client) {
    apply()
  }

  return { mode, isDark, toggle }
}
