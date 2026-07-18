# GUI quality contract

This contract is normative for every GUI task. Read [DESIGN-v2.md](DESIGN-v2.md) first.

## Product character

The GUI is an operations cockpit. Not a marketing site. Not a wall of equal cards. It must feel calm during normal work and brutally clear during failure.

Desired words:

- Precise.
- Dense without clutter.
- Fast.
- Trustworthy.
- Domain-specific.
- Memorable.

Forbidden result:

- Generic SaaS template.
- Admin theme with changed logo.
- Glass cards and purple gradient.
- Decorative dashboard with fake charts.
- Every number inside a rounded card.
- Desktop layout merely stacked on mobile.

## Deterministic identity

Project name: `Database Accelerator`

Seed calculation includes the space and exact capitalization.

```text
seed = 21188
candidate hue = 21188 % 360 = 308
harmony = 21188 % 4 = 0 = split-complementary
hues = 308, 98, 158
heading = Bricolage Grotesque
body = Lora
data/SQL = IBM Plex Mono
texture = risograph misregistration
imperfection = controlled uneven spacing
```

None of the derived hues enter the banned ranges from `DESIGN-v2.md`.

## Required base tokens

Create browser fallbacks for every OKLCH color during implementation.

```css
:root {
  /* Database Accelerator; seed 21188; split-complementary; riso; uneven spacing. */
  --hue-1-50:  oklch(0.97 0.01 308);
  --hue-1-200: oklch(0.85 0.04 308);
  --hue-1-500: oklch(0.55 0.15 308);
  --hue-1-700: oklch(0.40 0.12 308);
  --hue-1-900: oklch(0.25 0.08 308);

  --hue-2-50:  oklch(0.97 0.01 98);
  --hue-2-200: oklch(0.85 0.04 98);
  --hue-2-500: oklch(0.55 0.15 98);
  --hue-2-700: oklch(0.40 0.12 98);
  --hue-2-900: oklch(0.25 0.08 98);

  --hue-3-50:  oklch(0.97 0.01 158);
  --hue-3-200: oklch(0.85 0.04 158);
  --hue-3-500: oklch(0.55 0.15 158);
  --hue-3-700: oklch(0.40 0.12 158);
  --hue-3-900: oklch(0.25 0.08 158);

  --primary: var(--hue-1-500);
  --primary-light: var(--hue-1-200);
  --primary-dark: var(--hue-1-700);
  --secondary: var(--hue-2-500);
  --secondary-light: var(--hue-2-200);
  --accent: var(--hue-3-500);
  --accent-light: var(--hue-3-200);
  --surface: var(--hue-1-50);
  --surface-alt: var(--hue-2-50);
  --on-surface: oklch(0.20 0.02 308);
  --on-primary: oklch(0.98 0.005 308);
  --border: oklch(0.80 0.02 308);
  --error: oklch(0.55 0.20 25);
  --success: oklch(0.55 0.15 145);
  --warning: oklch(0.70 0.15 85);
}
```

The implementation task derives dark tokens from the same hues. It must not invent a second brand palette.

## Typography use

- Bricolage Grotesque: headings, navigation, buttons, short UI labels.
- Lora: explanations, help, runbooks, empty states, longer prose.
- IBM Plex Mono: SQL, identifiers, connection IDs, timings, counts, tabular numeric data.
- Self-host required weights and subsets. No runtime font CDN.
- Maximum three families.
- Numbers use tabular figures where available.
- Dense tables may use Bricolage or IBM Plex Mono. Never force long prose into monospace.

## Domain-specific layout

Primary layout is asymmetric and operational:

- Persistent navigation at wide widths.
- Global health strip always visible.
- Main operational canvas gets most area.
- Context rail holds alerts, explanations, and recent changes.
- Dense tables and definition rows replace repeated cards.
- One dominant metric or visualization per page. Not six equal KPI cards.
- Dangerous actions stay near affected resource, not in generic menu.

## Signature experience

Build a live `Connection Pressure Map`.

It shows:

```text
logical clients -> waiting work -> active pool -> pinned work -> database
```

It must answer in five seconds:

- Are clients being accepted?
- Where are they waiting?
- Why are connections pinned?
- Is the database at its safe limit?
- What action is safe now?

Animation is restrained and data-driven. Reduced-motion mode removes continuous movement without losing meaning.

## Texture and imperfection controls

- Risograph misregistration appears only on brand mark, selected section marker, or empty-state illustration.
- Maximum offset: 1–2 px.
- Never apply texture to table text, charts, SQL, forms, or status labels.
- Uneven spacing creates hierarchy between major regions.
- Controls inside one component remain mechanically aligned.
- Imperfection must look intentional at every viewport.

## Information architecture

1. Overview.
2. Connections.
3. Workload and query fingerprints.
4. Schema and acceleration.
5. Cache.
6. Atomic operations when enabled.
7. Maintenance and test lab.
8. Configuration.
9. Users and access.
10. Audit and diagnostics.

## Required states

Every view and reusable component supports:

- Loading.
- Empty.
- Fresh healthy data.
- Stale data.
- Partial data.
- Degraded service.
- Permission denied.
- Recoverable error.
- Blocking error.
- High-volume data.
- Long identifiers and values.

No endless spinner. Show work name, elapsed time, and safe cancel when available.

## Responsive contract

Test widths: `360`, `390`, `768`, `1024`, `1280`, `1440`, and `1920` CSS pixels.

- `1440+`: full navigation, operational canvas, context rail.
- `1024–1439`: compact navigation and collapsible context rail.
- `768–1023`: navigation drawer; important health and actions remain first.
- `<768`: top command bar, priority reflow, action sheets, horizontally scrollable data tables.
- Do not remove monitoring facts on small screens. Collapse detail behind explicit disclosure.
- Touch targets at least 44 by 44 CSS pixels.
- Sticky regions must not consume the mobile viewport.
- Tables preserve column headers and row identity while scrolling.

## Interaction contract

- Visible feedback begins within 100 ms of user action.
- Destructive actions show target, effect, and recovery before confirmation.
- Keyboard access reaches every action.
- Focus is always visible.
- Escape closes dismissible layers.
- Deep links preserve page and useful filters.
- URL owns shareable filter state; secrets never enter URL.
- Time, byte, rate, and duration units remain consistent.
- Status never relies on color alone.
- Live updates do not steal focus or reorder rows under pointer without notice.

## Motion contract

- Motion explains state change. No decorative looping.
- Normal transitions: 120–220 ms.
- Large route transitions: maximum 300 ms.
- Connection Pressure Map may use slow flow only when live.
- Honor `prefers-reduced-motion`.
- No layout shift caused by late animation or fonts.

## Accessibility contract

- WCAG AA contrast for text and controls.
- Keyboard-complete operation.
- Semantic landmarks, headings, forms, tables, and dialogs.
- Accessible chart summary and underlying data table.
- Screen-reader announcement for blocking state changes, not every metric tick.
- Zoom to 200% without lost action or clipped content.
- High-contrast and reduced-motion modes tested.

## Performance contract

- Embedded assets. No production CDN dependency.
- Route-level code splitting where useful.
- Initial shell usable within 1.5 seconds on the documented target profile.
- Interaction response within 200 ms for local UI work.
- Cumulative layout shift at or below 0.05 in test profile.
- Long tables use pagination or virtualization.
- Charts downsample safely and expose aggregation.
- Browser memory remains bounded during live updates and route changes.

Exact bundle budgets are frozen in the design-foundation task after framework measurement. They become regression gates.

## Copy contract

- Use database terms. No generic SaaS praise.
- Use numbers over adjectives.
- Explain why acceleration is on or off.
- State dangerous consequences plainly.
- Never use banned phrases from `DESIGN-v2.md`.
- No lorem ipsum in committed UI fixtures.

## Anti-slop release gate

- [ ] Seed and derived identity match this document.
- [ ] No banned font, hue, combination, layout, pattern, or phrase.
- [ ] No equal-card dashboard wall.
- [ ] Connection Pressure Map uses real or deterministic fixture data.
- [ ] Light and dark themes derive from one token system.
- [ ] All required states exist.
- [ ] All responsive widths pass visual review.
- [ ] WCAG AA and keyboard checks pass.
- [ ] Visual regression passes in supported browsers.
- [ ] Performance budgets pass.
- [ ] Reduced motion passes.
- [ ] Human reviewer can identify this product from an unlabeled screenshot.
- [ ] Human reviewer can find database pressure and safe next action within five seconds.
