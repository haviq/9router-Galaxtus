---
# gstack: design-md-format=spec
name: 9router Galactus
description: Precision cosmic obsidian instrument with zero-gradient surfaces, physical specular hairlines, and liquid spring layout-id indicator flows.
colors:
  primary: "#e56a4a"
  on-primary: "#ffffff"
  surface: "#0d0f17"
  text: "#f4f5f8"
  text-muted: "#8e95a8"
  accent: "#e56a4a"
  success: "#10b981"
  warning: "#f59e0b"
  error: "#ef4444"
typography:
  display:
    fontFamily: "Inter"
    fontWeight: 600
    fontSize: "1.25rem"
    letterSpacing: "-0.02em"
  body:
    fontFamily: "Inter"
    fontSize: "0.875rem"
    lineHeight: 1.5
  label:
    fontFamily: "Inter"
    fontSize: "0.75rem"
    letterSpacing: "0.04em"
  mono:
    fontFamily: "JetBrains Mono"
    fontFeature: "tnum"
rounded:
  sm: "8px"
  md: "13px"
  lg: "22px"
  full: "9999px"
spacing:
  xs: "4px"
  sm: "8px"
  md: "16px"
  lg: "24px"
  xl: "32px"
  2xl: "48px"
components:
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.on-primary}"
    rounded: "{rounded.md}"
  button-primary-hover:
    backgroundColor: "#d45a3b"
  input:
    borderColor: "{colors.text-muted}"
    rounded: "{rounded.md}"
  card:
    backgroundColor: "{colors.surface}"
    rounded: "{rounded.lg}"
  nav-link:
    textColor: "{colors.text}"
---

# 9router Galactus Design System

## Overview

**Creative North Star:** Precision cosmic instrument meets iOS 18 / Linear tactile hardware — zero rainbow gradients, zero glowing neon slop, pure smoked obsidian glass with physical liquid-spring connected pill transitions (`layoutId`-style flow).
**Product context:** High-throughput multi-provider AI gateway & stealth model remasking engine for engineers and operators.
**Mode per surface:** Operate (dense telemetry tables, provider pools, API key policies, real-time logs).

## Colors

**Strategy:** Restrained — Solid deep cosmic obsidian (`#040509` void, `rgba(13, 15, 23, 0.76)` smoked glass surface), crisp neutral borders (`rgba(255, 255, 255, 0.085)`), and a single solid terracotta accent (`#e56a4a`) without multi-color gradients.
**Anti-Slop Enforcement:**
- Zero purple/violet/pink/cyan gradient washes (`linear-gradient` color blends on cards, nav items, or buttons are strictly prohibited).
- Zero neon outer glows (`0 0 24px` colored halos).

## Shapes & iOS Geometry

- **Cards (`.bg-surface`):** `22px` continuous iOS squircle radius, solid translucent dark obsidian (`rgba(13, 15, 23, 0.76)`), `backdrop-filter: blur(28px)`, `1px` top specular hairline (`inset 0 1px 0 rgba(255, 255, 255, 0.14)`).
- **Controls & Segmented Pills:** `13px` inner radius (`outer 22px - 9px padding = 13px`).

## Motion & Connected Button Flow

- **Approach:** Intentional liquid-spring physics (`motion` + `GSAP`).
- **The Authored Moment — Connected Sliding Pill Flow:** Switching active navigation items in the Sidebar or switching filter tabs/segmented controls anywhere in the app does NOT abruptly toggle background classes. Instead, a single shared indicator pill glides, stretches, and morphs smoothly from the previous button's bounding box to the newly clicked button (`cubic-bezier(0.22, 1, 0.36, 1)` + spring velocity stretch).

## Decisions Log
| Date | Decision | Rationale |
|------|----------|-----------|
| 2026-10-31 | Eliminate all color gradients & AI slop; implement connected sliding pill flow | User requested `/design-consultation` to strip all gradient slop and add smooth connected flow animations between buttons. |

