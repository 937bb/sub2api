---
name: "937Sub B Console"
description: "A compact API console with muted blue actions, aligned records, and one component language across dialogs, sheets, and portals."
colors:
  console-bg: "#f7f8fa"
  console-surface: "#fff"
  console-nav: "#f6f7f9"
  console-line: "#e7e9ed"
  console-ink: "#242931"
  console-muted: "#626b78"
  console-accent: "#32688b"
  console-accent-hover: "#2b5673"
  console-selected: "#edf3f8"
  dark-console-bg: "#10151e"
  dark-console-surface: "#19212e"
  dark-console-nav: "#1e2837"
  dark-console-line: "#344154"
  dark-console-ink: "#edf2f8"
  dark-console-muted: "#aebed2"
  dark-console-accent: "#9ec0d8"
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
  chart-input: "#42799b"
  chart-output: "#627b8d"
  chart-cache-read: "#27857a"
  chart-cache-write: "#b7791f"
  chart-cache-rate: "#27857a"
  chart-standard: "#737f94"
  dark-chart-input: "#80b5d6"
  dark-chart-output: "#b3c4d5"
  dark-chart-cache-read: "#58bca5"
  dark-chart-cache-write: "#eab64d"
  dark-chart-standard: "#aab6c9"
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
  dialog-title:
    fontSize: "17px"
    fontWeight: 600
    lineHeight: 1.5
  field-label:
    fontSize: "13px"
    fontWeight: 500
    lineHeight: "20px"
  metric:
    fontSize: "25px"
    fontWeight: 600
    lineHeight: 1.3
    letterSpacing: "-0.025em"
  ops-metric:
    fontSize: "23px"
    fontWeight: 600
    lineHeight: 1.3
  usage-metric:
    fontSize: "17px"
    fontWeight: 600
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
  control: "6px"
  brand-mark: "10px"
  panel: "12px"
  dialog: "10px"
  popup: "8px"
  selection: "20px"
spacing:
  small: "8px"
  compact: "10px"
  control: "12px"
  regular: "16px"
  field-gap: "18px"
  section: "20px"
  panel: "24px"
  document-gap: "28px"
  editor-gap: "32px"
components:
  button-primary:
    backgroundColor: "{colors.console-accent}"
    textColor: "{colors.console-surface}"
    rounded: "{rounded.control}"
    padding: "6px 13px"
  button-primary-hover:
    backgroundColor: "{colors.console-accent-hover}"
  button-secondary:
    backgroundColor: "{colors.console-nav}"
    textColor: "{colors.console-ink}"
    rounded: "{rounded.control}"
    padding: "6px 13px"
  button-secondary-hover:
    backgroundColor: "{colors.console-line}"
  button-ghost:
    backgroundColor: "transparent"
    textColor: "#4b5563"
    rounded: "{rounded.control}"
    padding: "6px 13px"
  button-danger:
    backgroundColor: "{colors.danger-action}"
    textColor: "{colors.console-surface}"
    rounded: "{rounded.control}"
    padding: "6px 13px"
  input:
    backgroundColor: "{colors.console-nav}"
    textColor: "{colors.console-ink}"
    rounded: "{rounded.control}"
    padding: "7px 11px"
  input-focus:
    backgroundColor: "{colors.console-surface}"
  dialog-narrow:
    backgroundColor: "{colors.console-surface}"
    textColor: "{colors.console-ink}"
    rounded: "{rounded.dialog}"
  dialog-normal:
    backgroundColor: "{colors.console-surface}"
    textColor: "{colors.console-ink}"
    rounded: "{rounded.dialog}"
  select-popup:
    backgroundColor: "{colors.console-surface}"
    textColor: "{colors.console-ink}"
    rounded: "{rounded.popup}"
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
  ops-panel:
    backgroundColor: "{colors.console-surface}"
    rounded: "{rounded.panel}"
    padding: "16px"
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

The B console is a compact API operations workspace. Its user-pinned visual reference is New API: a white global header and sidebar, a neutral gray canvas, restrained blue actions, and readable records. Revision 8 carries the same component language into modal interiors, right-side sheets, and body-mounted portals, while retaining flat page roots, compact query controls, and aligned metric groups. The task and its data lead the screen.

Configured branding, Chinese and English labels, stored dark theme, permissions, and existing billing distinctions remain product truth. New API supplies the frontend reference; Sub2API remains the backend. This is the implemented visual system for the B console and shared authentication layout; deployment C and provider-owned payment flows retain their separate boundaries.

**Key Characteristics:**

- Global workspace header with directly visible navigation categories.
- Muted blue actions and selection on neutral surfaces; no decorative violet category system.
- Flat page roots, compact collections, preserved advanced filters, and integrated pagination.
- Operations metric matrix, compact resources, and deliberately placed traffic analysis.
- Responsive records with paired fields, optional detail, and preserved column choices.
- Proportionate dialogs and sheets with grouped fields, visible actions, and themed portals.

This document records revision 8 from current source. Its surface strategy and provenance live in [.impeccable/b-console-revision8.md](.impeccable/b-console-revision8.md); durable scope lives in [PRODUCT.md](PRODUCT.md). Source authority is frontend/tailwind.config.js, frontend/src/style.css, frontend/src/styles/console.css, frontend/src/styles/workbench.css, and frontend/src/styles/surfaces.css, followed by the corresponding Vue components. main.ts imports surfaces.css last so the shared interaction surface rules supersede older shell control styles. The Tailwind primary scale and CSS theme references agree; documentation must not create a separate accent palette.

The user repeatedly authorized a replacement redesign, rejected the preceding violet-heavy operations and usage presentation and then the remaining old modal interiors, and pinned New API as the reference throughout. This remains a reference-driven, code-led continuation without an approved comp or recorded final user acceptance. The inherited direction seed is 1f6e3f1a; no new revision-8 seed provenance is claimed.

The visual reference includes QuantumNous/new-api's dashboard and usage-log imagery plus its dialog and drawer hierarchy, as recorded in the revision-8 brief. No React dependency transplant, new raster illustration, or photography is introduced. Configured logos and payment/provider marks keep their origins. The [coverage ledger](.impeccable/review/revision8-coverage.md) records 111 source files, 121 BaseDialog uses, 42 ConfirmDialog uses, and 26 Teleport occurrences; composition makes these overlapping counts. It names representative rendered and keyboard evidence and its limits. Preview data is synthetic. This source extraction does not certify every provider branch, conditional error, locale, physical-phone keyboard, live payment settlement, deployment, or final review outcome. Defects and unverified states are not design precedents.

## Colors

Neutral surfaces carry a single muted blue action scale. The frontmatter is normative; values are extracted from the current CSS, Tailwind configuration, and chart components. The sidecar's ramps are display metadata: the action ramp comes from the existing primary scale, and other ramps are derived previews rather than additional CSS tokens.

### Primary

- **Muted Action Blue** supplies filled primary actions, light-theme selection text, and keyboard focus.
- **Deep Action Blue** supplies primary-action hover.
- **Selection Wash** marks active destinations and selected controls.
- **Night Blue** supplies dark-theme focus and selection. Filled primary buttons keep primary-600 blue and white text in both themes.

### Neutral

- **Canvas Gray** separates work regions from the shell.
- **Working White** fills the header, sidebar, and individual task panels.
- **Quiet Neutral** fills table headers, control tracks, secondary actions, and resting fields.
- **Fine Divider**, **Graphite Ink**, and **Muted Graphite** establish boundaries and text hierarchy.
- Dark-console tokens preserve the same roles, including fields and secondary actions inside body-mounted portals. The older Tailwind gray/slate tokens remain for badges, ghost actions, and specialized content; they do not override the shared field treatment.

### Semantic and Analytical Colors

Success, warning, danger, and neutral badges retain their operational meanings. Threshold colors communicate condition. Token trend input and output use muted blue and blue-gray; cache read/write retain teal and amber. Actual cost and requests use input blue; standard cost is gray with a dashed comparison line in area mode; cache rate is teal. Operations throughput retains blue QPS and green TPS series, while switching uses teal. Series labels, units, legends, tooltips, and numeric tables carry meaning alongside color.

Decorative purple has been removed from the redesigned console families. Genuine Stripe branding remains in its payment-specific button and embedded payment surface, as do other provider marks. A provider's brand color is not a shared console action token.

**The Action Rule.** Use the shared muted blue scale for console actions; preserve semantic data colors and genuine provider branding.

## Typography

Use the system sans stack with Chinese fallbacks and the existing monospace stack for code or identifiers. No custom display font is introduced. Shell numbers use tabular figures.

Page headings use the compact headline role; the operations heading uses 600 weight. Panel headings use the section-title role, while settings and order-review headings use 15px/600. Modal headings use the dialog-title role. Controls and table cells use 13px; shared form labels use the field-label role above their controls, while primary metadata and help use 12px. Supporting units, category labels, and chart ticks may use 11px where implemented, with readable contrast.

Dashboard headline values become 24px below 1280px. Operations headline values use the ops-metric role, becoming 21px between 1000px and 1199px; current realtime QPS/TPS values use 20px. Usage aggregates use the compact usage-metric role. Authentication headings reduce to 26px below 768px. Workbench descriptions cap at 70ch; records and forms do not inherit that measure.

**The Data Rule.** Keep numerical precision, units, and actual, account, and standard charge labels intact when changing presentation.

## Layout

The global header is 60px tall and sticks at the top. At 1024px and above, the sidebar starts beneath it, is 208px wide or 64px collapsed, and fills the remaining viewport height. Content offsets by that width. Main content is centered within a 1920px maximum width, with 20px vertical and 24px horizontal padding. Page roots are transparent and unframed; individual working regions provide the surface.

Main padding becomes 16px below 1280px and 10px below 768px. Below 1024px, the sidebar becomes a drawer, offsets disappear, and workspace switching moves into the drawer. The brand label is capped at narrow widths and hidden below 480px while its mark remains.

Collections use a compact heading/actions row, visible primary query strip, current-page controls, framed records, and attached pagination. Generic collection filters start open and preserve values when collapsed. Search uses a 200px minimum and 1.5 flex share; other collection fields use a 150px minimum and one share, with 10px gaps. These generic fields become full width below 768px. Query regions have no fixed-height cap.

Tables retain native table layout, local overflow, and sticky columns. Headers are 42px high; compact rows use 10px vertical padding. Table density persists under b-console-table-compact and defaults to compact; its control hides below 768px. Existing mobile record rendering remains content-specific and pages use document scrolling.

### Operations

The normal and fullscreen operations views share the same composition: compact toolbar; full-width diagnosis disclosure; eight metric groups; a short resource band; then traffic, quality, and log tabs. The groups are realtime traffic, request totals, SLA, request duration, TTFT, request errors, upstream errors, and per-request output speed.

The metric matrix is four columns by two rows from 1000px upward, two columns at 999px and below, and one column at 599px and below. Realtime traffic is a compact QPS/TPS table with current, peak, and average rows; it does not occupy a separate tall side column. The six resource cells cover CPU, memory, database, Redis, goroutines, and background jobs. They use six columns normally, three at 999px and below, and two at 599px and below.

Traffic places throughput in the wide first column and concurrency/queue status in the second column of the first row; average account switching spans the full second row. The grid is 2fr plus a minimum-280px 1fr column with a 16px gap. Throughput is 400px tall and switching is 280px tall. At 1023px and below, panels stack in document order with throughput ordered first. Quality panels form two columns with the final panel spanning both, then stack at the same breakpoint. Fullscreen content retains the 1920px cap and uses 24px padding, reduced to 12px below 600px.

### Usage and Dashboard

Administrator and customer usage default to records. The records frame contains queries, the short aggregate strip, then the table and pagination. Analysis is an explicit separate view with trends and model/group/endpoint distributions. Administrator primary filters use four columns and customer filters use three; secondary query dimensions use an expandable section. The expanded state does not clear criteria, and active hidden criteria are counted on the toggle. Below 768px, usage filter grids use two columns.

On desktop, aggregates occupy four compact columns. They become two columns below 768px. On records pages below 480px, aggregates collapse into a 44px-minimum summary control showing requests and actual cost; it expands the complete statistics in place. The mobile record view uses paired model, token, cost, and latency fields, with time/user identity spanning the row. Selected remaining columns are preserved behind More. This keeps a complete primary record compact while retaining its secondary data and actions. User-visible column preferences still determine which fields exist.

Dashboard summaries retain five measures, or four when their simple-mode prop removes finance, followed by a resource/usage band and optional lifetime details. They become three columns below 1024px and two below 768px. Dashboard/usage trend plots are full width and 310px high, reduced to 260px below 768px. Model analysis internally pairs bars and numeric results at a 0.85:1.5 ratio, stacks below 1024px, and caps each local scroll area at 330px.

### Shared Page Families

The payment overview uses a four-column, white-surface metric strip, a full-width
trend panel, then payment methods and customer rankings in a 1:1.4 grid. The trend
plot is 264px high (220px below 768px). Its metric tabs share the panel heading;
one muted blue series represents the selected count or currency, with full dates
retained in tooltips and shorter month/day axis labels. Breakdown lists scroll
locally at 280px, preserving every currency and customer. Below 768px the breakdown
stacks and summary metrics form two columns. Refresh retains the previous layout.
Order filters use visible labels and four columns, two below 1280px and one below
480px. TRON and BEP20 remain separate filter options. Financial values, refund
actions, and backend pricing remain unchanged.

Settings use horizontally scrollable pill categories, sticky at 60px on desktop and static on mobile, above individually framed sections. The save bar stays sticky. Identity places flat metadata inside a single panel; its three metadata columns and two editor columns become one below 768px. Repeated families share neutral, compact panels without restoring a page-wide outer card.

Checkout retains a flexible main panel and a 320px sticky review column; they stack below 1024px. Subscription, plan, monitor, catalog, and finance layouts preserve their dedicated quotas, actions, and query paths. Authentication retains a 0.85:1.15 identity/form split with a 320px minimum identity column and a 400px maximum form; below 768px it becomes an identity band followed by the form. Setup, callbacks, unrelated public pages, and provider-owned payment fields retain specialized flows.

### Dialogs, Sheets, and Interiors

BaseDialog has explicit maximum widths: narrow 440px, normal 600px, wide 760px, extra-wide 1000px, and full 1280px. Normal is the default. Use narrow for confirmations and short prompts, normal for standard forms, wide for multi-section editors or rich content, and larger sizes for tables or dense analysis. These are maximum widths, constrained by the viewport; a small form does not inherit a fixed wide sheet.

Centered overlays have 24px outer spacing and a maximum panel height of viewport height minus 48px. Headers, optional section navigation, and footer actions remain outside the scrolling body. The header has a 64px minimum height and 18px/24px/14px padding; the body uses 8px/24px/24px; the footer uses 16px/24px. Sheets opt into right alignment, use the caller's width tier, fill 100dvh, and have square edges. Their header and body use 24px horizontal spacing.

Related fields live inside named DialogSection groups. The first group starts without a separator; later groups have 24px separation and a fine top border. Group headings and short descriptions share one row when space permits. Fields use an 18px grid gap, one or two columns, and labels above controls. DialogSectionLinks scrolls to the real groups inside the active panel. Detail records use compact bordered grids, wrapping values, and local overflow for code or tables.

At 640px and below, centered overlay spacing becomes 12px, horizontal interior padding becomes 16px, two-column field groups stack, and footer action rows may wrap. Sheets occupy the available width and full viewport height. Modal text inputs, textareas, and native selects use 16px text at this breakpoint. The shared Select primitive also declares 16px mobile text; its surrounding surface cascade still governs computed styles, so physical-phone keyboard behavior is not certified here.

## Elevation & Depth

Working surfaces and shared console buttons are flat, separated by neutral fill and fine borders. Page roots add no outer frame. Selected switches may retain their existing small shadow. Legacy utility definitions do not establish a glass, gradient, or glow treatment for the console.

Dialogs use an unblurred scrim and soft depth, without an additional panel border. Centered dialogs and the version popover share the overlay shadow; menus, tooltips, and toasts use the smaller popup shadow. Right-side sheets use square edges and no panel shadow. Exact overlay and popup shadows live in the sidecar.

Shared action color transitions use 150ms; fields use 150ms; the sidebar drawer uses 300ms. Modal transitions use 160ms, with a short horizontal entrance for sheets; Select dropdowns retain their 200ms entrance. Token, operations throughput, and operations switching trend animations are disabled so refresh and fullscreen resizing show complete plots. Reduced-motion rules reduce shell-descendant animation/transition durations to 0.01ms and one iteration, and remove overlay, panel, and switch transitions. Other portal transitions are not covered by a universal reduced-motion guarantee.

**The Surface Rule.** Frame the working region once; use tone and fine borders for internal grouping and pronounced depth for overlays.

## Shapes

Shared controls and badges use compact corners. Dialog panels use the dialog radius, while retained page, chart, and collection frames use the larger working-panel radius. Popups, detail grids, and pagination use the popup/pagination radius; the fallback brand mark retains its own radius. Main navigation and page-level segmented choices retain pill corners. Dialog choices use compact rectangular corners. Borders are one CSS pixel. Configured logos, provider marks, circular avatars, and square sheet edges retain their purposeful geometry. The frontmatter distinguishes these roles instead of forcing every container to one radius.

## Components

### Buttons and Fields

Console actions use 13px medium text, 6px/13px padding, a 34px minimum height, and control corners; inherited line-height or borders can increase rendered height. Primary is muted blue/white; secondary uses neutral fill and a transparent border; ghost is transparent; danger is red. Disabled actions retain 50% opacity and a not-allowed cursor. The shell supplies a 2px accent focus outline with a 3px offset; dialog and popover controls use a 2px offset.

Inputs and select triggers have a 36px minimum height, 7px/11px padding, neutral resting fill, and themed borders. Focus switches the fill to the working surface, applies the accent border, and shows a selection-colored outline; keyboard-visible focus remains explicit. Shared control text is 13px, with page-specific compact query overrides retained where the cascade applies. Disabled inputs use 60% opacity and a not-allowed cursor. Fields preserve full-opacity muted placeholders, labels, validation, input adornments, help, and select search/keyboard behavior. Root theme variables reach body-mounted overlays.

### Navigation

The header owns configured branding, administrator/personal workspace switching for eligible administrators, and account utilities. Stock naming falls back to 937 Console. Sidebar destination categories remain directly visible; nested feature groups retain expansion and route-aware state.

Search operates after permission and feature filtering, may expose authorized destinations across workspaces, and clears on Escape. Simple-mode and configured administrator destinations retain their context. Collapsed navigation preserves labels/tooltips and expands when the mobile drawer opens. Selection uses blue wash and 600 weight; hover uses a quiet surface. A skip link targets main content, and theme preference uses the shared theme storage key.

### Chips and Panels

Neutral and semantic badges use compact corners with 12px medium labels and 2px/8px padding. Collection counts use selection wash; status counts retain semantic fills. Task panels use neutral surfaces, fine borders, and compact headings. Table frames integrate pagination without blanket inner padding. Metric groups share dividing lines inside one frame.

### Charts and Records

TokenUsageTrend starts with stacked token bars. Chart form and metric selection remain independent; token areas stack, and requests/cost/cache rate preserve their units and tooltip precision. Loading and empty states remain in the plot. Model/group/endpoint distributions retain explicit numeric tables and their enabled source, ranking, metric, and breakdown controls.

Operations throughput and switching chart palettes and options depend reactively on the stored theme, including labels, grids, and tooltip colors. Switching theme updates the current chart. The muted action ramp does not replace the established QPS/TPS data series.

Usage bars and legend markers use opaque series colors. Transparency belongs only to area fills. Sticky checkbox/name boundaries follow the measured checkbox header width at each density, so row backgrounds remain continuous during horizontal scrolling.

Usage records separate date/time and identity metadata, align token/charge fields, and group TTFT, duration, and output speed under latency. Native More disclosures preserve secondary columns on mobile; expanding query controls retains filter state. Error, pending, export, cleanup, column settings, detail, and pagination behavior remain attached to the existing workflow.

Administrator-dashboard simple mode suppresses its financial summary, pricing shortcut, spending tab/rankings, cost columns, and account-cost breakdown; a hidden selected cost trend resets to tokens. This is the established dashboard display path, not a claim that every usage route has been universally audited. Customer usage suppresses account-cost detail through its existing display prop.

### Settings, Checkout, and Overlays

Settings preserve fields, section organization, and save feedback. Checkout preserves currencies, methods, fees, quotas, and validation; provider payment identities remain recognizable. Editors group related inputs in real sections and preserve long-form scrolling and footer actions.

BaseDialog owns the modal stack, a unique accessible heading, localized close label, focus restoration, and body scroll locking. Only the topmost open dialog responds to Escape and Tab wrapping. Portalled listboxes handle their own keyboard focus. Closing a nested dialog restores its prior trigger and retains the body lock while the parent stays open. Escape closing defaults on; outside-click closing defaults off and remains caller-controlled. These are implemented shared behaviors, not a claim that every consumer's state has been exercised.

ConfirmDialog is a narrow centered dialog with a compact icon/message row and explicit cancel/confirm actions; danger changes the icon and action semantics. ImagePreview uses the full width tier and shared stack at z-index 100, contains the image without distortion, and allows outside-click close. Security challenges, announcements, and payment overlays join the same surface language while retaining their own validation, progress, and provider behavior.

Select triggers use the shared field tokens in their scoped CSS. Their body-mounted menu uses a compact popup frame, a search row where enabled, 32px-minimum options, blue selected/focused states, and explicit disabled options. Detail navigation uses a horizontally scrollable text row; active tabs, where the caller provides pressed state, carry an accent underline. Section-directory buttons scroll rather than inventing tab selection.

The administrator version control opens a nonmodal body-portal dialog, clamped to 12px horizontal and 16px vertical viewport margins, at most 400px wide and min(720px, viewport height minus 32px) tall. It scrolls internally and repositions on resize, scroll, and content changes. Escape/close restores trigger focus, long text wraps, and repeated leading version-prefix letters normalize to one. Update/rollback behavior remains intact; ordinary users receive static version text.

## Do's and Don'ts

### Do:

- **Do** use the global header, directly visible destination categories, and one clear page heading.
- **Do** use the shared muted blue ramp for console actions and retain meaningful semantic and provider colors.
- **Do** keep primary queries visible, preserve collapsed advanced criteria, and show their active count.
- **Do** preserve selected columns on mobile through primary fields and the More disclosure.
- **Do** preserve labels, units, charge distinctions, permissions, feature flags, validation, and stored theme.
- **Do** choose overlay width for its content, group related fields, keep labels above controls, and keep footer actions outside body scrolling.
- **Do** use shared theme tokens and modal behavior for body-mounted dialogs, sheets, and portals.
- **Do** treat local preview values and screenshots as synthetic visual evidence.

### Don't:

- **Don't** add an outer white card around every page or duplicate frames around the same working region.
- **Don't** reintroduce decorative violet as the console action or category palette.
- **Don't** turn the operations realtime metrics into a tall separate column with an empty space beneath it.
- **Don't** hide meaningful metadata through low contrast or remove advanced query dimensions to simplify the screen.
- **Don't** remove errors, input adornments, disabled behavior, or provider-owned controls for visual uniformity.
- **Don't** restore fixed 880px sheets, per-field horizontal rules, or old shell-only styling for overlay interiors.
- **Don't** interpret this document as user acceptance, deployment evidence, or universal route and accessibility certification.
