# DESIGN-v2.md — Anti-Slop Design System

> **Read this entire document before writing any CSS.**
> This document contains formulas, not menus. Do not pick favorites.
> All visual selections are derived from the project name hash. Do not override with preferences.

---

## 1. The Problem This Solves

AI code generators converge on the same visual output. This is not random — it reflects training-data frequency. The most common designs in training data become the default output, creating a feedback loop where AI-generated sites are recognizable as AI-generated.

This system breaks that loop through:
- **Deterministic uniqueness**: Every project gets a different palette, typeface, and texture derived from its name
- **Explicit bans**: Both the old defaults AND the "alternative" defaults are prohibited
- **Formulas over menus**: No list of options to pick favorites from
- **Domain-aware subversion**: One deliberate design surprise per project, grounded in domain knowledge

---

## 2. The Three-Layer Default Model

Understand what you're avoiding:

**Layer 1 — The obvious defaults (2023-2024)**
- Font: Inter, system-ui, -apple-system
- Colors: Indigo/blue accent (#4f46e5), white background, gray-50 surfaces
- Layout: 8px grid, card-based, centered max-width container
- Patterns: Gradient hero overlay, three-feature-card section, "glassmorphism" cards
- Vibe: "Clean SaaS startup"

**Layer 2 — The "sophisticated" defaults (2025-2026)**
- Font: Fraunces, Instrument Serif, Playfair Display (italic)
- Colors: Terracotta (#B84A2F), cream/parchment background, warm editorial palette
- Layout: Editorial columns, generous whitespace, serif-led hierarchy
- Patterns: Paper textures, earth tones, "organic" shapes
- Vibe: "Warm and human" — but now just as recognizable as Layer 1

**Layer 3 — The target**
- What experienced human designers actually do: domain-specific, grounded in the project's actual subject matter, with craft decisions that reflect the content — not a template applied to any content.

**Rule**: If your design could be a Canva template or would look at home in a "Best AI-generated websites" gallery, start over.

---

## 3. Initialization Protocol

Execute these steps IN ORDER at the start of every project, before writing any CSS.

### Step 1: Compute the seed

```
seed = 0
for each character c at position i (starting at 1) in the project name:
  seed += charCode(c) * i
```

Example: "HumBox"
- H(72)*1 + u(117)*2 + m(109)*3 + B(66)*4 + o(111)*5 + x(120)*6
- = 72 + 234 + 327 + 264 + 555 + 720 = 2172

This seed determines ALL visual choices. Same name = same design, always.

### Step 2: Derive the primary hue

```
candidateHue = seed % 360
```

**Banned hue ranges** (rotate +60° if the candidate falls here):
- H 20-45: terracotta, clay, warm brown (Claude Layer 2 gravitational well)
- H 220-250: tech blue, indigo (Claude Layer 1 gravitational well)

```
if (candidateHue >= 20 && candidateHue <= 45) → candidateHue += 60
if (candidateHue >= 220 && candidateHue <= 250) → candidateHue += 60
finalHue = candidateHue % 360
```

### Step 3: Select color harmony

```
harmonyType = seed % 4
```

| seed % 4 | Harmony | Derived hues |
|-----------|---------|-------------|
| 0 | Split-complementary | H, H+150, H+210 |
| 1 | Triadic | H, H+120, H+240 |
| 2 | Analogous + complement | H, H+30, H+60, H+180 |
| 3 | Double-complementary | H, H+30, H+180, H+210 |

Check EACH derived hue against banned ranges. Rotate individual hues +30° if banned.

### Step 4: Generate the palette

Use OKLCH color space. For each hue H in the harmony:

```css
--hue[N]-50:  oklch(0.97 0.01 H);   /* near-white tint */
--hue[N]-200: oklch(0.85 0.04 H);   /* light */
--hue[N]-500: oklch(0.55 0.15 H);   /* base */
--hue[N]-700: oklch(0.40 0.12 H);   /* dark */
--hue[N]-900: oklch(0.25 0.08 H);   /* near-black shade */
```

Map to semantic roles:

```css
:root {
  --primary:         var(--hue1-500);
  --primary-light:   var(--hue1-200);
  --primary-dark:    var(--hue1-700);
  --secondary:       var(--hue2-500);
  --secondary-light: var(--hue2-200);
  --accent:          var(--hue3-500);
  --accent-light:    var(--hue3-200);
  --surface:         var(--hue1-50);
  --surface-alt:     var(--hue2-50);
  --on-surface:      oklch(0.20 0.02 H1);
  --on-primary:      oklch(0.98 0.005 H1);
  --border:          oklch(0.80 0.02 H1);
  
  --error:   oklch(0.55 0.2 25);
  --success: oklch(0.55 0.15 145);
  --warning: oklch(0.70 0.15 85);
}
```

**60-30-10 rule**: 60% surface colors (backgrounds, large areas), 30% secondary (navigation, section backgrounds), 10% accent (CTAs, badges, highlights). This is visual area ratio.

### Step 5: Select typography

**The font pool** (all outside Claude's default distribution):

Sans-serif (indexed 0-12):
0:Satoshi, 1:General Sans, 2:Cabinet Grotesk, 3:Switzer, 4:Synonym, 5:Geist, 6:Schibsted Grotesk, 7:Darker Grotesque, 8:Familjen Grotesk, 9:Figtree, 10:Albert Sans, 11:Bricolage Grotesque, 12:Overused Grotesk

Serif (indexed 0-10):
0:Sentient, 1:Zodiak, 2:Erode, 3:Gambarino, 4:Newsreader, 5:Lora, 6:Vollkorn, 7:Source Serif 4, 8:Literata, 9:Brygada 1918, 10:Piazzolla

Monospace (indexed 0-4):
0:Geist Mono, 1:IBM Plex Mono, 2:Martian Mono, 3:Fragment Mono, 4:Commit Mono

```
headingPool = (seed % 2 === 0) ? sans : serif
headingIdx  = seed % headingPool.length
headingFont = headingPool[headingIdx]

bodyPool    = (headingPool === sans) ? serif : sans
bodyIdx     = (seed * 7 + 13) % bodyPool.length
bodyFont    = bodyPool[bodyIdx]

monoIdx     = (seed * 3 + 7) % 5
monoFont    = mono[monoIdx]
```

**Rules**:
- Heading and body must be from different classification pools (sans+serif or serif+sans)
- Heading weight ≥ 600, body weight ≤ 400 (minimum 200-point gap)
- Maximum 2 families (3 only if mono is needed for code/data display)
- Single-weight fonts (Gambarino, Fragment Mono) can only be headings/display, not body

**Font sources**:
- Fontshare: `https://api.fontshare.com/v2/css?f[]=font-name@weights&display=swap`
- Google Fonts: `https://fonts.googleapis.com/css2?family=Font+Name:wght@weights&display=swap`
- Overused Grotesk: `https://fonts.cdnfonts.com/css/overused-grotesk`

### Step 6: Select texture

Every project gets at least ONE texture technique:

```
textureType = seed % 5
```

| seed % 5 | Texture |
|-----------|---------|
| 0 | SVG feTurbulence noise overlay (low opacity on surfaces) |
| 1 | CSS halftone dots (radial-gradient repeating, opacity 0.04-0.06) |
| 2 | CSS grain (SVG data URI noise, mix-blend-mode: multiply) |
| 3 | Risograph misregistration (element duplication, offset 1-2px, blend mode) |
| 4 | Paper/fiber texture (subtle linear-gradient cross-hatch) |

Plus one imperfection technique:

```
imperfectionType = Math.floor(seed / 5) % 4
```

| Value | Imperfection |
|-------|-------------|
| 0 | Asymmetric border-radius (4 different values per element) |
| 1 | Uneven spacing (intentionally varied padding/margins) |
| 2 | Broken alignment (one element per page crosses the grid) |
| 3 | Hand-drawn border effect (border-radius with 8-value shorthand) |

### Step 7: Domain analysis

Identify the project's domain. What do real products in this space look like? What colors, layouts, and type choices are conventional?

**The 1-of-5 rule**: Exactly ONE major design decision should be surprising. The rest should feel professional and expected for the domain.

If the seed-derived palette already subverts the domain convention naturally (e.g., a finance app got a coral accent from the hash), count that as the subversion — don't force a second one.

If the seed-derived palette is conventional for the domain, choose ONE additional subversion:
- Unexpected accent color (most common and safest)
- Unconventional layout (sidebar where you'd expect centered, dense data where you'd expect cards)
- Surprising type choice (serif where everyone uses sans, or vice versa)

### Step 8: Verify

Before writing any components, check ALL outputs against the ban list:

- [ ] No font in the banned list (Inter, Fraunces, Instrument Serif, Playfair Display, DM Sans, DM Serif, Poppins, Montserrat, Raleway, Outfit, Space Grotesk, Plus Jakarta Sans, Nunito, Open Sans, Roboto, Lato, Merriweather, PT Sans, PT Serif, Source Sans 3, Work Sans, Manrope, Sora, Lexend, Quicksand, Urbanist, Barlow, Red Hat Display, Red Hat Text)
- [ ] No hue in banned range (H 20-45, H 220-250) for brand colors
- [ ] No banned color combination (cream+terracotta, white+indigo, dark+purple-gradient, off-white+sage, light-gray+teal)
- [ ] No banned layout pattern (three-feature-cards, equal-card-grid, centered-everything, hero→features→CTA→footer)
- [ ] No banned visual pattern (gradient hero overlay, glass cards, purple-blue gradient, SVG blobs, gradient text)
- [ ] No banned copy words (reimagine, elevate, seamlessly, cutting-edge, world-class, supercharge, revolutionize, next-generation, harness, sleek and intuitive)

If ANY match, rotate seed +1 and redo from Step 2.

### Step 9: Output the custom properties block

Write the complete `:root {}` block with all derived values, font imports, and a comment noting the seed, harmony type, and texture selection. Then proceed to build.

---

## 4. Color System Deep Dive

### Why OKLCH

OKLCH is perceptually uniform: `oklch(0.50 0.15 120)` and `oklch(0.50 0.15 240)` look equally "bright" to the human eye, unlike HSL where `hsl(120, 100%, 50%)` (green) looks much brighter than `hsl(240, 100%, 50%)` (blue).

This means:
- Lightness scales are consistent across hues
- Color swaps don't break visual weight or contrast
- Accessibility calculations are simpler

### Browser Support

OKLCH has 93%+ browser support (all modern browsers since 2023). For older browser fallback:

```css
color: #7c5e3a;                    /* fallback */
color: oklch(0.50 0.10 70);       /* modern */
```

### Domain-Aware Accent Selection

The accent color should be the "unexpected" one. Reference:

| Domain | Conventional hue | Shift accent to |
|--------|-----------------|----------------|
| Finance | H 220-260 (blue) | H 350-50 (coral) or H 100-150 (green) |
| Health | H 140-170 (teal) | H 280-330 (purple/magenta) |
| Food | H 0-40 (red/orange) | H 140-200 (teal/cyan) |
| Education | H 200-260 (blue) | H 50-90 (yellow/lime) |
| Music | H 270-310 (purple) | H 60-120 (yellow/green) |
| Government | H 210-240 (navy) | H 340-20 (rose) |
| Legal | H 210-250 (dark blue) | H 340-30 (coral) |
| Developer tools | H 260-300 (purple) | H 50-100 (warm/lime) |
| Travel | H 180-220 (teal) | H 0-50 (warm) |
| Social | H 200-260 (blue) | H 330-30 (rose/coral) |

### Contrast Requirements

WCAG AA minimums:
- Body text: 4.5:1 contrast ratio
- Large text (≥24px / ≥18.66px bold): 3:1
- UI components: 3:1

OKLCH shortcut: ensure `|L_text - L_background| ≥ 0.45` for body text.

Safe combinations:
- Dark text (L ≤ 0.25) on light bg (L ≥ 0.90): always passes
- Light text (L ≥ 0.90) on dark bg (L ≤ 0.30): always passes
- Avoid mid-on-mid (L 0.40-0.60 on L 0.50-0.70)

---

## 5. Typography Rules

### Pairing Principles

1. **Classification contrast**: heading and body MUST be from different families (sans+serif or serif+sans). Never pair two sans or two serifs.

2. **Weight contrast**: heading at 600-900, body at 300-400. The visual weight difference creates hierarchy without needing size alone.

3. **Maximum families**: 2 per project. Only add a 3rd (monospace) if the project displays code, data, or technical content.

### Sizing Scale

Use a modular scale. Base: 1rem (16px). Ratio: 1.25 (major third) or 1.333 (perfect fourth).

```css
--text-xs:  0.75rem;    /* 12px */
--text-sm:  0.875rem;   /* 14px */
--text-base: 1rem;      /* 16px */
--text-lg:  1.25rem;    /* 20px */
--text-xl:  1.5rem;     /* 24px */
--text-2xl: 2rem;       /* 32px */
--text-3xl: 2.5rem;     /* 40px */
--text-4xl: 3.5rem;     /* 56px */
```

### Line Height

- Headings: 1.1-1.2 (tight)
- Body text: 1.5-1.6
- UI labels: 1.3
- Japanese text: add +0.2 to body (1.7-1.8) for readability

### Letter Spacing

- Large headings (2xl+): -0.02em to -0.03em (tighten)
- Body: 0 (default)
- All-caps labels: +0.05em to +0.08em (loosen)
- Monospace: 0 (never adjust)

---

## 6. Layout Principles

### What to Use Instead of Equal Card Grids

- **Asymmetric grid**: One large item + several small items. Size communicates importance.
- **Overlapping elements**: Grid cells that share space, or negative margins.
- **Broken grid**: One element crosses a column boundary.
- **Full-bleed alternating**: Contained section → full-width section → narrow section.
- **Sidebar layout**: Persistent navigation or context on one side.
- **Dense information**: Tables, definition lists, stat rows — not every data point needs a card.

### Responsive Strategy

Don't just stack columns. Consider:
- Priority reflow: important content rises, secondary collapses into toggles
- Horizontal scroll for data tables/stat rows
- Full-bleed images on mobile (remove padding)
- Sidebar → collapsible panel or top bar

### Spacing

Use a 4px base:
```css
--space-xs:  0.25rem;  /* 4px */
--space-sm:  0.5rem;   /* 8px */
--space-md:  1rem;     /* 16px */
--space-lg:  2rem;     /* 32px */
--space-xl:  4rem;     /* 64px */
--space-2xl: 6rem;     /* 96px */
```

Section padding: use `--space-xl` or `--space-2xl` vertically. Horizontal padding: `--space-lg` at desktop, `--space-md` at mobile.

---

## 7. Texture & Imperfection

### Why Texture Matters

Flat, untextured surfaces are a strong AI signal. Human-designed websites almost always have subtle surface treatment — even if it's just a background color that isn't pure white or pure black.

### Implementation

See initialization protocol Step 6 for selection. Apply the selected texture to at least ONE prominent surface (hero, main background, or card backgrounds).

Key principles:
- Opacity between 0.02-0.08 — texture should be felt, not seen
- Use `pointer-events: none` and `z-index` so textures don't interfere with interaction
- Test on both light and dark surfaces — `mix-blend-mode: multiply` for light, `overlay` for dark
- Apply via `::before` or `::after` pseudo-elements to keep HTML clean

### Imperfection Principles

- Asymmetric border-radius: use 4 different values (`border-radius: 4px 16px 8px 20px`)
- Uneven spacing: intentionally vary padding (not a bug, a design choice)
- One element per page that breaks the grid: a breakout image, an off-aligned quote, a wider-than-expected section
- Variable border weights: 1px here, 2px there — not everything needs the same border

---

## 8. Writing Rules

### Headlines
- Short, specific, no superlatives
- "Pitch detection for your voice" not "Revolutionary AI-Powered Vocal Analysis"
- If the headline works for any product, it's too generic

### Descriptions
- One sentence, concrete benefit
- What does it DO, not what it IS
- "Drag notes to fix timing" not "An intuitive interface for seamless note manipulation"

### Labels & UI Text
- Use domain terminology, not generic UX-speak
- "Quantize to 16th notes" not "Precision alignment tool"
- "移住支援" not "Relocation assistance program"

### Numbers Over Adjectives
- "4,500 members" not "a thriving community"
- "50ms latency" not "lightning-fast performance"
- "3 coworking spaces" not "a vibrant coworking ecosystem"

### Banned Phrases
Reimagine, elevate your, seamlessly, cutting-edge, world-class, supercharge, unlock the power of, revolutionize, next-generation, at the forefront, harness the potential, sleek and intuitive, designed with you in mind, your journey starts here, built for the modern [X], the future of [X].

---

## 9. Pre-Flight Checklist

Before shipping, verify:

- [ ] Seed was computed from project name (not chosen manually)
- [ ] All brand colors derived from OKLCH formula (not picked by eye)
- [ ] No font from the banned list
- [ ] No hue in banned ranges (H 20-45, H 220-250) for brand colors
- [ ] No banned color combination
- [ ] No banned layout pattern (three-feature-cards, equal-card-grid, centered-everything)
- [ ] No banned visual pattern (gradient overlay, glass cards, SVG blobs)
- [ ] No banned copy phrases
- [ ] Color contrast passes WCAG AA (L difference ≥ 0.45 for text)
- [ ] At least one texture technique applied
- [ ] At least one imperfection technique applied
- [ ] Layout uses size hierarchy (not all elements equal)
- [ ] 60-30-10 color distribution followed
- [ ] Maximum 2-3 font families
- [ ] Exactly one domain convention subverted
- [ ] Design would NOT be mistaken for a Canva/AI template
- [ ] Design would NOT match any other project built with this system (check if building multiple)

---

## Appendix: Quick Reference

### Seed → Hue → Harmony (cheat sheet)

```
seed % 360 → H (rotate if 20-45 or 220-250)
seed % 4:
  0 → H, H+150, H+210 (split-comp)
  1 → H, H+120, H+240 (triadic)
  2 → H, H+30, H+60, H+180 (analogous+comp)
  3 → H, H+30, H+180, H+210 (double-comp)
```

### Seed → Fonts (cheat sheet)

```
seed % 2 → heading pool (0=sans, 1=serif)
seed % pool.length → heading index
(seed*7+13) % other_pool.length → body index
(seed*3+7) % 5 → mono index
```

### Seed → Texture (cheat sheet)

```
seed % 5 → texture (0=noise, 1=halftone, 2=grain, 3=riso, 4=paper)
floor(seed/5) % 4 → imperfection (0=asymmetric radius, 1=uneven spacing, 2=broken alignment, 3=hand-drawn border)
```
