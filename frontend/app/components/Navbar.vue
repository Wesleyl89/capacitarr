<template>
  <header
    data-slot="navbar"
    class="sticky top-0 z-50 relative"
  >
    <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
      <div class="flex items-center justify-between h-16">
        <!-- Brand -->
        <div class="flex items-center gap-6">
          <NuxtLink to="/" class="flex items-center gap-2.5 group">
            <div data-slot="brand-icon" class="w-8 h-8 rounded-lg bg-primary flex items-center justify-center">
              <component :is="DatabaseIcon" class="w-4.5 h-4.5 text-primary-foreground" />
            </div>
            <span class="text-lg font-bold tracking-tight text-foreground">
              Capacitarr
            </span>
          </NuxtLink>

          <!-- Nav Links -->
          <nav class="hidden sm:flex items-center gap-1">
            <NuxtLink
              v-for="link in navLinks"
              :key="link.to"
              :to="link.to"
              class="px-3 py-1.5 rounded-lg text-sm font-medium transition-all duration-200"
              :class="[
                $route.path === link.to
                  ? 'text-primary bg-primary/10'
                  : 'text-muted-foreground hover:text-foreground hover:bg-accent'
              ]"
              :data-slot="$route.path === link.to ? 'nav-link-active' : undefined"
            >
              {{ link.label }}
            </NuxtLink>
          </nav>
        </div>

        <!-- Right side -->
        <div class="flex items-center gap-1">
          <!-- Theme selector -->
          <UiDropdownMenu>
            <UiDropdownMenuTrigger as-child>
              <UiButton variant="ghost" size="icon" aria-label="Change theme">
                <component :is="PaletteIcon" class="w-5 h-5" />
              </UiButton>
            </UiDropdownMenuTrigger>
            <UiDropdownMenuContent align="end" class="w-40">
              <UiDropdownMenuLabel>Theme</UiDropdownMenuLabel>
              <UiDropdownMenuSeparator />
              <UiDropdownMenuItem
                v-for="t in themes"
                :key="t.id"
                class="flex items-center gap-2.5 cursor-pointer"
                @click="setTheme(t.id)"
              >
                <span
                  class="w-4 h-4 rounded-full border-2 shrink-0"
                  :class="theme === t.id ? 'border-primary' : 'border-transparent'"
                  :style="{ backgroundColor: themeSwatchColor(t) }"
                />
                <span>{{ t.label }}</span>
                <component
                  v-if="theme === t.id"
                  :is="CheckIcon"
                  class="w-4 h-4 ml-auto text-primary"
                />
              </UiDropdownMenuItem>
            </UiDropdownMenuContent>
          </UiDropdownMenu>

          <!-- Dark mode toggle -->
          <UiButton
            variant="ghost"
            size="icon"
            :aria-label="isDark ? 'Switch to light mode' : 'Switch to dark mode'"
            @click="toggle"
          >
            <component :is="isDark ? SunIcon : MoonIcon" class="w-5 h-5" />
          </UiButton>

          <!-- Help -->
          <UiButton variant="ghost" size="icon" as-child>
            <NuxtLink to="/help" aria-label="Help">
              <component :is="CircleHelpIcon" class="w-5 h-5" />
            </NuxtLink>
          </UiButton>

          <!-- Logout -->
          <UiButton
            variant="ghost"
            size="icon"
            aria-label="Logout"
            class="hover:text-destructive"
            @click="logout"
          >
            <component :is="LogOutIcon" class="w-5 h-5" />
          </UiButton>
        </div>
      </div>
    </div>
  </header>
</template>

<script setup lang="ts">
import { DatabaseIcon, MoonIcon, SunIcon, LogOutIcon, CircleHelpIcon, PaletteIcon, CheckIcon } from 'lucide-vue-next'
import type { ThemeMeta } from '~/composables/useTheme'

const { isDark, toggle } = useAppColorMode()
const { theme, setTheme, themes } = useTheme()
const router = useRouter()
const authenticated = useCookie('authenticated')

const navLinks = [
  { to: '/', label: 'Dashboard' },
  { to: '/rules', label: 'Scoring Engine' },
  { to: '/audit', label: 'Audit Log' },
  { to: '/settings', label: 'Settings' }
]

function logout() {
  authenticated.value = null
  router.push('/login')
}

/** Map theme hue to a swatch color for the dropdown */
function themeSwatchColor(t: ThemeMeta): string {
  return `oklch(0.6 0.2 ${t.hue})`
}
</script>
