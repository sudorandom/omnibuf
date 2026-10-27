---
name: "OmniBuf"
description: "Fast, self-describing binary wire format for .proto schemas"
colors:
  signal-orange: "#FF4F18"
  acid-lime: "#D4FF33"
  cobalt-blue: "#1E40AF"
  mint-emerald: "#10B981"
  drafting-paper: "#F4F0E6"
  warm-cream: "#FAF8F2"
  stone-chrome: "#E5E0D5"
  carbon-ink: "#111111"
  pure-white: "#FFFFFF"
typography:
  display:
    fontFamily: "Space Grotesk, sans-serif"
    fontSize: "clamp(2.6rem, 6vw, 5.5rem)"
    fontWeight: 700
    lineHeight: 0.94
    letterSpacing: "-0.025em"
  display-serif:
    fontFamily: "Instrument Serif, serif"
    fontSize: "clamp(3.1rem, 7vw, 6.2rem)"
    fontWeight: 400
    lineHeight: 0.94
  headline:
    fontFamily: "Space Grotesk, sans-serif"
    fontSize: "clamp(1.85rem, 4vw, 3rem)"
    fontWeight: 700
    lineHeight: 1.1
    letterSpacing: "-0.02em"
  title:
    fontFamily: "IBM Plex Mono, monospace"
    fontSize: "1.05rem"
    fontWeight: 700
    lineHeight: 1.35
  body:
    fontFamily: "Space Grotesk, sans-serif"
    fontSize: "1.0625rem"
    fontWeight: 500
    lineHeight: 1.75
  label:
    fontFamily: "IBM Plex Mono, monospace"
    fontSize: "0.875rem"
    fontWeight: 700
    lineHeight: 1.4
rounded:
  xs: "3px"
  sm: "4px"
  md: "6px"
  lg: "8px"
spacing:
  xs: "8px"
  sm: "12px"
  md: "16px"
  lg: "24px"
  xl: "32px"
  2xl: "48px"
components:
  button-primary:
    backgroundColor: "{colors.signal-orange}"
    textColor: "{colors.pure-white}"
    typography: "{typography.label}"
    rounded: "{rounded.md}"
    padding: "12px 20px"
  button-acid:
    backgroundColor: "{colors.acid-lime}"
    textColor: "{colors.carbon-ink}"
    typography: "{typography.label}"
    rounded: "{rounded.md}"
    padding: "12px 20px"
  button-secondary:
    backgroundColor: "{colors.warm-cream}"
    textColor: "{colors.carbon-ink}"
    typography: "{typography.label}"
    rounded: "{rounded.md}"
    padding: "12px 20px"
  button-ink:
    backgroundColor: "{colors.carbon-ink}"
    textColor: "{colors.drafting-paper}"
    typography: "{typography.label}"
    rounded: "{rounded.md}"
    padding: "12px 20px"
  card-window:
    backgroundColor: "{colors.warm-cream}"
    textColor: "{colors.carbon-ink}"
    rounded: "{rounded.md}"
    padding: "24px"
  nav-pill-active:
    backgroundColor: "{colors.acid-lime}"
    textColor: "{colors.carbon-ink}"
    typography: "{typography.label}"
    rounded: "{rounded.sm}"
    padding: "4px 10px"
---

# Design System: OmniBuf

## Overview

**Creative North Star: "The Classic Workstation Manual"**

OmniBuf's visual identity combines the clarity of a printed Swiss technical manual with classic 1980s Macintosh workstation window chrome and byte-level engineering blueprints. Surfaces rest on a warm drafting-paper canvas overlaid with a faint 32px technical grid, giving every diagram, benchmark chart, and wire-format breakdown the feel of an interactive hardware reference sheet.

Instead of pretending to be an abstract cloud service, the interface treats data structures as physical objects you can inspect and measure. Containers use crisp 2px ink borders, striped classic OS title bars, and hard zero-blur offset shadows. Geometric grotesque headlines pair with italic editorial serif counterpoints and dense monospace byte tables so engineers can scan high-level claims and immediately drill down into exact bit layouts.

The system explicitly rejects neon SaaS gradients, frosted glassmorphism, diffuse drop shadows, and sterile corporate documentation templates. Every visual element is tactile, high-contrast, and mechanical, with an optional "Night Workstation Manual" dark theme (`html.dark`) that maps the paper canvas to deep slate (`#0F1115`) while preserving crisp borders, offset shadows, and Signal Orange / Acid Lime accents.

**Key Characteristics:**
- Warm drafting-paper background with a 32px technical coordinate grid (`rgba(17, 17, 17, 0.05)` lines).
- Classic OS window chrome featuring 2px solid ink borders, horizontal title-bar stripes, and hard offset shadows.
- Tri-font editorial hierarchy pairing uppercase geometric sans (`Space Grotesk`), italic display serif (`Instrument Serif`), and technical monospace (`IBM Plex Mono`).
- High-visibility industrial signal accents (`Signal Orange` and `Acid Lime`) reserved for primary actions, active states, and OmniBuf benchmark bars.

## Colors

The palette contrasts warm, archival paper neutrals against pitch-black printer's ink and two high-visibility industrial signal accents.

### Primary
- **Signal Orange** (`#FF4F18`): Used for primary CTA buttons, italic serif headline counterpoints, OmniBuf's primary data series in benchmark charts, and key architectural callout labels.

### Secondary
- **Acid Lime** (`#D4FF33`): Used for active navigation pills, secondary CTA buttons (`Benchmarks`), highlighted OmniBuf benchmark rows (at 30% opacity over cream), link hover highlights, and high-contrast status badges on dark ink surfaces.

### Tertiary
- **Cobalt Blue** (`#1E40AF`): Used for structural byte-diagram accents, secondary protocol comparisons, and wire-layout field annotations.
- **Mint Emerald** (`#10B981`): Used for positive delta indicators, zero-allocation badges, and compact payload verifications.

### Neutral
- **Drafting Paper** (`#F4F0E6`): Root page background canvas across all routes, overlaid with the 32px coordinate grid.
- **Warm Cream** (`#FAF8F2`): Primary surface fill for `.os-window` containers, callout boxes, sticky navigation bars, and cards.
- **Stone Chrome** (`#E5E0D5`): Classic OS window title-bar fill, table header background, and unpopulated benchmark bar tracks.
- **Carbon Ink** (`#111111`): Primary typography, 2px structural borders, hard offset shadows, code block backgrounds, and inverted stat callout cards.
- **Pure White** (`#FFFFFF`): Inner table cells, non-highlighted benchmark rows, and high-contrast diagram interiors.

### Named Rules
**The Two-Signal Rule.** Signal Orange (`#FF4F18`) and Acid Lime (`#D4FF33`) are the only high-chroma focal colors on the canvas; never introduce competing gradients or decorative pastel fills.
**The Ink-Framed Accent Rule.** Every colored surface, badge, or benchmark bar must be enclosed by a solid Carbon Ink (`#111111`) border so bright accents never bleed directly into the paper background.

## Typography

**Display Font:** Space Grotesk (with system sans-serif fallback)
**Accent Display Font:** Instrument Serif Italic (with Georgia, serif fallback)
**Body Font:** Space Grotesk (with system sans-serif fallback)
**Label/Mono Font:** IBM Plex Mono (with monospace fallback)

**Character:** Bold, tightly tracked uppercase `Space Grotesk` establishes an assertive engineering voice, while `Instrument Serif Italic` in Signal Orange injects human editorial warmth into section hooks, and `IBM Plex Mono` grounds every claim in byte-level precision.

### Hierarchy
- **Display** (`700` weight, `clamp(2.6rem, 6vw, 5.5rem)`, `0.94` line-height, uppercase): Hero and page-level statements, paired with `Instrument Serif Italic` (`400` weight, `clamp(3.1rem, 7vw, 6.2rem)`, normal case) for the second line counterpoint.
- **Headline** (`700` weight, `clamp(1.85rem, 4vw, 3rem)`, `1.1` line-height, uppercase): Major section headers (`h2`), often ending with an italic serif accent word.
- **Title** (`700` weight, `1.05rem` to `1.4rem`, `1.35` line-height): Card titles, `.os-titlebar` window labels (uppercase `IBM Plex Mono`), and specification subheadings (`h3`, `h4`).
- **Body** (`500` weight, `1.0625rem` / `17px`, `1.75` line-height, max width `65ch` to `75ch` in prose): Technical explanations, specification prose, and architectural summaries.
- **Label** (`700` weight, `0.875rem` / `14px`, `1.4` line-height, `IBM Plex Mono`): Buttons, navigation links, benchmark axis labels, byte offsets, and wire-type tags.

### Named Rules
**The Serif Counterpoint Rule.** `Instrument Serif` is used exclusively in italic style as a contrasting phrase inside display or section headlines, never for body copy or UI controls.
**The Monospace Data Rule.** All buttons, navigation bars, window title bars, benchmark rows, byte offsets, and wire-type identifiers must use `IBM Plex Mono`.

## Layout

Pages center content within a `1280px` (`max-w-7xl`) container with responsive horizontal padding (`16px` mobile, `24px` tablet, `32px` desktop). Major page sections stack vertically with `48px` to `64px` (`py-12 sm:py-16`) vertical rhythm and are separated by `2px` solid dividers (`border-t-2 border-ink/15` or solid `#111111`).

Multi-card layouts for pure data/charts use 2-column (`lg:grid-cols-2`) and 3-column (`sm:grid-cols-3`) CSS grids with `24px` (`gap-6`) gutters. However, **anything containing prose that we want users to read must occupy its own entire horizontal ledger row** (`border-t-2 border-ink divide-y-2 divide-ink/20`, either full-width like `FAQ.astro` or a 12-column row pairing left-hand prose with a right-hand code/visual artifact like `Motivation.astro`, `Capabilities.astro`, and `Quickstart.astro`). Never cram readable prose into side-by-side half-width columns. The background canvas carries a persistent `32px x 32px` orthogonal line grid (`1px` lines at `5%` ink opacity) that visually anchors floating window containers. Technical diagrams use dedicated horizontal SVG layouts on desktop (`hidden md:block`) and vertical stacked SVG layouts on mobile (`block md:hidden`) so labels remain legible at every viewport width.

## Elevation & Depth

Depth is strictly structural and mechanical, achieved through solid, zero-blur Carbon Ink (`#111111`) offset shadows that treat every card and button as a physical cut-out layered over the drafting grid. There is zero ambient blur anywhere in the system.

### Shadow Vocabulary
- **Nav Pill / Inline Anchor Shadow** (`box-shadow: 2px 2px 0px #111111`): Applied to active navigation pills (`.nav-link[data-active='true']`), header GitHub buttons (`2px 2px 0px #FF4F18`), and hovered spec heading anchors.
- **Interactive Control Shadow** (`box-shadow: 4px 4px 0px #111111`): Default resting shadow for `.os-btn` buttons and hero summary callout boxes. Expands to `6px 6px 0px #111111` with `translate(-2px, -2px)` on hover, and collapses to `1px 1px 0px #111111` with `translate(2px, 2px)` on active press.
- **Standard Window Shadow** (`box-shadow: 6px 6px 0px #111111`): Applied to `.os-window` cards, benchmark panels, and diagram frames.
- **Featured Window Shadow** (`box-shadow: 10px 10px 0px #111111`): Applied to `.os-window-lg` primary specification containers and flagship interactive showcases.

### Named Rules
**The Zero-Blur Rule.** Every `box-shadow` in the system must have a `0px` blur radius and `0px` spread radius; soft or feathered shadows are forbidden.
**The Mechanical Press Rule.** Interactive buttons must physically translate toward the viewer on hover (`-2px, -2px`) and press into their shadow on click (`+2px, +2px`) over a crisp `120ms ease` transition.

## Shapes

Corners are crisp and slightly blunted rather than pill-shaped or razor-sharp: `6px` (`rounded-md`) for standard `.os-window` containers, code blocks, and buttons; `8px` (`rounded-lg`) for large `.os-window-lg` frames; `4px` (`rounded`) for navigation pills and inner benchmark rows; and `3px` for inline code tags. Every container and interactive control is bounded by a `2px solid #111111` stroke (`1.5px` on compact pills), creating a comic-blueprint line weight that holds together complex technical diagrams. Slightly rotated inline word badges (`-rotate-1`) provide occasional kinetic contrast against the strict orthogonal grid.

## Components

Components feel tactile and mechanical: classic OS window panels with striped title bars, and springy physical buttons that lift on hover and press into the surface on active.

### Buttons (`.os-btn`)
- **Shape:** Slightly rounded corners (`6px` radius) with a `2px solid #111111` border.
- **Primary:** Signal Orange background (`#FF4F18`), Pure White text (`#FFFFFF`), `IBM Plex Mono` bold (`14px` to `16px`), `12px 20px` padding, resting `4px 4px 0px #111111` shadow.
- **Hover / Focus:** Translates `(-2px, -2px)` with shadow expanding to `6px 6px 0px #111111` over `0.12s ease`. On `:active`, translates `(2px, 2px)` with shadow collapsing to `1px 1px 0px #111111`.
- **Secondary / Acid / Ink Variants:** Acid Lime (`#D4FF33` bg, `#111111` text), Warm Cream (`#FAF8F2` bg, `#111111` text), and Carbon Ink (`#111111` bg, `#F4F0E6` text) share identical geometry and press physics.

### Cards / Containers (`.os-window`)
- **Corner Style:** `6px` border radius (`8px` for `.os-window-lg`).
- **Background:** Warm Cream (`#FAF8F2`), or inverted Carbon Ink (`#111111`) with Drafting Paper (`#F4F0E6`) text for primary stat callouts.
- **Shadow Strategy:** Hard offset shadow (`6px 6px 0px #111111`, or `10px 10px 0px #111111` for `.os-window-lg`).
- **Border:** `2px solid #111111`, often paired with a top `.os-titlebar` (`background: #E5E0D5`, `border-bottom: 2px solid #111111`, `padding: 8px 14px`) containing a bold uppercase monospace title and horizontal `.os-stripes` (`repeating-linear-gradient` of `2px` ink lines at `25%` opacity).
- **Internal Padding:** `16px` on mobile, `24px` (`p-4 sm:p-6`) on desktop.

### Navigation
- **Style:** Sticky top bar (`height: 56px`, `background: #FAF8F2`, `border-bottom: 2px solid #111111`, `IBM Plex Mono`).
- **Brand Mark:** Carbon Ink badge (`#111111` bg, `#F4F0E6` text, `4px` radius) with a `10px x 10px` Acid Lime (`#D4FF33`) square indicator.
- **Links (`.nav-link`):** `4px 10px` padding, `4px` radius. Hover applies a subtle ink wash (`rgba(17, 17, 17, 0.06)`). Active page (`data-active="true"`) locks in Acid Lime (`#D4FF33`), `700` weight, `1.5px solid #111111` border, and `2px 2px 0px #111111` hard shadow.

### Inline Documentation Links (`.doc-link`)
- **Style:** `1.5px` underline with `3px` offset, `600` weight.
- **Hover:** Highlights background in Acid Lime (`#D4FF33`) with Carbon Ink (`#111111`) text.

### Benchmark Comparison Rows
- **Style:** 12-column monospace grid row (`padding: 8px 12px`, `4px` radius) containing the codec name, a bordered horizontal bar track (`background: #E5E0D5`, `border: 1px solid #111111`, `height: 20px`), and right-aligned metric value.
- **Highlighted State (OmniBuf):** Highlighted with `rgba(212, 255, 51, 0.3)` background, `2px solid #111111` border, and Signal Orange (`#FF4F18`) bar fill.

## Do's and Don'ts

### Do:
- **Do** reserve `.os-window` and `.os-titlebar` with `.os-stripes` sparingly for flagship interactive diagrams, side-by-side code comparison frames, and benchmark chart panels; use architectural horizontal ledger rows (`border-t-2 border-ink divide-y-2 divide-ink/20`) for capability lists, motivation points, and FAQs so section headings stand out cleanly.
- **Do** frame interactive buttons and callout boxes with a `2px solid #111111` border and a hard zero-blur offset shadow (`4px`, `6px`, or `10px`).
- **Do** pair uppercase bold `Space Grotesk` with a normal-case `Instrument Serif Italic` phrase in Signal Orange (`#FF4F18`) for major section headings.
- **Do** give every section item with readable prose its own entire horizontal row (`border-t-2 border-ink divide-y-2 divide-ink/20`) rather than splitting prose into multi-column panels.
- **Do** use `buf` (`buf.gen.yaml` and `buf generate`) rather than raw `protoc` CLI flags in documentation, onboarding snippets, and code generation examples.
- **Do** provide dedicated desktop (horizontal) and mobile (vertical stacked) SVG layouts for complex architectural and wire-format diagrams so text never shrinks below legibility.

### Don't:
- **Don't** place readable prose inside half-width side-by-side columns; reserve multi-column grids strictly for data tables, stat strips, and chart comparisons.
- **Don't** wrap prose-heavy lists or FAQ items in `.os-window` and `.os-titlebar` chrome; overusing window headers detracts from panel titles.
- **Don't** use blurred drop shadows (`box-shadow` with blur > `0px`), glassmorphism (`backdrop-filter: blur`), or soft neumorphic gradients anywhere in the interface.
- **Don't** use pill-shaped (`border-radius: 9999px`) buttons or containers; keep radii between `3px` and `8px`.
- **Don't** set body paragraphs or UI controls in `Instrument Serif`; reserve the serif strictly for display headline counterpoints.
- **Don't** use em-dashes or generic AI filler copy inside component labels, callout cards, or technical explanations.
