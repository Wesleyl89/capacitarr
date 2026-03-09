<script setup lang="ts">
/**
 * ProseCode — overrides @nuxt/ui's default code block component.
 *
 * - For `language === 'mermaid'`: renders diagrams client-side via mermaid.render()
 *   with violet-branded dark/light themes and the ELK layout engine.
 * - For all other languages: passes through to the default slot (shiki-highlighted HTML).
 */
import { computed, nextTick, onMounted, ref, watch } from 'vue'

const props = defineProps<{
  code?: string
  language?: string
  filename?: string
  highlights?: number[]
  meta?: string
}>()

const isMermaid = computed(() => props.language === 'mermaid')

// ─── Mermaid state (client-only) ─────────────────────────────────
const mermaidSvg = ref('')
const renderError = ref('')
let renderCount = 0
let elkRegistered = false

// ─── Theme palettes ──────────────────────────────────────────────
const darkTheme = {
  primaryColor: '#1e1b4b',
  primaryBorderColor: '#8b5cf6',
  primaryTextColor: '#e9d5ff',
  lineColor: '#a78bfa',
  secondaryColor: '#110f24',
  tertiaryColor: '#110f24',
  mainBkg: '#1e1b4b',
  nodeBorder: '#8b5cf6',
  clusterBkg: '#110f24',
  clusterBorder: '#4c1d95',
  titleColor: '#e9d5ff',
  edgeLabelBackground: '#110f24',
  textColor: '#e9d5ff',
}

const lightTheme = {
  primaryColor: '#ede9fe',
  primaryBorderColor: '#8b5cf6',
  primaryTextColor: '#1e1b4b',
  lineColor: '#6d28d9',
  secondaryColor: '#f5f3ff',
  tertiaryColor: '#f5f3ff',
  mainBkg: '#ede9fe',
  nodeBorder: '#8b5cf6',
  clusterBkg: '#f5f3ff',
  clusterBorder: '#c4b5fd',
  titleColor: '#1e1b4b',
  edgeLabelBackground: '#f5f3ff',
  textColor: '#1e1b4b',
}

// ─── Mermaid rendering ───────────────────────────────────────────
async function renderDiagram() {
  if (!props.code) return

  try {
    const mermaid = (await import('mermaid')).default

    // Register ELK layout engine once
    if (!elkRegistered) {
      const elkModule = await import('@mermaid-js/layout-elk')
      mermaid.registerLayoutLoaders(elkModule.default || elkModule)
      elkRegistered = true
    }

    const colorMode = useColorMode()
    const isDark = colorMode.value === 'dark'

    mermaid.initialize({
      startOnLoad: false,
      theme: 'base',
      themeVariables: isDark ? darkTheme : lightTheme,
      flowchart: {
        defaultRenderer: 'elk',
        nodeSpacing: 60,
        rankSpacing: 70,
        padding: 20,
        curve: 'basis',
      },
      sequence: {
        actorMargin: 80,
      },
      fontFamily: '\'Geist Sans\', \'Geist\', ui-sans-serif, system-ui, sans-serif',
      themeCSS: `
        .node rect, .node polygon, .node circle, .node ellipse { rx: 8; ry: 8; }
        .cluster rect { rx: 12; ry: 12; }
        .edgeLabel { font-size: 12px; }
      `,
    })

    // Use a unique ID per render pass to avoid mermaid ID collisions
    renderCount++
    const { svg } = await mermaid.render(`mermaid-${renderCount}-${Date.now()}`, props.code)
    mermaidSvg.value = svg
    renderError.value = ''
  }
  catch (err) {
    renderError.value = String(err)
    console.error('[Mermaid] Render error:', err)
  }
}

// ─── Lifecycle ───────────────────────────────────────────────────
if (isMermaid.value) {
  onMounted(async () => {
    await renderDiagram()

    // Re-render when color mode changes
    const colorMode = useColorMode()
    watch(() => colorMode.value, async () => {
      await nextTick()
      await renderDiagram()
    })
  })
}
</script>

<template>
  <!-- Mermaid diagram: client-only rendering -->
  <ClientOnly v-if="isMermaid">
    <div class="mermaid-wrapper">
      <div
        v-if="mermaidSvg"
        class="mermaid-diagram"
        v-html="mermaidSvg"
      />
      <div v-else-if="renderError" class="mermaid-error">
        <p><strong>Diagram render error:</strong></p>
        <pre>{{ renderError }}</pre>
      </div>
      <div v-else class="mermaid-loading">
        <UIcon name="i-lucide-loader-2" class="size-5 animate-spin" />
        <span>Rendering diagram…</span>
      </div>
    </div>
    <template #fallback>
      <div class="mermaid-wrapper mermaid-fallback">
        <pre><code>{{ code }}</code></pre>
      </div>
    </template>
  </ClientOnly>

  <!-- Non-mermaid code: pass through to default slot (shiki-highlighted HTML) -->
  <slot v-else />
</template>

<style scoped>
.mermaid-wrapper {
  display: flex;
  justify-content: center;
  padding: 1.5rem 1rem;
  border-radius: 0.75rem;
  margin: 1.5rem 0;
  overflow-x: auto;
  background: var(--color-neutral-50);
  border: 1px solid var(--color-neutral-200);
}

:root.dark .mermaid-wrapper {
  background: var(--color-neutral-950);
  border-color: var(--color-neutral-800);
}

.mermaid-diagram :deep(svg) {
  max-width: 100%;
  height: auto;
}

.mermaid-loading {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  color: var(--color-neutral-500);
  font-size: 0.875rem;
  padding: 2rem;
}

.mermaid-error {
  color: var(--color-red-600);
  font-size: 0.875rem;
  padding: 1rem;
}

:root.dark .mermaid-error {
  color: var(--color-red-400);
}

.mermaid-error pre {
  margin-top: 0.5rem;
  white-space: pre-wrap;
  font-size: 0.75rem;
}

.mermaid-fallback {
  opacity: 0.7;
}

.mermaid-fallback pre {
  font-size: 0.8125rem;
  white-space: pre-wrap;
}
</style>
