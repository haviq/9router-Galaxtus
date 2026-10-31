<script lang="ts">
  import { onMount } from 'svelte'
  import gsap from 'gsap'

  let canvasEl: HTMLCanvasElement | undefined = $state()
  let containerEl: HTMLDivElement | undefined = $state()

  // Parallax Anchors (Driven by 3D Scroll Zoom & Parallax Drift)
  let planetGasGiantAnchor: HTMLDivElement | undefined = $state()
  let planetSaturnAnchor: HTMLDivElement | undefined = $state()
  let planetExoEarthAnchor: HTMLDivElement | undefined = $state()
  let planetCrimsonAnchor: HTMLDivElement | undefined = $state()
  let andromedaAnchor: HTMLDivElement | undefined = $state()
  let blackHoleAnchor: HTMLDivElement | undefined = $state()
  let sombreroAnchor: HTMLDivElement | undefined = $state()
  let asteroidsAnchor: HTMLDivElement | undefined = $state()

  // Inner Elements (Driven by Continuous GSAP Orbital Floating)
  let planetGasGiantEl: HTMLDivElement | undefined = $state()
  let planetSaturnEl: HTMLDivElement | undefined = $state()
  let planetExoEarthEl: HTMLDivElement | undefined = $state()
  let planetCrimsonEl: HTMLDivElement | undefined = $state()
  let moonOrbitEl: HTMLDivElement | undefined = $state()
  let andromedaEl: HTMLDivElement | undefined = $state()
  let blackHoleEl: HTMLDivElement | undefined = $state()
  let sombreroEl: HTMLDivElement | undefined = $state()

  onMount(() => {
    const reduced =
      window.matchMedia('(prefers-reduced-motion: reduce)').matches ||
      document.documentElement.getAttribute('data-anim-level') === 'off'

    // 1. Offscreen-Cached Starfield + Twinkling Flare Stars + Shooting Meteors
    let animId = 0
    const canvas = canvasEl
    if (canvas) {
      const ctx2d = canvas.getContext('2d', { alpha: true })
      const staticCanvas = document.createElement('canvas')
      const staticCtx = staticCanvas.getContext('2d', { alpha: true })

      let w = (canvas.width = staticCanvas.width = window.innerWidth)
      let h = (canvas.height = staticCanvas.height = window.innerHeight)

      interface Star {
        x: number
        y: number
        r: number
        baseAlpha: number
        speed: number
        phase: number
        color: string
        flare: boolean
      }

      let isDark = typeof document !== 'undefined' ? document.documentElement.classList.contains('dark') : true

      const darkPalette = ['#ffffff', '#e0f2fe', '#ddd6fe', '#fef3c7', '#fbcfe8', '#bae6fd']
      const lightPalette = ['#4338ca', '#6366f1', '#7c3aed', '#0284c7', '#ea580c', '#db2777', '#2563eb']

      const allStars: Star[] = Array.from({ length: 360 }, (_, i) => {
        const inBand = i % 5 < 2
        let nx = ((i * 7919 + 104729) % 10000) / 10000
        let ny = ((i * 15485863 + 32452843) % 10000) / 10000
        if (inBand) {
          const t = nx
          const spread = (ny - 0.5) * 0.32
          nx = Math.min(0.99, Math.max(0.01, t))
          ny = Math.min(0.99, Math.max(0.01, 1 - t * 0.78 + spread - 0.08))
        }
        const r = i % 29 === 0 ? 1.85 : i % 7 === 0 ? 1.2 : 0.75
        const curPalette = isDark ? darkPalette : lightPalette
        return {
          x: nx,
          y: ny,
          r,
          baseAlpha: 0.25 + (i % 70) / 100,
          speed: 0.6 + (i % 19) * 0.12,
          phase: (i * 1.37) % (Math.PI * 2),
          color: curPalette[i % curPalette.length],
          flare: i % 38 === 0,
        }
      })

      const staticStars = allStars.filter((_, i) => i % 9 !== 0)
      const dynamicStars = allStars.filter((_, i) => i % 9 === 0)

      const paintStaticStars = () => {
        if (!staticCtx) return
        staticCtx.clearRect(0, 0, w, h)
        const alphaMultiplier = isDark ? 0.85 : 1.2
        const rMultiplier = isDark ? 1 : 1.25
        for (let i = 0; i < staticStars.length; i++) {
          const s = staticStars[i]
          const sx = s.x * w
          const sy = s.y * h
          staticCtx.globalAlpha = Math.min(1, s.baseAlpha * alphaMultiplier)
          staticCtx.fillStyle = s.color
          staticCtx.beginPath()
          staticCtx.arc(sx, sy, s.r * rMultiplier, 0, Math.PI * 2)
          staticCtx.fill()
        }
      }

      const updateStarColors = () => {
        isDark = document.documentElement.classList.contains('dark')
        const curPalette = isDark ? darkPalette : lightPalette
        for (let i = 0; i < allStars.length; i++) {
          allStars[i].color = curPalette[i % curPalette.length]
        }
        paintStaticStars()
      }

      paintStaticStars()

      const themeObserver = new MutationObserver(() => {
        const currentDark = document.documentElement.classList.contains('dark')
        if (currentDark !== isDark) {
          updateStarColors()
        }
      })
      themeObserver.observe(document.documentElement, {
        attributes: true,
        attributeFilter: ['class'],
      })

      const handleResize = () => {
        if (!canvas) return
        w = canvas.width = staticCanvas.width = window.innerWidth
        h = canvas.height = staticCanvas.height = window.innerHeight
        paintStaticStars()
      }
      window.addEventListener('resize', handleResize, { passive: true })

      const meteor = {
        x: 0,
        y: 0,
        vx: -14,
        vy: 8,
        len: 150,
        alpha: 0,
        active: false,
        cooldown: 160,
      }

      let t = 0
      const render = () => {
        if (!ctx2d) return
        t += 0.016
        ctx2d.clearRect(0, 0, w, h)
        ctx2d.globalAlpha = 1
        ctx2d.drawImage(staticCanvas, 0, 0)

        const alphaMultiplier = isDark ? 1 : 1.25
        const rMultiplier = isDark ? 1 : 1.25
        for (let i = 0; i < dynamicStars.length; i++) {
          const s = dynamicStars[i]
          const sx = s.x * w
          const sy = s.y * h
          const twinkle = reduced ? 1 : 0.55 + 0.45 * Math.sin(t * s.speed + s.phase)
          const a = Math.min(1, s.baseAlpha * twinkle * alphaMultiplier)

          ctx2d.globalAlpha = a
          ctx2d.fillStyle = s.color
          ctx2d.beginPath()
          ctx2d.arc(sx, sy, s.r * rMultiplier, 0, Math.PI * 2)
          ctx2d.fill()

          if (s.flare) {
            ctx2d.globalAlpha = a * (isDark ? 0.45 : 0.65)
            ctx2d.strokeStyle = s.color
            ctx2d.lineWidth = isDark ? 0.8 : 1.1
            ctx2d.beginPath()
            ctx2d.moveTo(sx - s.r * 4.2 * rMultiplier, sy)
            ctx2d.lineTo(sx + s.r * 4.2 * rMultiplier, sy)
            ctx2d.moveTo(sx, sy - s.r * 4.2 * rMultiplier)
            ctx2d.lineTo(sx, sy + s.r * 4.2 * rMultiplier)
            ctx2d.stroke()
          }
        }

        if (!reduced) {
          if (!meteor.active) {
            meteor.cooldown--
            if (meteor.cooldown <= 0) {
              meteor.active = true
              meteor.x = w * (0.45 + Math.random() * 0.5)
              meteor.y = h * (0.05 + Math.random() * 0.35)
              meteor.vx = -(12 + Math.random() * 8)
              meteor.vy = 6 + Math.random() * 5
              meteor.len = 130 + Math.random() * 90
              meteor.alpha = 1
            }
          } else {
            meteor.x += meteor.vx
            meteor.y += meteor.vy
            meteor.alpha -= 0.018

            if (meteor.alpha <= 0 || meteor.x < -200 || meteor.y > h + 200) {
              meteor.active = false
              meteor.cooldown = 240 + Math.floor(Math.random() * 260)
            } else {
              const grad = ctx2d.createLinearGradient(
                meteor.x,
                meteor.y,
                meteor.x - meteor.vx * 10,
                meteor.y - meteor.vy * 10
              )
              if (!isDark) {
                grad.addColorStop(0, `rgba(99, 102, 241, ${meteor.alpha * 0.95})`)
                grad.addColorStop(0.35, `rgba(234, 88, 12, ${meteor.alpha * 0.55})`)
                grad.addColorStop(1, 'rgba(234, 88, 12, 0)')
              } else {
                grad.addColorStop(0, `rgba(255, 255, 255, ${meteor.alpha})`)
                grad.addColorStop(0.3, `rgba(196, 181, 253, ${meteor.alpha * 0.6})`)
                grad.addColorStop(1, 'rgba(196, 181, 253, 0)')
              }

              ctx2d.globalAlpha = 1
              ctx2d.strokeStyle = grad
              ctx2d.lineWidth = 1.8
              ctx2d.beginPath()
              ctx2d.moveTo(meteor.x, meteor.y)
              ctx2d.lineTo(meteor.x - meteor.vx * 9, meteor.y - meteor.vy * 9)
              ctx2d.stroke()
            }
          }
          animId = requestAnimationFrame(render)
        }
      }
      render()
    }

    // 2. High-Performance 3D Parallax Scroll & Smooth Cinematic Zoom Engine
    let targetScroll = 0
    let currentScroll = 0
    let parallaxRafId = 0

    const updateScroll = () => {
      const mainEl = document.querySelector('main')
      const mainScroll = mainEl ? mainEl.scrollTop : 0
      const winScroll = window.scrollY || document.documentElement.scrollTop || 0
      targetScroll = Math.max(mainScroll, winScroll)
    }

    const onScroll = () => {
      updateScroll()
    }

    window.addEventListener('scroll', onScroll, { passive: true })
    const mainEl = document.querySelector('main')
    if (mainEl) {
      mainEl.addEventListener('scroll', onScroll, { passive: true })
    }

    const bodyObserver = new MutationObserver(() => {
      const curMain = document.querySelector('main')
      if (curMain && curMain !== mainEl) {
        curMain.addEventListener('scroll', onScroll, { passive: true })
      }
    })
    bodyObserver.observe(document.body, { childList: true, subtree: true })

    const renderParallax = () => {
      if (!reduced) {
        currentScroll += (targetScroll - currentScroll) * 0.085
        const norm = Math.min(2.0, currentScroll / 450)

        // 1. Gas Giant (Foreground Supergiant): aggressive 3D push-in zoom (+48%) + downward drift
        if (planetGasGiantAnchor) {
          const scale = 1 + norm * 0.48
          const x = -norm * 45
          const y = norm * 90
          planetGasGiantAnchor.style.transform = `translate3d(${x}px, ${y}px, 0) scale(${scale})`
        }

        // 2. Saturn (Bottom-Left Ringed Giant): zooms (+42%) and glides upward into view
        if (planetSaturnAnchor) {
          const scale = 1 + norm * 0.42
          const x = norm * 40
          const y = -norm * 85
          planetSaturnAnchor.style.transform = `translate3d(${x}px, ${y}px, 0) scale(${scale})`
        }

        // 3. Exo-Earth (Oceanic Jewel): zooms (+38%) and floats upward with majestic presence
        if (planetExoEarthAnchor) {
          const scale = 1 + norm * 0.38
          const x = -norm * 35
          const y = -norm * 75
          planetExoEarthAnchor.style.transform = `translate3d(${x}px, ${y}px, 0) scale(${scale})`
        }

        // 4. Crimson Dwarf World: zooms (+34%) and pans diagonally
        if (planetCrimsonAnchor) {
          const scale = 1 + norm * 0.34
          const x = norm * 35
          const y = norm * 55
          planetCrimsonAnchor.style.transform = `translate3d(${x}px, ${y}px, 0) scale(${scale})`
        }

        // 5. Andromeda Galaxy: deep celestial background push (+18%) and subtle drift
        if (andromedaAnchor) {
          const scale = 1 + norm * 0.18
          const y = norm * 40
          andromedaAnchor.style.transform = `translate3d(0, ${y}px, 0) scale(${scale})`
        }

        // 6. Gargantua Supermassive Black Hole: zooms (+26%) with warped relativistic shift
        if (blackHoleAnchor) {
          const scale = 1 + norm * 0.26
          const x = -norm * 25
          const y = norm * 50
          blackHoleAnchor.style.transform = `translate3d(${x}px, ${y}px, 0) scale(${scale})`
        }

        // 7. Sombrero Galaxy (NGC 4594): zooms (+22%) and drifts
        if (sombreroAnchor) {
          const scale = 1 + norm * 0.22
          const x = norm * 30
          const y = -norm * 35
          sombreroAnchor.style.transform = `translate3d(${x}px, ${y}px, 0) scale(${scale})`
        }

        // 8. Asteroid Belt: moves dynamically on parallax plane
        if (asteroidsAnchor) {
          const x = -norm * 50
          const y = -norm * 40
          asteroidsAnchor.style.transform = `translate3d(${x}px, ${y}px, 0)`
        }
      }
      parallaxRafId = requestAnimationFrame(renderParallax)
    }
    renderParallax()

    if (reduced) return

    // 3. GSAP Orbital & Floating Choreography for Photorealistic 3D Planets & Deep Space Systems
    const gsapCtx = gsap.context(() => {
      if (planetGasGiantEl) {
        gsap.to(planetGasGiantEl, {
          y: -16,
          x: 10,
          rotation: 2.5,
          duration: 16,
          repeat: -1,
          yoyo: true,
          ease: 'sine.inOut',
          force3D: true,
        })
      }
      if (planetSaturnEl) {
        gsap.to(planetSaturnEl, {
          y: 14,
          x: -12,
          rotation: -2,
          duration: 20,
          repeat: -1,
          yoyo: true,
          ease: 'sine.inOut',
          force3D: true,
        })
      }
      if (planetExoEarthEl) {
        gsap.to(planetExoEarthEl, {
          y: -12,
          x: -8,
          duration: 13,
          repeat: -1,
          yoyo: true,
          ease: 'sine.inOut',
          force3D: true,
        })
      }
      if (planetCrimsonEl) {
        gsap.to(planetCrimsonEl, {
          y: 10,
          x: 9,
          duration: 11,
          repeat: -1,
          yoyo: true,
          ease: 'sine.inOut',
          force3D: true,
        })
      }
      if (moonOrbitEl) {
        gsap.to(moonOrbitEl, {
          rotation: 360,
          duration: 44,
          repeat: -1,
          ease: 'none',
          force3D: true,
        })
      }
      if (andromedaEl) {
        gsap.to(andromedaEl, {
          rotation: 360,
          duration: 180,
          repeat: -1,
          ease: 'none',
          force3D: true,
        })
      }
      if (blackHoleEl) {
        gsap.to(blackHoleEl, {
          rotation: -360,
          duration: 90,
          repeat: -1,
          ease: 'none',
          force3D: true,
        })
      }
      if (sombreroEl) {
        gsap.to(sombreroEl, {
          rotation: 3,
          y: -8,
          duration: 18,
          repeat: -1,
          yoyo: true,
          ease: 'sine.inOut',
          force3D: true,
        })
      }
    }, containerEl)

    const handleVisibilityChange = () => {
      if (document.hidden) {
        cancelAnimationFrame(animId)
        cancelAnimationFrame(parallaxRafId)
        if (gsapCtx && typeof gsapCtx.pause === 'function') gsapCtx.pause()
      } else {
        if (!reduced) {
          animId = requestAnimationFrame(render)
          parallaxRafId = requestAnimationFrame(renderParallax)
          if (gsapCtx && typeof gsapCtx.resume === 'function') gsapCtx.resume()
        }
      }
    }
    document.addEventListener('visibilitychange', handleVisibilityChange)

    return () => {
      cancelAnimationFrame(animId)
      cancelAnimationFrame(parallaxRafId)
      themeObserver.disconnect()
      bodyObserver.disconnect()
      window.removeEventListener('resize', handleResize)
      window.removeEventListener('scroll', onScroll)
      if (mainEl) mainEl.removeEventListener('scroll', onScroll)
      document.removeEventListener('visibilitychange', handleVisibilityChange)
      gsapCtx.revert()
    }
  })
</script>

<div
  bind:this={containerEl}
  class="galaxy-scene fixed inset-0 pointer-events-none overflow-hidden select-none transition-opacity duration-500 opacity-100"
  style="z-index: 0;"
  aria-hidden="true"
>
  <!-- 1. Deep Milky Way Galactic Core & Volumetric Nebula Clouds -->
  <div class="milky-way-core"></div>
  <div class="nebula-pillars"></div>

  <!-- 2. High-DPI Starfield & Meteor Canvas -->
  <canvas bind:this={canvasEl} class="absolute inset-0 w-full h-full"></canvas>

  <!-- 3. GALAXY 1: Realistic Inclined Spiral Galaxy (Andromeda style) -->
  <div bind:this={andromedaAnchor} class="celestial-anchor andromeda-system">
    <div bind:this={andromedaEl} class="andromeda-disc">
      <div class="andromeda-core"></div>
      <div class="andromeda-arms"></div>
    </div>
  </div>

  <!-- 4. GALAXY 2: Gargantua Supermassive Black Hole & Gravitational Accretion Ring -->
  <div bind:this={blackHoleAnchor} class="celestial-anchor black-hole-system">
    <div bind:this={blackHoleEl} class="black-hole-disc">
      <div class="black-hole-jet"></div>
      <div class="accretion-warp-top"></div>
      <div class="accretion-disc-main"></div>
      <div class="event-horizon-shadow"></div>
      <div class="photon-ring-flare"></div>
      <div class="accretion-warp-bottom"></div>
    </div>
  </div>

  <!-- 5. GALAXY 3: Sombrero Galaxy (NGC 4594 Edge-On Interstellar Dust Lane Disc) -->
  <div bind:this={sombreroAnchor} class="celestial-anchor sombrero-system">
    <div bind:this={sombreroEl} class="sombrero-disc">
      <div class="sombrero-halo"></div>
      <div class="sombrero-bulge"></div>
      <div class="sombrero-dust-disc"></div>
      <div class="sombrero-dust-line"></div>
    </div>
  </div>

  <!-- 6. Floating Asteroid Belt Stream -->
  <div bind:this={asteroidsAnchor} class="celestial-anchor asteroid-belt-stream">
    <span class="asteroid ast-1"></span>
    <span class="asteroid ast-2"></span>
    <span class="asteroid ast-3"></span>
    <span class="asteroid ast-4"></span>
    <span class="asteroid ast-5"></span>
    <span class="asteroid ast-6"></span>
  </div>

  <!-- 7. PLANET 1: Photorealistic Galactus Gas Supergiant (Top-Right) -->
  <div bind:this={planetGasGiantAnchor} class="planet-anchor planet-gas-giant">
    <div bind:this={planetGasGiantEl} class="planet-inner-orbit">
      <div class="gas-giant-atmosphere"></div>
      <div class="gas-giant-sphere">
        <div class="gas-giant-clouds"></div>
        <div class="gas-giant-terminator"></div>
        <div class="gas-giant-fresnel"></div>
      </div>
      <div bind:this={moonOrbitEl} class="gas-giant-moon-orbit">
        <div class="cratered-moon">
          <span class="moon-crater c1"></span>
          <span class="moon-crater c2"></span>
        </div>
      </div>
    </div>
  </div>

  <!-- 8. PLANET 2: Photorealistic Ringed Saturnian Exoplanet (Bottom-Left) -->
  <div bind:this={planetSaturnAnchor} class="planet-anchor planet-saturn">
    <div bind:this={planetSaturnEl} class="planet-inner-orbit">
      <div class="saturn-ambient-glow"></div>
      <div class="saturn-rings saturn-rings-back"></div>
      <div class="saturn-sphere">
        <div class="saturn-bands"></div>
        <div class="saturn-terminator"></div>
      </div>
      <div class="saturn-rings saturn-rings-front"></div>
    </div>
  </div>

  <!-- 9. PLANET 3: Realistic Oceanic Exo-Earth with Atmosphere & Clouds (Bottom-Right) -->
  <div bind:this={planetExoEarthAnchor} class="planet-anchor planet-exo-earth">
    <div bind:this={planetExoEarthEl} class="planet-inner-orbit">
      <div class="exo-earth-glow"></div>
      <div class="exo-earth-sphere">
        <div class="exo-earth-continents"></div>
        <div class="exo-earth-clouds"></div>
        <div class="exo-earth-terminator"></div>
      </div>
    </div>
  </div>

  <!-- 10. PLANET 4: Magma / Volcanic Dwarf Planet (Upper Left-Center) -->
  <div bind:this={planetCrimsonAnchor} class="planet-anchor planet-crimson">
    <div bind:this={planetCrimsonEl} class="planet-inner-orbit">
      <div class="crimson-sphere">
        <div class="crimson-terminator"></div>
      </div>
    </div>
  </div>
</div>

<style>
  .galaxy-scene {
    contain: strict;
    transform: translateZ(0);
    background:
      radial-gradient(ellipse 120% 80% at 50% -10%, #0d132e 0%, #050714 52%, #020308 100%);
    transition: background 0.4s ease;
  }

  /* Celestial Daylight / Cosmic Dawn (Light Theme) */
  :global(html:not(.dark)) .galaxy-scene,
  :global(.light) .galaxy-scene {
    background:
      radial-gradient(ellipse 130% 90% at 50% -10%, #e0e7ff 0%, #ede9fe 25%, #e2e8f0 60%, #cbd5e1 100%);
  }

  /* Volumetric Milky Way diagonal dust lane */
  .milky-way-core {
    position: absolute;
    inset: -25%;
    background:
      radial-gradient(45% 22% at 62% 28%, rgba(139, 92, 246, 0.22) 0%, rgba(79, 70, 229, 0.09) 48%, transparent 75%),
      radial-gradient(38% 18% at 35% 68%, rgba(229, 106, 74, 0.16) 0%, rgba(236, 72, 153, 0.08) 50%, transparent 78%),
      radial-gradient(50% 20% at 50% 50%, rgba(56, 189, 248, 0.10) 0%, transparent 70%);
    transform: rotate(-28deg) translateZ(0);
    filter: blur(42px);
    transition: background 0.4s ease;
  }

  :global(html:not(.dark)) .milky-way-core,
  :global(.light) .milky-way-core {
    background:
      radial-gradient(45% 22% at 62% 28%, rgba(124, 58, 237, 0.15) 0%, rgba(79, 70, 229, 0.07) 48%, transparent 75%),
      radial-gradient(38% 18% at 35% 68%, rgba(229, 106, 74, 0.12) 0%, rgba(219, 39, 119, 0.07) 50%, transparent 78%),
      radial-gradient(50% 20% at 50% 50%, rgba(2, 132, 199, 0.09) 0%, transparent 70%);
    mix-blend-mode: multiply;
  }

  .nebula-pillars {
    position: absolute;
    inset: -10%;
    background:
      radial-gradient(28% 24% at 82% 72%, rgba(14, 165, 233, 0.14), transparent 72%),
      radial-gradient(24% 22% at 18% 24%, rgba(168, 85, 247, 0.15), transparent 70%);
    filter: blur(55px);
    transform: translateZ(0);
    transition: background 0.4s ease;
  }

  :global(html:not(.dark)) .nebula-pillars,
  :global(.light) .nebula-pillars {
    background:
      radial-gradient(28% 24% at 82% 72%, rgba(2, 132, 199, 0.11), transparent 72%),
      radial-gradient(24% 22% at 18% 24%, rgba(147, 51, 234, 0.12), transparent 70%);
    mix-blend-mode: multiply;
  }

  /* Celestial Anchors (GPU Parallax Transformed) */
  .celestial-anchor,
  .planet-anchor {
    position: absolute;
    will-change: transform;
    transform: translate3d(0, 0, 0);
  }

  .planet-inner-orbit {
    position: relative;
    width: 100%;
    height: 100%;
    will-change: transform;
  }

  /* =========================================================
   * GALAXY 1: Realistic Inclined Spiral Galaxy (Andromeda Style)
   * ========================================================= */
  .andromeda-system {
    top: 24%;
    left: 42%;
    width: 420px;
    height: 190px;
    transform: rotate(-26deg) scaleY(0.42) translateZ(0);
    opacity: 0.72;
    transition: opacity 0.4s ease;
  }

  :global(html:not(.dark)) .andromeda-system,
  :global(.light) .andromeda-system {
    opacity: 0.48;
    mix-blend-mode: multiply;
  }

  .andromeda-disc {
    position: relative;
    width: 420px;
    height: 420px;
    margin-top: -115px;
    border-radius: 50%;
    will-change: transform;
  }

  .andromeda-core {
    position: absolute;
    inset: 135px;
    border-radius: 50%;
    background: radial-gradient(
      circle,
      #ffffff 0%,
      #fef3c7 18%,
      rgba(251, 191, 36, 0.55) 38%,
      rgba(167, 139, 250, 0.25) 65%,
      transparent 82%
    );
    filter: blur(8px);
  }

  .andromeda-arms {
    position: absolute;
    inset: 0;
    border-radius: 50%;
    background:
      conic-gradient(
        from 0deg,
        transparent 0deg,
        rgba(196, 181, 253, 0.22) 45deg,
        rgba(56, 189, 248, 0.14) 95deg,
        transparent 160deg,
        rgba(196, 181, 253, 0.22) 225deg,
        rgba(244, 114, 182, 0.14) 275deg,
        transparent 340deg
      ),
      radial-gradient(circle, rgba(255, 255, 255, 0.2) 0%, rgba(139, 92, 246, 0.15) 42%, transparent 72%);
    filter: blur(12px);
  }

  /* =========================================================
   * GALAXY 2: Gargantua Supermassive Black Hole & Accretion Ring
   * ========================================================= */
  .black-hole-system {
    top: 14%;
    left: 65%;
    width: 220px;
    height: 220px;
    opacity: 0.85;
    z-index: 1;
  }

  :global(html:not(.dark)) .black-hole-system,
  :global(.light) .black-hole-system {
    opacity: 0.65;
    mix-blend-mode: multiply;
  }

  .black-hole-disc {
    position: relative;
    width: 100%;
    height: 100%;
    will-change: transform;
  }

  /* Gravitationally warped Einstein top ring */
  .accretion-warp-top {
    position: absolute;
    top: 18%;
    left: 10%;
    width: 80%;
    height: 64%;
    border-radius: 50%;
    background: radial-gradient(
      ellipse at 50% 30%,
      rgba(254, 240, 138, 0.95) 0%,
      rgba(251, 146, 60, 0.75) 35%,
      rgba(234, 88, 12, 0.4) 60%,
      transparent 80%
    );
    filter: blur(6px);
    transform: rotate(-16deg) scaleY(0.72);
  }

  /* Primary relativistic accretion disk */
  .accretion-disc-main {
    position: absolute;
    top: 36%;
    left: 0;
    width: 100%;
    height: 28%;
    border-radius: 50%;
    background: linear-gradient(
      90deg,
      rgba(255, 255, 255, 0.95) 0%,
      rgba(254, 215, 170, 0.9) 20%,
      rgba(251, 146, 60, 0.85) 45%,
      rgba(194, 65, 12, 0.6) 80%,
      transparent 100%
    );
    filter: blur(4px);
    transform: rotate(-16deg);
    box-shadow: 0 0 24px rgba(251, 146, 60, 0.65);
  }

  /* Pure black event horizon */
  .event-horizon-shadow {
    position: absolute;
    top: 32%;
    left: 32%;
    width: 36%;
    height: 36%;
    border-radius: 50%;
    background: #000000;
    box-shadow:
      inset 0 0 12px rgba(0, 0, 0, 1),
      0 0 10px rgba(0, 0, 0, 0.9);
    z-index: 2;
  }

  /* Thin photon ring around the event horizon */
  .photon-ring-flare {
    position: absolute;
    top: 30%;
    left: 30%;
    width: 40%;
    height: 40%;
    border-radius: 50%;
    border: 1.5px solid rgba(254, 240, 138, 0.95);
    box-shadow: 0 0 14px rgba(254, 240, 138, 0.8);
    z-index: 1;
  }

  .black-hole-jet {
    position: absolute;
    top: -20%;
    left: 48%;
    width: 4%;
    height: 140%;
    background: linear-gradient(
      180deg,
      transparent 0%,
      rgba(56, 189, 248, 0.4) 30%,
      rgba(255, 255, 255, 0.8) 50%,
      rgba(56, 189, 248, 0.4) 70%,
      transparent 100%
    );
    filter: blur(3px);
    transform: rotate(74deg);
    opacity: 0.6;
  }

  /* =========================================================
   * GALAXY 3: Sombrero Galaxy (NGC 4594 Edge-on Dust Lane Disc)
   * ========================================================= */
  .sombrero-system {
    top: 50%;
    left: 4%;
    width: 260px;
    height: 110px;
    opacity: 0.78;
    z-index: 1;
  }

  :global(html:not(.dark)) .sombrero-system,
  :global(.light) .sombrero-system {
    opacity: 0.55;
    mix-blend-mode: multiply;
  }

  .sombrero-disc {
    position: relative;
    width: 100%;
    height: 100%;
    will-change: transform;
    transform: rotate(-8deg);
  }

  .sombrero-halo {
    position: absolute;
    inset: -10%;
    border-radius: 50%;
    background: radial-gradient(ellipse at 50% 50%, rgba(196, 181, 253, 0.22) 0%, rgba(99, 102, 241, 0.08) 50%, transparent 75%);
    filter: blur(14px);
  }

  .sombrero-bulge {
    position: absolute;
    top: 15%;
    left: 32%;
    width: 36%;
    height: 70%;
    border-radius: 50%;
    background: radial-gradient(circle at 50% 50%, #ffffff 0%, #fef3c7 28%, rgba(251, 191, 36, 0.6) 55%, transparent 78%);
    filter: blur(6px);
  }

  .sombrero-dust-disc {
    position: absolute;
    top: 38%;
    left: 2%;
    width: 96%;
    height: 24%;
    border-radius: 50%;
    background: linear-gradient(
      90deg,
      transparent 0%,
      rgba(224, 231, 255, 0.4) 20%,
      rgba(255, 255, 255, 0.8) 50%,
      rgba(224, 231, 255, 0.4) 80%,
      transparent 100%
    );
    filter: blur(2px);
  }

  .sombrero-dust-line {
    position: absolute;
    top: 48%;
    left: 5%;
    width: 90%;
    height: 3px;
    background: rgba(15, 23, 42, 0.85);
    border-radius: 2px;
    filter: blur(1px);
    box-shadow: 0 0 6px rgba(15, 23, 42, 0.9);
  }

  /* =========================================================
   * Floating Asteroid Belt Stream
   * ========================================================= */
  .asteroid-belt-stream {
    bottom: 22%;
    left: 28%;
    width: 340px;
    height: 120px;
    opacity: 0.7;
    transform: rotate(-14deg);
    pointer-events: none;
  }

  .asteroid {
    position: absolute;
    border-radius: 40% 60% 70% 30% / 40% 50% 60% 50%;
    background: radial-gradient(circle at 30% 30%, #e2e8f0 0%, #64748b 60%, #1e293b 100%);
    box-shadow: 0 0 4px rgba(0, 0, 0, 0.5);
  }
  .ast-1 { width: 7px; height: 6px; top: 20%; left: 10%; }
  .ast-2 { width: 5px; height: 5px; top: 45%; left: 32%; }
  .ast-3 { width: 8px; height: 7px; top: 30%; left: 55%; }
  .ast-4 { width: 4px; height: 4px; top: 65%; left: 74%; }
  .ast-5 { width: 6px; height: 5px; top: 40%; left: 88%; }
  .ast-6 { width: 9px; height: 8px; top: 15%; left: 42%; }

  /* =========================================================
   * PLANET 1: Photorealistic Gas Supergiant (Top-Right)
   * ========================================================= */
  .planet-gas-giant {
    top: -55px;
    right: -45px;
    width: 360px;
    height: 360px;
    z-index: 2;
  }

  .gas-giant-atmosphere {
    position: absolute;
    inset: 10px;
    border-radius: 50%;
    background: radial-gradient(
      circle at 34% 34%,
      rgba(167, 139, 250, 0.42) 0%,
      rgba(99, 102, 241, 0.22) 52%,
      rgba(56, 189, 248, 0.08) 66%,
      transparent 76%
    );
    filter: blur(20px);
  }

  .gas-giant-sphere {
    position: absolute;
    inset: 38px;
    border-radius: 50%;
    overflow: hidden;
    background: radial-gradient(
      circle at 30% 28%,
      #e0e7ff 0%,
      #a78bfa 18%,
      #6d28d9 42%,
      #311068 66%,
      #090418 92%
    );
    box-shadow:
      inset -48px -48px 75px rgba(2, 1, 9, 0.98),
      inset 8px 8px 24px rgba(224, 231, 255, 0.55),
      0 0 60px -10px rgba(139, 92, 246, 0.45);
  }

  .gas-giant-clouds {
    position: absolute;
    inset: -15%;
    background: repeating-linear-gradient(
      -21deg,
      rgba(255, 255, 255, 0.14) 0px,
      rgba(221, 214, 254, 0.08) 9px,
      rgba(76, 29, 149, 0.28) 18px,
      rgba(196, 181, 253, 0.15) 29px,
      rgba(30, 27, 75, 0.35) 42px,
      rgba(139, 92, 246, 0.12) 54px
    );
    mix-blend-mode: soft-light;
    filter: blur(1.5px);
    transform: scale(1.1);
  }

  .gas-giant-terminator {
    position: absolute;
    inset: 0;
    border-radius: 50%;
    background: radial-gradient(
      circle at 24% 24%,
      rgba(255, 255, 255, 0.12) 0%,
      transparent 34%,
      rgba(4, 3, 14, 0.78) 66%,
      rgba(2, 1, 8, 0.97) 86%
    );
  }

  .gas-giant-fresnel {
    position: absolute;
    inset: 0;
    border-radius: 50%;
    box-shadow: inset 3px 3px 10px rgba(186, 230, 253, 0.65);
    mask-image: linear-gradient(135deg, #000 0%, transparent 55%);
    -webkit-mask-image: linear-gradient(135deg, #000 0%, transparent 55%);
  }

  .gas-giant-moon-orbit {
    position: absolute;
    inset: -6px;
    border-radius: 50%;
    border: 1px solid rgba(224, 231, 255, 0.09);
    will-change: transform;
  }

  .cratered-moon {
    position: absolute;
    top: 16px;
    left: 50%;
    width: 26px;
    height: 26px;
    margin-left: -13px;
    border-radius: 50%;
    background: radial-gradient(circle at 28% 28%, #f8fafc 0%, #cbd5e1 35%, #475569 72%, #0f172a 95%);
    box-shadow:
      inset -6px -6px 10px rgba(2, 6, 23, 0.95),
      0 0 16px rgba(248, 250, 252, 0.55);
    overflow: hidden;
  }

  .moon-crater {
    position: absolute;
    border-radius: 50%;
    background: rgba(30, 41, 59, 0.45);
    box-shadow: inset 1px 1px 2px rgba(0, 0, 0, 0.6);
  }
  .moon-crater.c1 { width: 6px; height: 6px; top: 7px; left: 9px; }
  .moon-crater.c2 { width: 4px; height: 4px; top: 14px; left: 6px; }

  /* =========================================================
   * PLANET 2: Photorealistic Saturnian Ringed Planet (Bottom-Left)
   * ========================================================= */
  .planet-saturn {
    bottom: 2%;
    left: 15%;
    width: 290px;
    height: 290px;
    z-index: 2;
  }

  .saturn-ambient-glow {
    position: absolute;
    inset: 10px;
    border-radius: 50%;
    background: radial-gradient(circle, rgba(251, 146, 60, 0.22) 0%, transparent 70%);
    filter: blur(22px);
  }

  .saturn-sphere {
    position: absolute;
    inset: 72px;
    border-radius: 50%;
    overflow: hidden;
    background: radial-gradient(
      circle at 30% 28%,
      #ffedd5 0%,
      #fdba74 22%,
      #ea580c 52%,
      #7c2d12 76%,
      #140602 96%
    );
    box-shadow:
      inset -28px -26px 46px rgba(5, 2, 1, 0.96),
      inset 5px 5px 14px rgba(255, 237, 213, 0.55),
      0 0 38px rgba(234, 88, 12, 0.28);
    z-index: 2;
  }

  .saturn-bands {
    position: absolute;
    inset: -10%;
    background: repeating-linear-gradient(
      -24deg,
      rgba(255, 255, 255, 0.12) 0px,
      rgba(254, 215, 170, 0.08) 6px,
      rgba(124, 45, 18, 0.28) 13px,
      transparent 20px
    );
    mix-blend-mode: overlay;
  }

  .saturn-terminator {
    position: absolute;
    inset: 0;
    border-radius: 50%;
    background: radial-gradient(circle at 26% 26%, transparent 38%, rgba(5, 2, 0, 0.92) 82%);
  }

  .saturn-rings {
    position: absolute;
    top: 50%;
    left: 50%;
    width: 310px;
    height: 310px;
    margin-left: -155px;
    margin-top: -155px;
    border-radius: 50%;
    background: radial-gradient(
      circle,
      transparent 0%,
      transparent 34%,
      rgba(254, 215, 170, 0.18) 36%,
      rgba(251, 146, 60, 0.42) 43%,
      rgba(253, 186, 116, 0.52) 48%,
      rgba(15, 6, 2, 0.75) 50.5%,
      rgba(251, 146, 60, 0.38) 52.5%,
      rgba(234, 88, 12, 0.25) 60%,
      rgba(254, 215, 170, 0.1) 66%,
      transparent 70%
    );
    transform: rotate(-24deg) scaleY(0.32);
  }

  .saturn-rings-back {
    z-index: 1;
    clip-path: polygon(0 0, 100% 0, 100% 50%, 0 50%);
    opacity: 0.78;
  }

  .saturn-rings-front {
    z-index: 3;
    clip-path: polygon(0 50%, 100% 50%, 100% 100%, 0 100%);
    filter: drop-shadow(0 6px 8px rgba(0, 0, 0, 0.65));
  }

  /* =========================================================
   * PLANET 3: Oceanic Exo-Earth with Atmosphere (Bottom-Right)
   * ========================================================= */
  .planet-exo-earth {
    bottom: 9%;
    right: 6%;
    width: 136px;
    height: 136px;
    z-index: 2;
  }

  .exo-earth-glow {
    position: absolute;
    inset: -12px;
    border-radius: 50%;
    background: radial-gradient(circle, rgba(56, 189, 248, 0.35) 0%, transparent 70%);
    filter: blur(12px);
  }

  .exo-earth-sphere {
    position: relative;
    width: 100%;
    height: 100%;
    border-radius: 50%;
    overflow: hidden;
    background: radial-gradient(
      circle at 30% 28%,
      #e0f2fe 0%,
      #38bdf8 22%,
      #0284c7 48%,
      #0c4a6e 74%,
      #020617 95%
    );
    box-shadow:
      inset -22px -22px 36px rgba(2, 6, 23, 0.96),
      inset 4px 4px 12px rgba(186, 230, 253, 0.65),
      0 0 32px rgba(56, 189, 248, 0.32);
  }

  .exo-earth-continents {
    position: absolute;
    inset: 0;
    background:
      radial-gradient(circle at 42% 38%, rgba(16, 185, 129, 0.35) 0%, transparent 28%),
      radial-gradient(circle at 28% 62%, rgba(52, 211, 153, 0.25) 0%, transparent 24%);
    filter: blur(3px);
  }

  .exo-earth-clouds {
    position: absolute;
    inset: -10%;
    background:
      radial-gradient(ellipse 60% 18% at 38% 32%, rgba(255, 255, 255, 0.45), transparent 70%),
      radial-gradient(ellipse 50% 14% at 55% 58%, rgba(255, 255, 255, 0.32), transparent 70%);
    transform: rotate(-18deg);
    filter: blur(2px);
  }

  .exo-earth-terminator {
    position: absolute;
    inset: 0;
    border-radius: 50%;
    background: radial-gradient(circle at 26% 26%, transparent 36%, rgba(2, 6, 23, 0.92) 80%);
  }

  /* =========================================================
   * PLANET 4: Crimson Volcanic World (Top-Left-Center)
   * ========================================================= */
  .planet-crimson {
    top: 10%;
    left: 25%;
    width: 64px;
    height: 64px;
    z-index: 2;
  }

  .crimson-sphere {
    position: relative;
    width: 100%;
    height: 100%;
    border-radius: 50%;
    overflow: hidden;
    background: radial-gradient(
      circle at 30% 28%,
      #fecdd3 0%,
      #fb7185 26%,
      #be123c 56%,
      #4c0519 82%,
      #090103 98%
    );
    box-shadow:
      inset -10px -10px 20px rgba(0, 0, 0, 0.95),
      inset 3px 3px 8px rgba(255, 228, 230, 0.55),
      0 0 24px rgba(244, 63, 94, 0.35);
  }

  .crimson-terminator {
    position: absolute;
    inset: 0;
    border-radius: 50%;
    background: radial-gradient(circle at 26% 26%, transparent 38%, rgba(4, 1, 2, 0.92) 82%);
  }

  /* =========================================================
   * LIGHT MODE (CELESTIAL DAYBREAK) PLANETARY SHADING ADAPTATIONS
   * ========================================================= */
  :global(html:not(.dark)) .planet-gas-giant,
  :global(.light) .planet-gas-giant {
    filter: saturate(1.25) contrast(1.1);
  }

  :global(html:not(.dark)) .planet-saturn,
  :global(.light) .planet-saturn {
    filter: saturate(1.25) contrast(1.1);
  }

  :global(html:not(.dark)) .planet-exo-earth,
  :global(.light) .planet-exo-earth {
    filter: saturate(1.3) contrast(1.15);
  }

  :global(html:not(.dark)) .planet-crimson,
  :global(.light) .planet-crimson {
    filter: saturate(1.3) contrast(1.1);
  }

  :global(html:not(.dark)) .gas-giant-sphere,
  :global(.light) .gas-giant-sphere {
    box-shadow:
      inset -38px -38px 65px rgba(30, 27, 75, 0.65),
      inset 8px 8px 24px rgba(255, 255, 255, 0.8),
      0 16px 45px -8px rgba(124, 58, 237, 0.28);
  }

  :global(html:not(.dark)) .gas-giant-terminator,
  :global(.light) .gas-giant-terminator {
    opacity: 0.52;
  }

  :global(html:not(.dark)) .gas-giant-atmosphere,
  :global(.light) .gas-giant-atmosphere {
    opacity: 0.65;
  }

  :global(html:not(.dark)) .gas-giant-moon-orbit,
  :global(.light) .gas-giant-moon-orbit {
    border-color: rgba(99, 102, 241, 0.22);
  }

  :global(html:not(.dark)) .cratered-moon,
  :global(.light) .cratered-moon {
    box-shadow:
      inset -4px -4px 8px rgba(30, 41, 59, 0.6),
      0 2px 10px rgba(99, 102, 241, 0.25);
  }

  :global(html:not(.dark)) .saturn-sphere,
  :global(.light) .saturn-sphere {
    box-shadow:
      inset -22px -20px 38px rgba(67, 20, 7, 0.65),
      inset 5px 5px 14px rgba(255, 237, 213, 0.8),
      0 12px 32px -4px rgba(234, 88, 12, 0.24);
  }

  :global(html:not(.dark)) .saturn-terminator,
  :global(.light) .saturn-terminator {
    opacity: 0.48;
  }

  :global(html:not(.dark)) .saturn-rings-front,
  :global(.light) .saturn-rings-front {
    filter: drop-shadow(0 6px 12px rgba(124, 45, 18, 0.28));
  }

  :global(html:not(.dark)) .exo-earth-sphere,
  :global(.light) .exo-earth-sphere {
    box-shadow:
      inset -18px -18px 28px rgba(8, 47, 73, 0.65),
      inset 4px 4px 12px rgba(255, 255, 255, 0.85),
      0 10px 28px -4px rgba(2, 132, 199, 0.28);
  }

  :global(html:not(.dark)) .exo-earth-terminator,
  :global(.light) .exo-earth-terminator {
    opacity: 0.5;
  }

  :global(html:not(.dark)) .exo-earth-glow,
  :global(.light) .exo-earth-glow {
    opacity: 0.75;
  }

  :global(html:not(.dark)) .crimson-sphere,
  :global(.light) .crimson-sphere {
    box-shadow:
      inset -8px -8px 16px rgba(76, 5, 25, 0.65),
      inset 3px 3px 8px rgba(255, 228, 230, 0.8),
      0 8px 20px -3px rgba(225, 29, 72, 0.24);
  }

  :global(html:not(.dark)) .crimson-terminator,
  :global(.light) .crimson-terminator {
    opacity: 0.52;
  }
</style>
