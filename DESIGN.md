---
name: "937Sub B Console"
description: "A light console organized as collection ledgers, analytical reports, and configuration documents."
colors:
  console-bg: "#fff"
  console-surface: "#fff"
  console-nav: "#f6f7fa"
  console-line: "#e3e8ef"
  console-ink: "#263449"
  console-muted: "#64748b"
  console-accent: "#635bce"
  console-accent-hover: "#5146b5"
  console-selected: "#f5f3ff"
  dark-console-bg: "#111720"
  dark-console-surface: "#1a2330"
  dark-console-nav: "#151d28"
  dark-console-line: "#344154"
  dark-console-ink: "#edf2f8"
  dark-console-muted: "#b0bfd2"
  dark-console-accent: "#c4b5fd"
  dark-console-selected: "#302a4c"
  control-border: "#e5e7eb"
  control-ink: "#374151"
  control-hover: "#f9fafb"
  field-ink: "#111827"
  dark-control-bg: "#1e293b"
  dark-control-border: "#475569"
  dark-control-ink: "#e5e7eb"
  dark-field-ink: "#f3f4f6"
  badge-neutral-bg: "#f3f4f6"
  badge-neutral-ink: "#374151"
  success-bg: "#ecfdf5"
  success-ink: "#047857"
  warning-bg: "#fffbeb"
  warning-ink: "#b45309"
  danger-bg: "#fef2f2"
  danger-ink: "#b91c1c"
  danger-action: "#dc2626"
  dark-success-bg: "rgb(6 78 59 / 30%)"
  dark-success-ink: "#34d399"
  dark-warning-bg: "rgb(120 53 15 / 30%)"
  dark-warning-ink: "#fbbf24"
  dark-danger-bg: "rgb(127 29 29 / 30%)"
  dark-danger-ink: "#f87171"
  chart-input: "#3562ce"
  chart-output: "#183d8b"
  chart-cache-read: "#62b7ad"
  chart-cache-write: "#c28a42"
  chart-cache-rate: "#27857a"
  chart-standard: "#8b97ac"
  chart-label: "#536175"
  chart-grid: "#edf0f4"
  dark-chart-label: "#bec9d9"
  dark-chart-grid: "#303e50"
typography:
  body:
    fontFamily: "system-ui, -apple-system, BlinkMacSystemFont, Segoe UI, Roboto, Helvetica Neue, Arial, PingFang SC, Hiragino Sans GB, Microsoft YaHei, sans-serif"
    fontSize: "14px"
    lineHeight: "20px"
  mono:
    fontFamily: "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace"
  headline:
    fontSize: "24px"
    fontWeight: 650
    lineHeight: 1.3
    letterSpacing: "-0.025em"
  utility-title:
    fontSize: "13px"
    fontWeight: 500
  section-title:
    fontSize: "15px"
    fontWeight: 600
  metric:
    fontSize: "30px"
    fontWeight: 650
    lineHeight: 1.2
    letterSpacing: "-0.03em"
  navigation:
    fontSize: "13px"
  label:
    fontSize: "12px"
  supporting:
    fontSize: "11px"
  auth-headline:
    fontSize: "40px"
    fontWeight: 600
    lineHeight: 1.35
    letterSpacing: "-0.03em"
rounded:
  density-choice: "4px"
  compact: "6px"
  navigation: "7px"
  control: "8px"
  brand-mark: "9px"
  panel: "12px"
spacing:
  small: "8px"
  control: "12px"
  regular: "16px"
  section: "20px"
  panel: "24px"
  document-section: "28px"
  report-gap: "32px"
  workspace-block: "32px"
  workspace-inline: "40px"
  workspace-bottom: "48px"
components:
  button-primary:
    backgroundColor: "{colors.console-accent}"
    textColor: "{colors.console-surface}"
    rounded: "{rounded.control}"
    padding: "8px 12px"
    typography: "{typography.body}"
  button-primary-hover:
    backgroundColor: "{colors.console-accent-hover}"
  button-secondary:
    backgroundColor: "{colors.console-surface}"
    textColor: "{colors.control-ink}"
    rounded: "{rounded.control}"
    padding: "8px 12px"
  button-danger:
    backgroundColor: "{colors.danger-action}"
    textColor: "{colors.console-surface}"
    rounded: "{rounded.control}"
    padding: "8px 12px"
  input:
    backgroundColor: "{colors.console-surface}"
    textColor: "{colors.field-ink}"
    rounded: "{rounded.control}"
    padding: "8px 12px"
    typography: "{typography.body}"
  navigation:
    textColor: "{colors.console-muted}"
    rounded: "{rounded.navigation}"
    padding: "8px 10px"
    typography: "{typography.navigation}"
  navigation-active:
    backgroundColor: "{colors.console-selected}"
    textColor: "{colors.console-accent}"
  navigation-search:
    backgroundColor: "{colors.console-surface}"
    textColor: "{colors.console-ink}"
    rounded: "{rounded.navigation}"
    padding: "8px 10px"
  badge-neutral:
    backgroundColor: "{colors.badge-neutral-bg}"
    textColor: "{colors.badge-neutral-ink}"
    rounded: "{rounded.compact}"
    padding: "2px 8px"
    typography: "{typography.label}"
  card:
    backgroundColor: "{colors.console-surface}"
    rounded: "{rounded.panel}"
  query-panel:
    backgroundColor: "{colors.console-surface}"
    padding: "16px 0"
  collection-ledger:
    backgroundColor: "{colors.console-surface}"
  module-heading:
    textColor: "{colors.console-ink}"
    rounded: "{rounded.navigation}"
    padding: "10px"
  configuration-section:
    padding: "0 0 28px"
  settings-category:
    rounded: "{rounded.compact}"
    padding: "8px 12px"
    textColor: "{colors.console-muted}"
  order-review:
    backgroundColor: "{colors.console-surface}"
    rounded: "{rounded.panel}"
    padding: "24px"
---

# Design System: 937Sub B Console

## Overview

**Creative North Star: "The Operations Workbench"**

The B console is a light, professional workspace for API traffic, charges, resources, and account settings. The chosen reference is Stripe: light surfaces, clear hierarchy, and restrained blue-violet accents. A pale module sidebar and quiet breadcrumb header frame a white working surface. Collections read as ledgers, analytics as reports, and configuration as a document.

Collections use on-demand horizontal filters above records. Reports separate primary analysis from content-sized rankings. Settings use a directory and flat divided sections with headings above fields. Identity uses flat metadata; purchasing separates editable selection from order review. Stored dark theme, configured branding, permissions, feature flags, and existing financial meaning remain authoritative.

**Key Characteristics:**

- One light module sidebar with workspace switching, permission-aware search, and collapse.
- A white working surface, quiet dividers, and restrained violet actions and selections.
- On-demand horizontal filters, unboxed collection ledgers, and explicit record counts.
- Flat configuration sections, identity metadata, and responsive checkout columns.
- Metric-switching reports, content-sized rankings, and explicit numerical values.

This document records revision-5 source and supersedes the rejected revision-4 documentation. Evidence: `frontend/tailwind.config.js`, `frontend/src/style.css`, `frontend/src/styles/console.css`, the later `workbench.css` composition layer, and layout/table/version/select/payment/chart/settings/subscription components. Direction is recorded in `.impeccable/b-console.md`; `PRODUCT.md` retains the confirmed Stripe reference. Local visual evidence is `.impeccable/screenshots/revision5/`, with scope and exclusions recorded in `.impeccable/verification-revision5.md`. Preview data is synthetic. This specification establishes neither all-page visual certification nor deployment or payment completion.

## Colors

Cool white and gray surfaces carry the interface; violet identifies actions and selections. Frontmatter records the exact shared light/dark values. `console-accent` resolves Tailwind primary 600 in light mode and primary 300 in dark mode; filled primary buttons continue to use primary 600 and white text in both themes.

### Primary

- **Action Violet** is the primary action fill, light-theme active navigation text, and focus.
- **Deep Action Violet** is the filled primary-button hover.
- **Selection Wash** marks selected destinations and controls.
- **Night Violet** carries dark-theme focus/selection, not the filled primary-button background.

### Neutral

- **Working White** and **Panel White** share the light canvas; **Navigation Gray** distinguishes the sidebar and quiet table headings. Dividers, spacing, and task structure establish hierarchy without enclosing every section in a card.
- **Fine Divider**, **Slate Ink**, and **Muted Slate** establish boundaries and information hierarchy. Placeholders use the theme's muted text at full opacity.
- Dark-console tokens preserve these roles under the stored theme.
- Shared inputs and secondary buttons retain their Tailwind gray text/borders and slate dark backgrounds, represented by the separate control tokens.

### Semantic and Analytical Colors

Badges use quiet emerald success, amber warning, and red danger pairs, with translucent dark fills. Destructive buttons use red 600, hovering to red 700. Neutral badges use gray 100/700 in light mode and slate 700/300 in dark mode. Provider marks and entitlement/status indicators retain their meanings.

Trend charts use blue input/requests/actual-cost, navy output, teal cache reads/rate, amber cache writes, and a gray dashed standard-cost line. Distribution charts retain muted component-specific categorical palettes. Chart-table actual cost remains green 600 (dark green 400), account cost orange 500 (dark orange 400), and standard cost gray 600 (dark gray 400). Labels distinguish these measures as well as color. Analytical colors do not define additional action palettes.

**The Action Rule.** Use the shared violet primary action for console purchase and renewal; retain semantic colors for their existing data and status meanings.

## Typography

Use the system sans stack with Chinese fallbacks and the existing monospace stack for code fields. No custom font is required. Console numbers use tabular figures.

Page headings use 24px/650 and reduce to 22px below 768px; dashboard overview line-height is 1.35, while document/collection headings use 1.3. The 13px/500 utility title names the workspace without duplicating the page title. Section headings use 15px/600; analytical headings use 650. Dashboard metrics use 30px/650, reducing to 26px on mobile; finance strips use 26px and chart summaries 25px.

Shared controls and comfortable table cells use 14px/20px. Compact table rows use 13px; navigation and descriptions also use 13px. Labels are 12px and supporting units, table headings, and group labels 11px. Authentication headings use 40px, reducing to 26px on mobile. Document descriptions have a 70ch maximum; this is not a table/form width rule.

**The Data Rule.** Preserve existing precision, units, currency distinctions, and labels when changing presentation.

## Layout

At 1024px and above, the sidebar is 232px wide or 68px collapsed and spans `100dvh`. Content offsets by that width. The sticky breadcrumb header is 60px high at top zero. Main content has a 1760px maximum width, centered within its workspace, with 32px top, 40px horizontal, and 48px bottom padding. Padding becomes 24px below 1280px and 20px/16px below 1024px. Below 1024px the sidebar is a dismissible drawer and the content offset disappears. There is no separate horizontal business bar. Breadcrumbs are hidden below 768px.

Collections start with title/actions, then a compact toolbar combining the filter toggle, matching/current-page counts, supplied current-page status counts, and density. Filters start closed. Opening them inserts full-width horizontal fields above the unboxed ledger; `v-show` preserves query state. Fields wrap with 12px gaps; shared search/field widths are 260px/170px, and account filters have a 160px minimum. The filter dock has 16px vertical padding and no fixed height cap. Tables preserve overflow containers and native sticky columns without imposing a viewport-derived minimum height on the collection. Mobile uses document flow and full-width query fields. Compact density persists in `b-console-table-compact`, defaults on, and uses 12px vertical cell padding with 13px type; comfortable rows use 14px type. Density, current-page count, and status counts hide below 768px; matching records remain available.

Settings use a 164px category directory and flexible document with a 24px gap. The directory sticks at 84px. Sections are transparent and square, separated by a bottom divider, 28px bottom padding, and 28px bottom margin. Headings sit above fields with 20px bottom spacing. Below 1024px the directory is 160px with a 20px gap and 80px sticky offset. Below 768px it becomes horizontal scrolling categories. The save bar sticks to the bottom with a top border and 14px vertical padding.

Identity uses a bordered document, three-column flat metadata, divided sections, and two-column editors. An optional support panel occupies a secondary column; absent that panel, identity fills the width. Profile columns stack below 1024px; metadata/editors stack below 768px, with 20px mobile section padding.

Checkout uses a flexible main column, a 320px review column, and a 24px gap. Below 1280px review becomes `minmax(280px,.8fr)`; below 1024px both stack and review stops sticking. Desktop review sticks at 84px. Plan rows contain identity/price, quota, and action regions followed by features; they become two columns below 1024px and vertical below 768px. Subscription quota columns stack below 1280px and the entire entitlement row stacks below 768px.

Dashboard summaries group volume, finance, and resources in a divided strip; simple mode omits finance. Analysis uses a 1.8:1 plot/distribution split with a 300px minimum ranking column and 32px gap. Payment analysis uses the same ratio with a 320px minimum ranking column: revenue trend and content-sized payment distribution share the left column, while per-currency user rankings occupy the right. Both stack below 1280px, replacing the vertical divider with a horizontal one. At intermediate widths model bars and their table share two columns, then stack below 768px. Primary plots are 280px tall, reducing to 250px on mobile. Summary columns stack on mobile. Operations monitoring retains separate diagnosis/vitals, traffic/capacity, and latency/error regions with responsive stacking.

Authentication uses a 0.85:1.15 split, minimum 320px identity column, and maximum 400px form. Its identity surface is navigation white. Below 768px it becomes a compact top band followed by the form. Public pages, setup, callbacks, and provider-owned payment fields retain specialized flows.

## Elevation & Depth

Shared primary/secondary buttons and cards are flat, using tones and fine borders. Chart toggles and dropdowns can retain small local shadows. Specialized legacy button variants are not the shared console action standard.

Dialogs use an unblurred overlay (`rgb(19 30 48 / 32%)`) and shadow (`0 18px 60px rgb(10 24 45 / 16%)`). Opt-in editor sheets align right, fill viewport height, and use `min(880px, 100vw)` width with square edges; BaseDialog retains focus and scroll-lock behavior.

The version popover uses `0 12px 48px rgb(15 23 42 / 20%)` depth and a fixed body portal. Its own 200ms transition is separate from the reduced-motion rules scoped to descendants of the console shell.

Buttons/cards transition colors over 150ms; inputs/selects over 200ms; sidebar drawer transform uses 300ms. Reduced-motion shell transitions/animations use 0.01ms and one iteration. Trend-chart animation is disabled. There is no decorative workspace-margin motion contract.

**The Surface Rule.** Use tone and fine borders for work panels; reserve pronounced depth for overlays.

## Shapes

Shared buttons/text inputs use 8px corners; console select triggers, settings categories, badges, and compact chart controls use 6px. Navigation links/search use 7px. Panels/dialogs use 12px. Density choices use 4px within a 6px group. The fallback brand mark uses 9px; configured logos retain their form.

Collection ledgers, analytical reports, and configuration sections use square dividers rather than enclosing rounded cards. Standalone identity, checkout, subscription, and dialog containers retain 12px corners. Editor sheets have square viewport edges. Borders are one CSS pixel. Component-specific provider/status pills do not define the shared badge geometry.

## Components

### Buttons

Shared actions have a minimum 36px height, 8px/12px padding, 14px medium text, and 8px corners. Size modifiers can change padding/radius/type; 36px is a minimum, not a fixed height. Primary is violet/white, secondary neutral bordered, ghost transparent, and danger red. Disabled actions use 50% opacity and a not-allowed cursor. Shell keyboard focus uses a 2px accent outline with 3px offset, alongside component-owned focus styling. Purchase, plan purchase, and subscription renewal share the primary action.

### Inputs / Fields

Text inputs match shared action density. Focus uses primary 500 border/ring, errors remain red, and disabled fields preserve their own treatment. Placeholders use theme muted text at opacity 1. Query-panel input/select controls have a 38px minimum and 12px type. Preserve labels, help, validation, and icon/adornment spacing.

Console select triggers use a 36px minimum and 6px corners. Menus teleport to the body and preserve search, selected/focused states, disabled choices, and keyboard behavior; root theme variables also reach portals.

### Navigation

The single sidebar contains configured branding, workspace switching, search, expandable modules, and footer utilities. The stock Sub2API name falls back to 937 Console; configured names/logos remain authoritative. Administrator modules cover overview, gateway, customers, billing, security, and workspace, omitting empty modules. The active module opens by default, manual overrides last until route change, and search exposes matching authorized destinations across workspaces. The administrator/personal switch appears only for expanded, non-simple-mode administrators. Administrator context includes simple mode and exact configured administrator destinations as well as `/admin/` routes, so `/keys` and administrator custom pages retain their modules.

Search runs after permission/feature/simple-mode visibility, matches labels and nested destinations, and clears on Escape. Collapsed navigation hides search and text but retains accessible labels/tooltips. Mobile opening expands the sidebar even when desktop collapse was stored. Selected links use wash and 600 weight; hover uses divider tone. Nested module links use 12px text and a 34px minimum height. A skip link appears on keyboard focus. The header provides quiet breadcrumbs and account utilities. Theme switching uses the shared VueUse `useDark({ storageKey: 'theme' })` behavior, with root theme variables reaching portals.

### Version Popover

The administrator version control opens a nonmodal dialog in a body portal. Fixed positioning chooses the available side of the trigger and clamps to 12px horizontal/16px vertical viewport margins. Width is at most 400px; height is at most 720px or viewport height minus 32px, with internal scrolling. Resize, scroll, and content resize reposition it. Opening focuses the panel; Escape or the explicit close button restores trigger focus. Long content wraps, and displayed versions normalize repeated leading `v` characters to one. Existing update/rollback actions retain their semantics; ordinary users receive static version text.

### Chips and Status

Shared badges use 6px corners, 2px/8px padding, and 12px medium type. Success, warning, danger, primary, purple, and neutral variants preserve their meanings. Separate provider pills and status dots retain their own geometry.

### Cards / Containers

Solid theme surfaces, fine borders, and 12px corners remain appropriate for standalone identity, checkout, subscription, and overlay containers. Collection ledgers, report sections, and configuration documents remove the enclosing card treatment. Checkout/identity primarily use 24px insets; report headings in the analytical grid align directly with their content. Table wrappers and divided sections do not inherit blanket card padding.

### Settings and Editor Sheets

Category controls use a 40px minimum, 8px/12px padding, and selected wash. Section headings precede fields. Workspace tabs use underlines and preserve ARIA selection state. Sheets keep long forms scrollable and actions in a footer; field-label columns stack on mobile.

### Checkout and Plans

Editable fields occupy the main panel; review retains totals, fees, and the final action. Payment method choices have a 64px minimum, neutral borders, provider logos, and a consistent violet selected border/wash. Unavailable methods stay disabled. Plan labels use gray 600 in light mode and dark 400 in dark mode; prices and quantities remain prominent. Currency, quota periods, rate multipliers, enabled methods, and validation remain content truth. Provider logos do not recolor the shared confirmation or renewal CTA.

### Charts

Trend tabs switch token composition, requests, cost, and cache rate. Tokens use stacked bars; other metrics use focused lines, including a dashed standard-cost comparison. Axes/tooltips preserve token, currency, and percentage units. Model/group/endpoint distributions use horizontal bars plus explicit tables; ranking and source controls retain their behavior. Payment method bars are content-sized and normalized separately within each currency; rankings keep currencies separate rather than aggregating unlike monetary units. Actual, account, and standard cost stay separate; standard cost text uses gray 600/400 rather than light gray 400. Empty/loading/error states remain visible.

## Do's and Don'ts

### Do:

- **Do** use one sidebar, a quiet utility header, and a clear content heading.
- **Do** use the shared violet action language for purchase and renewal.
- **Do** reveal horizontal filters on demand and preserve query state when hidden.
- **Do** place settings section headings above fields and keep identity metadata flat.
- **Do** preserve semantic colors, units, currency distinctions, permissions, feature flags, validation, and stored theme.
- **Do** treat preview values and screenshots as synthetic visual evidence only.

### Don't:

- **Don't** restore the rejected dark horizontal business bar, teal action styling, or permanent collection filter column.
- **Don't** duplicate page titles in the utility header or use competing provider-colored console CTAs.
- **Don't** replace labeled analytical comparisons with overlapping multi-metric fills or doughnut distributions in this workspace.
- **Don't** erase errors, adornments, disabled behavior, or provider-owned controls for visual uniformity.
- **Don't** apply this B-console specification to C, backend behavior, or unrelated public surfaces.
- **Don't** interpret local screenshots or documentation as proof of deployment or live payment success.
