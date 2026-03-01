/**
 * Theme composable for multi-theme color system.
 * Manages 6 theme palettes via data-theme attribute on <html>,
 * persisted in localStorage under 'capacitarr-theme'.
 */

export type ThemeId = 'violet' | 'ocean' | 'emerald' | 'sunset' | 'rose' | 'slate'

export interface ThemeMeta {
  id: ThemeId
  label: string
  hue: number
  type: 'analogous' | 'complementary' | 'monochrome'
}

/** All available themes with display metadata */
export const THEMES: ThemeMeta[] = [
  { id: 'violet', label: 'Violet', hue: 293, type: 'analogous' },
  { id: 'ocean', label: 'Ocean', hue: 230, type: 'analogous' },
  { id: 'emerald', label: 'Emerald', hue: 160, type: 'analogous' },
  { id: 'sunset', label: 'Sunset', hue: 55, type: 'complementary' },
  { id: 'rose', label: 'Rose', hue: 350, type: 'complementary' },
  { id: 'slate', label: 'Slate', hue: 260, type: 'monochrome' }
]

const STORAGE_KEY = 'capacitarr-theme'
const DEFAULT_THEME: ThemeId = 'violet'
const VALID_THEMES = new Set<string>(THEMES.map(t => t.id))

export const useTheme = () => {
  const theme = useState<ThemeId>('appTheme', () => {
    if (import.meta.client) {
      const stored = localStorage.getItem(STORAGE_KEY)
      if (stored && VALID_THEMES.has(stored)) return stored as ThemeId
    }
    return DEFAULT_THEME
  })

  function setTheme(id: ThemeId) {
    theme.value = id
    if (import.meta.client) {
      document.documentElement.setAttribute('data-theme', id)
      localStorage.setItem(STORAGE_KEY, id)
    }
  }

  // Apply on first client load
  if (import.meta.client) {
    document.documentElement.setAttribute('data-theme', theme.value)
  }

  return { theme: readonly(theme), setTheme, themes: THEMES }
}
