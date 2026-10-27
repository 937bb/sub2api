---
name: "937Sub B Console"
description: "A direct-access API console with framed data workspaces and full-width analysis."
colors:
  console-bg: "#f6f8fb"
  console-surface: "#fff"
  console-nav: "#f8fafc"
  console-line: "#e7ebf1"
  console-ink: "#202a3b"
  console-muted: "#617086"
  console-accent: "#2563eb"
  console-accent-hover: "#1d4ed8"
  console-selected: "#edf4ff"
  dark-console-bg: "#10151e"
  dark-console-surface: "#19212e"
  dark-console-nav: "#1e2837"
  dark-console-line: "#344154"
  dark-console-ink: "#edf2f8"
  dark-console-muted: "#aebed2"
  dark-console-accent: "#93c5fd"
  dark-console-selected: "#233654"
  control-ink: "#374151"
  control-hover: "#f9fafb"
  field-ink: "#111827"
  dark-control-bg: "#1e293b"
  dark-control-ink: "#e5e7eb"
  badge-neutral-bg: "#f3f4f6"
  success-bg: "#ecfdf5"
  success-ink: "#047857"
  warning-bg: "#fffbeb"
  warning-ink: "#b45309"
  danger-bg: "#fef2f2"
  danger-ink: "#b91c1c"
  danger-action: "#dc2626"
  chart-input: "#4f78ec"
  chart-output: "#8c6bdd"
  chart-cache-read: "#58bca5"
  chart-cache-write: "#eab64d"
  chart-cache-rate: "#27857a"
  chart-standard: "#8b97ac"
  chart-label: "#536175"
  chart-grid: "#edf0f4"
  dark-chart-label: "#bec9d9"
  dark-chart-grid: "#303e50"
typography:
  body:
    fontFamily: "system-ui, -apple-system, BlinkMacSystemFont, Segoe UI, Roboto, Helvetica Neue, Arial, PingFang SC, Hiragino Sans GB, Microsoft YaHei, sans-serif"
    fontSize: "13px"
    lineHeight: "20px"
  mono:
    fontFamily: "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace"
  headline:
    fontSize: "20px"
    fontWeight: 650
    lineHeight: 1.4
    letterSpacing: "-0.02em"
  brand:
    fontSize: "15px"
    fontWeight: 650
  section-title:
    fontSize: "14px"
    fontWeight: 600
  metric:
    fontSize: "27px"
    fontWeight: 650
    lineHeight: 1.3
    letterSpacing: "-0.025em"
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
  compact: "6px"
  pagination: "8px"
  control: "9px"
  brand-mark: "10px"
  panel: "12px"
  workspace: "16px"
  selection: "20px"
spacing:
  small: "8px"
  compact: "10px"
  control: "12px"
  regular: "16px"
  section: "20px"
  panel: "24px"
  document-gap: "28px"
  editor-gap: "32px"
components:
  button-primary:
    backgroundColor: "{colors.console-accent}"
    textColor: "{colors.console-surface}"
    rounded: "{rounded.control}"
    padding: "7px 12px"
  button-primary-hover:
    backgroundColor: "{colors.console-accent-hover}"
  button-secondary:
    backgroundColor: "{colors.console-surface}"
    textColor: "{colors.control-ink}"
    rounded: "{rounded.control}"
    padding: "7px 12px"
  button-ghost:
    backgroundColor: "transparent"
    textColor: "#4b5563"
    rounded: "{rounded.control}"
    padding: "7px 12px"
  button-danger:
    backgroundColor: "{colors.danger-action}"
    textColor: "{colors.console-surface}"
    rounded: "{rounded.control}"
    padding: "7px 12px"
  input:
    backgroundColor: "{colors.console-surface}"
    textColor: "{colors.field-ink}"
    rounded: "{rounded.control}"
    padding: "8px 12px"
  navigation-active:
    backgroundColor: "{colors.console-selected}"
    textColor: "{colors.console-accent}"
    rounded: "{rounded.selection}"
    padding: "8px 12px"
  badge-neutral:
    backgroundColor: "{colors.badge-neutral-bg}"
    textColor: "{colors.control-ink}"
    rounded: "{rounded.compact}"
    padding: "2px 8px"
  content-panel:
    backgroundColor: "{colors.console-surface}"
    rounded: "{rounded.panel}"
    padding: "20px"
  workspace:
    backgroundColor: "{colors.console-surface}"
    rounded: "{rounded.workspace}"
    padding: "24px"
  chart-choice:
    backgroundColor: "{colors.console-surface}"
    textColor: "{colors.console-ink}"
    rounded: "{rounded.selection}"
    padding: "5px 11px"
  collection-header:
    backgroundColor: "{colors.console-nav}"
    textColor: "{colors.console-muted}"
    height: "42px"
---

# Design System: 937Sub B Console

## Overview

**Creative North Star: "The Direct-Access Console"**

The B console is a compact API operations workspace. Its confirmed reference is New API: a white global header and sidebar, a light gray canvas, rounded white working areas, restrained blue selection, and readable records. The interface uses direct destination categories, visible collection filters, and full-width analytical panels to make operational work easy to find and scan.

Configured branding, Chinese and English labels, stored dark theme, permissions, and existing billing distinctions remain product truth. This is an interface grammar for the B console and shared authentication layout; it does not prescribe a new backend, replace provider-owned flows, or extend to deployment C.

**Key Characteristics:**

- Global workspace header with directly visible navigation categories.
- Blue actions and selected states on white surfaces and a light gray canvas.
- Framed collections with filters open by default, compact records, and integrated pagination.
- Five headline measures, a resource band, and full-width metric/form-switching analysis.
- Responsive stacking, local table overflow, and a mobile navigation drawer.

This document records revision 6 and replaces the rejected revision-5 Stripe system. Authority: PRODUCT.md and .impeccable/b-console-revision6.md. Source evidence: frontend/tailwind.config.js; frontend/src/style.css; frontend/src/styles/console.css and its later workbench.css composition layer; AppLayout, AppHeader, AppSidebar, TablePageLayout, ConsoleSummary, TokenUsageTrend, ModelDistributionChart, administrator/user usage, settings, payment, and version components. The documenter sampled final dashboard and groups captures; the separate finish review records broader evidence. Local screenshots under .impeccable/screenshots/revision6/ are labeled synthetic previews, not authenticated production sessions.

Reference provenance is visual only: QuantumNous/new-api's official dashboard.en.jpg and usage-logs.en.jpg screenshots, viewed on 2026-10-27. No New API source was copied, and this revision adds no raster illustration or photography. Existing configured logos/provider marks retain their origins. This describes current source; it does not certify every route, focus state, error branch, or accessibility requirement.

## Colors

Cool neutral surfaces carry a single blue action scale. Frontmatter contains the normative source values.

Sidecar tonal ramps are preview metadata, not additional CSS tokens. The primary ramp uses existing Tailwind values; other ramps are synthesized in OKLCH for the token panel.

### Primary

- **Action Blue** supplies light-theme selection text, keyboard focus, and filled primary actions.
- **Deep Action Blue** supplies filled primary-action hover.
- **Selection Wash** marks active destinations, selected controls, and hovered collection rows.
- **Night Blue** supplies dark-theme focus and selection. Filled primary buttons retain primary-600 blue and white text in both themes.

### Neutral

- **Canvas Gray** separates the working surface from the shell.
- **Working White** fills the header, sidebar, and workspaces.
- **Quiet Neutral** fills table headers, control tracks, and secondary surfaces.
- **Fine Divider**, **Slate Ink**, and **Muted Slate** establish boundaries and information hierarchy. Meaningful group usage-period and billing metadata now use the themed muted token.
- Dark-console tokens retain these roles. Shared fields and secondary buttons keep their existing Tailwind gray/slate foregrounds and dark-control backgrounds.

### Semantic and Analytical Colors

Success, warning, danger, and neutral badges retain their meanings. Dark badges use existing translucent emerald/amber/red fills and lighter text; status-count chips have opaque dark pairs. Provider marks remain distinct.

Trend input, output, cache read, and cache write map to blue, violet, teal, and amber. Requests and actual cost share input blue; standard cost is a gray comparison with a dashed line in area mode; cache rate is teal. Model/group/endpoint rankings retain component-local categorical palettes. Labels, legends, units, and numeric tables distinguish measures as well as color. Analytical colors are not alternate action palettes.

**The Action Rule.** Use the shared blue action scale for console actions; reserve semantic colors for their existing status and data meanings.

## Typography

Use the system sans stack with Chinese fallbacks and the existing monospace stack where code or identifiers need it. No custom display font is introduced. Shell numbers use tabular figures.

Page headings use the compact headline role. Analytical headings use the section-title role; settings and order-review headings use 15px/600. Controls and table cells use 13px; labels/metadata use 12px; supporting units, chart ticks, and category headings use 11px. Small type is not a substitute for readable contrast.

Headline metrics become 24px below 1280px. Usage-summary values use 24px; finance-summary values use 26px. Authentication headings reduce from their recorded size to 26px below 768px. Workbench descriptions cap at 70ch; records and forms do not inherit that measure.

**The Data Rule.** Keep numerical precision, units, and actual, account, and standard charge labels intact when changing presentation.

## Layout

The full-width global header sticks at top zero with a 60px height. At 1024px and above, the sidebar begins below it, is 208px wide or 64px collapsed, and occupies the remaining viewport height. Content offsets by that width. Main content is centered within a 1920px maximum width with a 20px outer inset. Supported page roots have a white workspace, a fine border, a 16px radius, and 24px internal padding.

Below 1280px, outer/internal insets become 16px/20px. Below 1024px, the sidebar becomes a drawer, content offsets disappear, and workspace switching moves into the drawer. Below 768px, insets become 10px/16px and workspace radius becomes 12px. The brand label is capped at narrow widths and hidden below 480px while the mark remains.

Collections form one framed task: title/count and actions, optional status tabs, visible query strip, current-page details and density, bordered records, and attached pagination. Filters start open and use v-show when collapsed to preserve values. Search has a 200px minimum and 1.5 flex share; other fields use a 150px minimum and one share, with 10px gaps. Mobile collection fields become full width. The query region has no fixed-height cap.

Tables keep native table layout, local horizontal/vertical overflow, and sticky columns. Headers are 42px high with neutral fill and 12px type; cells use 13px. Compact rows use 10px vertical padding; the layout's comfortable base uses 12px. Density persists under b-console-table-compact and defaults to compact; its control hides below 768px. Existing mobile record views and locally scrollable numeric tables remain content-specific. Mobile pages use document scrolling.

Dashboard summaries show five measures, or four when the simple-mode prop removes finance, followed by a wrapping resource/usage band and optional lifetime details. The summary grid becomes three columns below 1024px and two below 768px. The full-width trend plot is 310px high, or 260px below 768px. Model/user analysis share the following workspace through a selector. Within model analysis, bars and the numeric table use a 0.85:1.5 split, stack below 1024px, and each cap their scroll area at 330px. This internal split does not divide the primary trend.

Administrator/customer usage pages default to records, with an explicit analysis switch, summary strip, and shared time controls. Record filters sit above the bordered collection. Analysis reveals the trend and selectable model/group/endpoint distributions. Usage-summary columns become two below 768px.

Settings use horizontal scrolling pill categories, sticky at 60px on desktop, above bordered sections with 20px padding. The bottom save bar sticks; mobile categories become static. Identity retains flat metadata within a framed panel; its three metadata columns and two editor columns become one below 768px.

Checkout retains a flexible main panel and a 320px sticky review column, both padded 24px; they stack below 1024px. Plan/subscription sections retain responsive quota and action regions. Payment analysis has trend and method columns inside a full-width area followed by rankings; inner columns stack below 1280px. Operations monitoring keeps specialized traffic, capacity, quality, and diagnosis compositions.

Authentication retains a 0.85:1.15 identity/form split with a 320px minimum identity column and 400px maximum form; below 768px it becomes an identity band followed by the form. Setup, callbacks, unrelated public pages, and provider-owned payment fields keep specialized flows.

## Elevation & Depth

Workspaces, panels, and shared console buttons are flat, separated by fill and fine borders. Some source/ranking switches retain a small selected-state shadow. Unused legacy Tailwind definitions do not establish a glass or gradient system.

Dialogs use an unblurred scrim and soft depth. The version popover has its own shadow/body portal. Opt-in editor sheets align right, fill viewport height, and use min(880px, 100vw) width with square edges. BaseDialog focus and scroll locking remain intact. Exact shadow values live in the sidecar.

Shared button color transitions use 150ms; fields use 200ms; the sidebar drawer uses 300ms. Trend animation is disabled. Reduced-motion rules reduce shell-descendant animation/transition durations to 0.01ms and one iteration. Body-mounted portals retain their own behavior and are outside that blanket scope.

**The Surface Rule.** Use tone and fine borders for work areas; reserve pronounced depth for overlays.

## Shapes

Controls use 9px corners, badges 6px, pagination choices 8px, and the fallback brand mark 10px. Inner work panels use 12px; desktop workspaces/dialogs use 16px. Main navigation and segmented choices use 20px pill corners. Borders are one CSS pixel. Configured logos, provider marks, circular avatars, nested feature groups, and square editor-sheet edges keep their purposeful geometry.

## Components

### Buttons

Shared console actions use 13px medium text, 7px/12px padding, a 34px minimum height, and 9px corners; inherited line-height or borders can make the rendered height larger. Primary is blue/white; secondary is bordered neutral; ghost is transparent; danger is red. Disabled actions retain 50% opacity and a not-allowed cursor. The shell adds a 2px accent focus outline with 3px offset. Icon/size variants retain their content spacing.

### Inputs / Fields

Inputs/select triggers use a 36px minimum height, 13px text, 9px corners, and themed borders. Collection-query controls use 12px text. Inputs retain primary-500 focus rings, error/disabled states, labels, adornments, and help. Placeholders use full-opacity themed muted ink. Selects retain body portals and keyboard/search behavior; root theme variables reach overlays.

### Navigation

The global header owns configured branding, administrator/personal switching for non-simple administrators, and account utilities. Stock Sub2API naming falls back to 937 Console. Sidebar categories are directly visible; existing feature groups with children still expand, navigate, and preserve route-aware state.

Search runs after permission and feature filtering, may expose authorized destinations across workspaces, and clears on Escape. Simple-mode and exact configured administrator destinations keep administrator context. Collapsed navigation retains labels/tooltips and expands when its mobile drawer opens. Selection uses blue wash and 600 weight; hover uses a quiet surface. A keyboard skip link targets main content. Theme state uses the shared theme storage key.

### Chips and Status

Neutral/semantic badges use 12px medium labels, 2px/8px padding, and 6px corners. Collection total counts use selection wash; current-page status counts keep semantic fills. Provider pills do not redefine button styling.

### Cards / Containers

Outer workspaces frame the task. Inner collection/chart/settings/identity/checkout/subscription panels use fine borders and the panel radius. Panel headings divide controls from content. Table panels integrate pagination without blanket inner padding.

### Charts and Financial Visibility

TokenUsageTrend starts with stacked token bars. Its bar/area switch is independent of metric selection. Token areas stack; requests, costs, and cache rate preserve units, legends, and tooltip formatting. Loading/empty states remain inside the plot. Model/group/endpoint views pair horizontal bars with explicit tables and retain enabled source, ranking, metric, and breakdown controls.

Administrator dashboard simple mode now reaches summary/chart display flags: it omits monetary summaries, pricing shortcut, spending tab, monetary rankings and cost columns, account-cost breakdown, and the spending choice for user trends. When costs are hidden, a selected cost trend resets to tokens. This records the corrected administrator-dashboard path, not a universal audit of every usage/customer route. Standard-mode finance remains available. Customer analytics suppress account-cost columns where configured.

### Settings, Checkout, and Sheets

Settings categories are horizontal pills with section headings above fields. Checkout separates editable selection from review and preserves currencies, methods, fees, quotas, and validation. Plan and renewal actions use the shared primary action. Editor sheets preserve long-form scrolling and footer actions; field-label columns stack on mobile.

### Version Popover

The administrator version control opens a nonmodal body-portal dialog. It clamps to 12px horizontal/16px vertical viewport margins, at most 400px wide and min(720px, viewport height minus 32px) tall, with internal scrolling. Resize, scroll, and content-size changes reposition it. Escape/close restores trigger focus. Long text wraps; repeated leading version-prefix letters normalize to one. Existing update/rollback behavior remains intact. Ordinary users receive static version text.

## Do's and Don'ts

### Do:

- **Do** use the global header, directly visible destination categories, and one clear page heading.
- **Do** keep collection filters open initially and preserve their query state when collapsed.
- **Do** use blue for shared primary actions and selection while retaining existing semantic status colors.
- **Do** keep trends full width and expose both metric and chart-form choices where implemented.
- **Do** preserve labels, units, currency distinctions, permissions, feature flags, validation, and stored theme.
- **Do** treat local preview values and screenshots as synthetic visual evidence.

### Don't:

- **Don't** restore revision 5's expandable top-level modules, closed-by-default filters, unboxed collections, or split primary analysis.
- **Don't** hide meaningful billing-period metadata with low-contrast decorative text.
- **Don't** treat nested feature groups as evidence that top-level categories should collapse.
- **Don't** remove errors, input adornments, disabled behavior, or provider-owned controls for visual uniformity.
- **Don't** apply B-console visual rules to C, backend contracts, or unrelated public surfaces.
- **Don't** interpret this document or local captures as universal accessibility, all-route, deployment, or live-payment certification.

Not canonized: legacy violet gradient/glow definitions and isolated component-specific styles remain outside the shared revision-6 rules. Meaningful low-contrast text is a defect, not a house style. This documentation pass does not repair unrelated source or expand the bounded review into an accessibility certification.
