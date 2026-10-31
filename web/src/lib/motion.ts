// 9router Galactus — 120fps GPU Liquid Flow & Spring Motion Engine
//
// 1. Dual-Layer Liquid Button Flow (`layoutId`-style connected stream):
//    Tracks every tab bar, segmented control, and filter pill group.
//    When switching from Button A to Button B, a GPU-composited (`translate3d`)
//    lead capsule + trailing liquid surface-tension stream stretches across the
//    gap and coalesces smoothly into Button B with zero layout reflow.
// 2. Zero-Jank Page Enter: lightweight single-layer GPU glide that never
//    forces multi-card `backdrop-filter` re-rasterization.
import gsap from 'gsap'

const reduced = () =>
  typeof window !== 'undefined' &&
  (window.matchMedia('(prefers-reduced-motion: reduce)').matches ||
    document.documentElement.getAttribute('data-anim-level') === 'off')

/** Svelte action: 120fps GPU-composited page transition without backdrop-filter thrashing. */
export function pageEnter(node: HTMLElement) {
  if (reduced()) return
  gsap.killTweensOf(node)
  gsap.fromTo(
    node,
    { opacity: 0, y: 6 },
    {
      opacity: 1,
      y: 0,
      duration: 0.22,
      ease: 'power3.out',
      force3D: true,
      clearProps: 'transform,opacity',
    }
  )
}

const INTERACTIVE = 'button:not(:disabled), a[href], [role="button"]'

// WeakMap storing the last active/clicked button rect per logical group container
const groupLastRect = new WeakMap<HTMLElement, DOMRect>()

function findGroupContainer(btn: HTMLElement): HTMLElement | null {
  if (btn.closest('[data-no-bridge]')) return null
  let curr: HTMLElement | null = btn.parentElement
  let depth = 0
  while (curr && depth < 4) {
    if (curr.hasAttribute('data-no-bridge')) return null
    if (
      curr.tagName === 'NAV' ||
      curr.getAttribute('role') === 'tablist' ||
      curr.hasAttribute('data-flow-group')
    ) {
      return curr
    }
    const interactiveChildren = Array.from(curr.children).filter(
      (c) => c.tagName === 'BUTTON' || c.tagName === 'A'
    )
    if (interactiveChildren.length >= 2) {
      return curr
    }
    curr = curr.parentElement
    depth++
  }
  return null
}

/**
 * Spawns a pure GPU-composited (`translate3d`) dual-layer liquid stream
 * (lead capsule + trailing surface-tension bridge) that pours smoothly from
 * `fromRect` into `toRect`.
 */
function animateConnectedBridge(fromRect: DOMRect, toRect: DOMRect, targetEl: HTMLElement) {
  const dx = toRect.left - fromRect.left
  const dy = toRect.top - fromRect.top
  const dist = Math.hypot(dx, dy)
  if (dist < 4 || dist > 680) return

  const computed = window.getComputedStyle(targetEl)
  const radius = computed.borderRadius || '13px'

  // Layer 1: Leading Liquid Capsule
  const lead = document.createElement('div')
  lead.className = 'galactus-flow-bridge'
  Object.assign(lead.style, {
    position: 'fixed',
    left: '0px',
    top: '0px',
    width: `${fromRect.width}px`,
    height: `${fromRect.height}px`,
    borderRadius: radius,
    pointerEvents: 'none',
    zIndex: '46',
    backgroundColor: 'rgba(229, 106, 74, 0.18)',
    border: '1px solid rgba(229, 106, 74, 0.42)',
    boxShadow: 'inset 0 1px 0 rgba(255, 255, 255, 0.22)',
    transformOrigin: 'center center',
    willChange: 'transform, width, height, opacity',
  })

  // Layer 2: Trailing Liquid Stream (stretches across the gap like water surface tension)
  const trail = document.createElement('div')
  trail.className = 'galactus-flow-trail'
  Object.assign(trail.style, {
    position: 'fixed',
    left: '0px',
    top: '0px',
    width: `${fromRect.width}px`,
    height: `${fromRect.height}px`,
    borderRadius: radius,
    pointerEvents: 'none',
    zIndex: '45',
    backgroundColor: 'rgba(229, 106, 74, 0.12)',
    border: '1px solid rgba(229, 106, 74, 0.24)',
    transformOrigin: 'center center',
    willChange: 'transform, width, height, opacity',
  })

  document.body.appendChild(trail)
  document.body.appendChild(lead)

  const isHorizontal = Math.abs(dx) >= Math.abs(dy)
  const stretchRatio = Math.min(1.38, 1 + dist / 220)
  const squeezeRatio = Math.max(0.9, 1 - dist / 950)

  const leadScaleX = isHorizontal ? stretchRatio : squeezeRatio
  const leadScaleY = isHorizontal ? squeezeRatio : stretchRatio

  const trailStretch = Math.min(1.75, 1 + dist / 140)
  const trailScaleX = isHorizontal ? trailStretch : 0.88
  const trailScaleY = isHorizontal ? 0.88 : trailStretch

  gsap.set([lead, trail], {
    x: fromRect.left,
    y: fromRect.top,
    scaleX: 1,
    scaleY: 1,
    force3D: true,
  })

  // Lead droplet glides and settles with elastic liquid surface tension
  gsap.to(lead, {
    x: toRect.left,
    y: toRect.top,
    width: toRect.width,
    height: toRect.height,
    duration: 0.42,
    ease: 'expo.out',
    force3D: true,
  })

  gsap.to(lead, {
    keyframes: [
      { scaleX: leadScaleX, scaleY: leadScaleY, opacity: 1, duration: 0.13, ease: 'sine.out' },
      { scaleX: 1, scaleY: 1, opacity: 0, duration: 0.31, ease: 'power2.out' },
    ],
    force3D: true,
    onComplete: () => lead.remove(),
  })

  // Trailing stream stretches across the midpoint and pours into the destination
  gsap.to(trail, {
    keyframes: [
      {
        x: fromRect.left + dx * 0.38,
        y: fromRect.top + dy * 0.38,
        width: (fromRect.width + toRect.width) * 0.5,
        height: (fromRect.height + toRect.height) * 0.5,
        scaleX: trailScaleX,
        scaleY: trailScaleY,
        opacity: 0.78,
        duration: 0.14,
        ease: 'power2.out',
      },
      {
        x: toRect.left,
        y: toRect.top,
        width: toRect.width,
        height: toRect.height,
        scaleX: 1,
        scaleY: 1,
        opacity: 0,
        duration: 0.34,
        ease: 'expo.out',
      },
    ],
    force3D: true,
    onComplete: () => trail.remove(),
  })
}

/** Installs delegated connected liquid button flow across the entire app. */
export function initGlossyMotion() {
  if (typeof document === 'undefined' || reduced()) return

  // Seed initial active buttons in groups so the first click flows from the active tab
  requestAnimationFrame(() => {
    document
      .querySelectorAll<HTMLElement>(
        'button[aria-selected="true"], button.text-brand-500, button.bg-primary, button.bg-brand-500'
      )
      .forEach((activeEl) => {
        const grp = findGroupContainer(activeEl)
        if (grp) {
          groupLastRect.set(grp, activeEl.getBoundingClientRect())
        }
      })
  })

  document.addEventListener(
    'pointerdown',
    (e) => {
      if (reduced()) return
      const el = (e.target as Element)?.closest<HTMLElement>(INTERACTIVE)
      if (!el || el.closest('[data-no-motion]') || el.closest('[data-no-bridge]')) return

      const grp = findGroupContainer(el)
      if (grp) {
        const currentRect = el.getBoundingClientRect()
        let prevRect = groupLastRect.get(grp)

        if (!prevRect) {
          const activeSibling = grp.querySelector<HTMLElement>(
            '[aria-selected="true"], .text-brand-500, .bg-primary, .bg-brand-500, .bg-surface'
          )
          if (activeSibling && activeSibling !== el) {
            prevRect = activeSibling.getBoundingClientRect()
          }
        }

        if (prevRect) {
          animateConnectedBridge(prevRect, currentRect, el)
        }
        groupLastRect.set(grp, currentRect)
      }
    },
    { passive: true }
  )
}
