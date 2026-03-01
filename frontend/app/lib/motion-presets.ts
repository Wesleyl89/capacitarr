/**
 * Motion animation presets for Capacitarr.
 * Used with @vueuse/motion's v-motion directive.
 */

// Card/element entrance: fade up with spring
export const fadeUp = {
  initial: { opacity: 0, y: 12 },
  enter: {
    opacity: 1,
    y: 0,
    transition: { type: 'spring', stiffness: 260, damping: 24, delay: 100 }
  }
}

// Page-level fade in
export const pageFade = {
  initial: { opacity: 0 },
  enter: {
    opacity: 1,
    transition: { duration: 300 }
  }
}

// Slide in from left (for sidebars/nav)
export const slideInLeft = {
  initial: { opacity: 0, x: -20 },
  enter: {
    opacity: 1,
    x: 0,
    transition: { type: 'spring', stiffness: 300, damping: 28 }
  }
}

// Scale up (for modals/dialogs)
export const scaleUp = {
  initial: { opacity: 0, scale: 0.95 },
  enter: {
    opacity: 1,
    scale: 1,
    transition: { type: 'spring', stiffness: 350, damping: 25 }
  }
}

// Stagger delay helper: returns a delay based on index
export function staggerDelay(index: number, baseDelay = 50): number {
  return index * baseDelay
}

// Create a fadeUp variant with custom delay (for staggered lists)
export function fadeUpStaggered(index: number) {
  return {
    initial: { opacity: 0, y: 12 },
    enter: {
      opacity: 1,
      y: 0,
      transition: {
        type: 'spring',
        stiffness: 260,
        damping: 24,
        delay: 100 + staggerDelay(index)
      }
    }
  }
}
