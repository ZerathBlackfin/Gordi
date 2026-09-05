import { cubicOut } from 'svelte/easing'

// Needs a one-track grid around a clipped child, or padding shows at zero.
export function reveal(node, { duration = 220, delay = 0 } = {}) {
  return {
    delay,
    duration,
    easing: cubicOut,
    css: (t) => `grid-template-rows: ${t}fr; opacity: ${t}`,
  }
}
