# PleumCloud design system

The visual and interaction spec for the PleumCloud UI.
[decisions.md](decisions.md) records architecture and product decisions;
this document records what the UI looks like and the rules that keep it
consistent.

The design is deliberately buildable from stock Tailwind CSS v4 — no
config file, no component library, no custom tokens, no webfonts. Every
value below is a default Tailwind utility applied inline. That is what
makes this document portable: to reuse it for a new project, copy the
file and swap only the items in the next section.

## Porting to a new project

**Keep:** the layout skeleton, the radius/type/gap scales, the state and
interaction patterns, the responsive rules.

**Re-theme:**

| Swap | PleumCloud value | New project |
|---|---|---|
| Accent hue | `blue-*` | pick one hue, keep the steps: 600 solid · 700 hover · 400 focus border · 100 active nav · 50 selection tint |
| Neutral canvas | `slate-*` | another gray family if the brand calls for it |
| Brand mark | ☁️ emoji + favicon gradient square | own mark |
| Domain color map | `providerColor` (19 services) | status colors, category colors, … |
| Icon vocabulary | emoji per file type / action | own emoji set (or an icon library — the rules stay the same) |

## Principles

- **Stock Tailwind only.** No `tailwind.config`, no CSS variables, no
  shadcn/Radix. `web/src/index.css` holds only the Tailwind import, body
  defaults, and third-party preview CSS (docx/sheet).
- **Light only.** No `dark:` variants anywhere. Dark surfaces are
  intentional inversions: modal overlay, preview lightbox, neutral toast
  (see [Dark surfaces](#dark-surfaces)).
- **Borders over shadows.** Elevation appears only as `shadow-sm` on
  resting cards, a hover upgrade, and `shadow-xl` on the modal.
- **Emoji as icons.** No SVG icon library; navigation, file types, and
  actions use emoji or plain glyphs.
- **Text as loading state.** No spinners or skeletons in the web app;
  busy buttons swap their label ("Connecting…", "Uploading…") and disable.
- **150 ms transitions.** The bare `transition` class wherever things
  move; nothing longer.

## Color

### Semantic palette

| Role | Value |
|---|---|
| Canvas | `bg-slate-100` (body) |
| Surface | `bg-white` — cards, sidebar, topbar, table |
| Border (default 1px) | `border-slate-200` · de-emphasized/dashed: `border-slate-300` |
| Hairline | `border-slate-100`, `border-slate-50` (table row dividers) |
| Text primary | `text-slate-900` (body default) · `text-slate-700` (list-item text, file names) |
| Text secondary | `text-slate-500` (captions, metadata) |
| Text tertiary | `text-slate-400` (incl. `placeholder:text-slate-400`) |
| Primary action | `bg-blue-600` → `hover:bg-blue-700`, `text-white` |
| Links | `text-blue-600 hover:underline` |
| Focus | `focus:border-blue-400 focus:outline-none` (border color only — no rings on inputs) |
| Selection tint | `bg-blue-50` (table row) + `text-blue-700` (selected name) · active nav `bg-blue-100/70 text-blue-800` |
| Danger | `text-red-600` / `text-red-700` · tint `bg-red-50` / `hover:bg-red-50` |
| Warning | `bg-amber-50 text-amber-700` |
| Success | `bg-emerald-600 text-white` (connected toast) |
| Disabled | `disabled:opacity-50` (buttons) · `text-slate-300 cursor-not-allowed` (nav) · `opacity-60` (unsupported cards) |

### Brand / provider colors

Third-party brand colors live in one map — `providerColor` in
`web/src/api.ts` — surfaced via `providerDot(id)` (dots: `size-2` badges,
`size-2.5` lightbox header) with fallback `#94a3b8` (slate-400). Used
for cloud badges, quota-bar segments, the lightbox header dot, and a
`ring-2 ring-white` chip on gallery thumbnails.

gdrive `#4285f4` · onedrive `#0078d4` · dropbox `#0061ff` · mybox
`#03c75a` · drime `#7c5cff` · pcloud `#ef7e33` · koofr `#38bdf8` · webdav
`#64748b` · infinicloud `#14b8a6` · mega `#d9272e` · box `#0061d5` ·
mediafire `#1299f3` · yandex `#fc3f1d` · hidrive `#0f766e` · jottacloud
`#2dd4bf` · filen `#3b82f6` · internxt `#111827` · protondrive `#6d4aff`

Rule: brand colors are data (the map), never inline classes — adding a
provider is one map entry.

## Typography

System font stack (Tailwind default sans — no webfonts), `antialiased`
on body.

| Size | Use |
|---|---|
| `text-[10px]` | "beta" tag on cards |
| `text-xs` | captions, table header, badges, field labels, section labels |
| `text-sm` | **default body size** — nearly all UI text, buttons, inputs, rows, modal bodies |
| `text-lg` | page title, sidebar logo, modal title, file-type emoji in rows |
| `text-2xl` | auth/empty-state headings, brand emoji |
| `text-4xl` / `text-5xl` | display emoji only (auth, fallback, audio) |

- `font-semibold` is the workhorse emphasis; `font-medium` secondary;
  `font-bold` only the brand wordmark and large headings.
- Section label recipe: `text-xs font-semibold uppercase tracking-wider
  text-slate-400`.
- Changing numbers (zoom %) get `tabular-nums`.

## Layout & spacing

App shell (`App.tsx`): `flex h-full` → fixed Sidebar + `flex min-w-0
flex-1 flex-col` → TopBar + `main flex-1 overflow-y-auto p-4 sm:p-6`.

| Region | Spec |
|---|---|
| Sidebar | `hidden w-72 shrink-0 border-r border-slate-200 bg-white md:flex` |
| TopBar | `h-16 shrink-0 border-b border-slate-200 bg-white px-4 sm:px-6` · search `mx-auto w-full max-w-xl min-w-0` |
| Content columns | table view `max-w-5xl` · connect/search `max-w-3xl` · modal `max-w-md` · auth card `max-w-sm` · empty state `max-w-md` — all `mx-auto` |

- Gap scale: `gap-1` chips · `gap-2` toolbars/forms · `gap-2.5` sidebar
  rows · `gap-3` cards and rows · `gap-4` empty-state logos. Stacks use
  `space-y-1.5` … `space-y-4`.
- Grids: provider cards `grid gap-3 sm:grid-cols-2` · gallery `grid
  grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4` · rules form `grid
  grid-cols-2 gap-2` with `col-span-2` full rows.
- Padding: buttons `px-3–4 py-1.5–2.5` · row cards `p-3` · provider
  cards `p-4` · modals/auth `p-6`.
- File table: `w-full text-sm` table inside a `rounded-2xl border
  border-slate-200 bg-white` wrapper with `overflow-hidden`; cells
  `px-4 py-2.5`; row dividers `border-b border-slate-50 last:border-0`.

## Shape & elevation

Radius grows with surface size:

| Radius | Surfaces |
|---|---|
| `rounded-full` | primary/secondary buttons, search input, badges, zoom pill |
| `rounded-lg` | nav items, ghost buttons, dialog inputs, row action buttons |
| `rounded-xl` | list cards, interactive cards, toasts, gallery tiles, auth inputs |
| `rounded-2xl` | table wrapper, modal, auth card, empty state, preview frames |

Borders are always 1px. Shadows: `shadow-sm` resting cards · `shadow`
hover upgrade · `shadow-md` gallery tiles · `shadow-xl` the modal, only.

## Component recipes

### Buttons

```tsx
// Primary (pill) — loading = label swap + disabled
className="rounded-full bg-blue-600 px-4 py-2 text-sm font-semibold text-white hover:bg-blue-700 disabled:opacity-50"

// Secondary (outline pill)
className="rounded-full border border-slate-200 bg-white px-3.5 py-1.5 text-sm font-semibold text-slate-600 hover:border-blue-300"
// toggle-active adds: border-blue-400 bg-blue-50 text-blue-700

// Ghost
className="rounded-lg px-2 py-1.5 text-sm font-medium text-slate-600 hover:bg-slate-100"

// Danger ghost
className="rounded-lg px-2.5 py-1.5 text-xs font-semibold text-red-600 hover:bg-red-50"

// Row action (revealed on hover)
className="rounded-lg px-1.5 py-1 text-slate-400 opacity-0 transition hover:bg-slate-200 hover:text-slate-700 group-hover:opacity-100"
```

### Sidebar nav item

```tsx
className="flex w-full items-center gap-3 rounded-lg px-3 py-2 text-sm"
// idle:     text-slate-600 hover:bg-slate-100
// active:   bg-blue-100/70 font-semibold text-blue-800
// disabled: cursor-not-allowed text-slate-300
```

### Inputs

```tsx
// Search (TopBar)
className="rounded-full border border-slate-200 bg-slate-50 px-4 py-2 text-sm text-slate-700 placeholder:text-slate-400 focus:border-blue-400 focus:bg-white focus:outline-none"

// Field (label + input)
label:  className="text-xs font-semibold text-slate-500"
input:  className="rounded-lg border border-slate-200 px-3 py-2 text-sm focus:border-blue-400 focus:outline-none"
// auth inputs use rounded-xl
```

### Cards

```tsx
// List row (connected account, search result)
className="flex items-center gap-3 rounded-xl border border-slate-200 bg-white p-3"

// Interactive (provider card) — adds p-4 text-left shadow-sm transition
className="... hover:border-blue-300 hover:shadow"   // supported
className="... opacity-60"                           // unsupported
```

### Modal

```tsx
overlay: className="fixed inset-0 z-50 grid place-items-center bg-slate-900/50 p-4"  // click-outside closes
panel:   className="w-full max-w-md rounded-2xl bg-white p-6 shadow-xl"
header:  className="mb-4 flex items-center justify-between" + title text-lg font-semibold + ghost ✕
```

### Badges & tags

```tsx
// Cloud pill (with provider dot)
className="inline-flex items-center gap-1.5 rounded-full bg-slate-100 px-2 py-0.5 text-xs font-medium text-slate-600"
dot:     className="size-2 rounded-full" style={{ background: providerDot(id) }}

// Experimental tag
className="rounded bg-slate-100 px-1.5 py-0.5 text-[10px] font-semibold uppercase text-slate-400"
```

### Quota ribbon

Segmented stacked bar (`Sidebar.tsx`): `flex h-1.5 gap-0.5 overflow-hidden
rounded-full bg-slate-100`. One segment per account — width = account
total / fleet total; fill `background: providerDot(id)` at full opacity
over a 0.25-opacity track, fill width = used / total. Empty state: eight
provider dots at `opacity: 0.35` as stripes. Caption `text-xs
text-slate-500`, used figure in `font-semibold text-slate-700`.

### Empty states

- **No accounts** (`EmptyState.tsx`): centered `max-w-md`, four provider
  logos at `size-11 gap-4`, `text-2xl font-bold` heading, `text-slate-500`
  paragraph, primary pill CTA.
- **Empty folder**: `rounded-2xl border border-dashed border-slate-300
  bg-white py-16 text-center`, semibold line + muted sub.

### Toasts & loading

- Toasts are inline banners at the top of `<main>` (not fixed): success
  `rounded-xl bg-emerald-600 px-4 py-2.5 text-sm font-medium text-white`;
  neutral `rounded-xl bg-slate-900 …` · dismiss ✕ at `opacity-60` ·
  auto-dismiss after 5 s.
- Loading is plain centered text: `py-16 text-center text-sm
  text-slate-400`.

## Icons & assets

- Icons are emoji or plain glyphs. Vocabulary: nav 🗂️ 🕘 ⭐ 🗑️ · file
  types 🖼️ 🎬 🎵 📕 📄 📦 · actions 👁 ⬇ 🔗 ⇄ ✎ 🗑 · controls
  `← + − ✕ / ↗`.
- **ProviderLogo** (`ProviderLogo.tsx`): a `grid shrink-0 place-items-center`
  box wrapping `<img src="/logos/{id}.svg" class="max-h-full max-w-full
  object-contain">`; on load error it degrades to a `rounded-lg` tile
  filled with the provider color. Sizes: `size-6` sidebar · `size-8`
  rows · `size-10` default · `size-11` cards. Sources are documented in
  `web/public/logos/README.md`.
- **Brand mark**: favicon only — a gradient rounded square (`#38bdf8` →
  `#2563eb`) with a white cloud. In-app "logo" is ☁️ + `font-bold`
  wordmark.

## Interaction & states

- **Hover**: surface `hover:bg-slate-100` / `hover:bg-slate-50` · border
  accent `hover:border-blue-300` · primary darken `hover:bg-blue-700` ·
  links `hover:underline` · dark surfaces `hover:bg-white/10`.
- **Group-reveal**: row actions `opacity-0 transition
  group-hover:opacity-100`; sidebar disconnect `hidden group-hover:block`
  — actions never clutter the resting row.
- **Selection**: table row `bg-blue-50` with name `text-blue-700`;
  gallery tile `ring-4 ring-blue-500 ring-offset-2`. Single-click
  selects, double-click opens, Enter opens.
- **Focus**: `focus:border-blue-400 focus:outline-none` — border color
  only.
- **Keyboard**: Escape closes the lightbox.
- **Destructive/renaming flows** use native `prompt()` / `confirm()`
  (rename, new folder, delete).

## Responsive

Only `sm:` and `md:` breakpoints exist.

- Below `md` the sidebar disappears; the TopBar compensates: compact
  ☁️ + wordmark (`md:hidden`), page title `hidden md:block`, mobile-only
  blue "+" pill (`md:hidden`).
- Table columns drop progressively: Cloud hidden below `sm`, Size and
  Modified below `md` (`hidden sm:table-cell` / `hidden md:table-cell`).
- Gallery reflows `grid-cols-2 → sm:grid-cols-3 → md:grid-cols-4`;
  provider grid goes 1 → `sm:grid-cols-2`.
- Padding steps up at `sm`: `p-4 sm:p-6` (main, topbar, lightbox).

## Dark surfaces

Three intentional inversions; everything else is light.

1. **Modal overlay** — `bg-slate-900/50`.
2. **Preview lightbox** (`Preview.tsx`) — `fixed inset-0 z-50
   bg-slate-950/95`, Escape-closable. Chrome: `text-white` header,
   meta `text-white/50`, actions `rounded-lg px-2 py-1 text-white/70
   hover:bg-white/10 hover:text-white`. Zoom pill: `rounded-full
   bg-black/60 px-2 py-1 text-white backdrop-blur` — the only
   `backdrop-blur` in the app. Images `max-h-full max-w-full
   object-contain`: wheel zoom ×1.15, drag-pan when zoomed, double-click
   toggles fit/1:1. Audio sits in `rounded-2xl bg-white/5 p-10` with a
   `text-5xl` 🎵. Unsupported files get `rounded-2xl bg-white/5 px-10
   py-12` + a download pill.
3. **Office previews — dark frame, white paper.** The docx host is
   `rounded-2xl bg-slate-800/60` with pages forced white and shadowed
   (`0 12px 32px rgb(0 0 0 / .45)`, index.css). Sheets render on white
   (`rounded-2xl bg-white`) with a tab strip — active `rounded-full
   bg-blue-600 text-white`, inactive `text-slate-500 hover:bg-slate-100`
   — and 12 px bordered cells (index.css).

Desktop splash (`desktop/frontend/index.html`): `#0b1220` background,
`#e5e7eb` text, spinner ring `#1f2a44` / `#60a5fa` — the only custom
keyframes in the project.

## i18n

- Dictionary is `Record<string, [en, ko]>` in `web/src/i18n.tsx`;
  `useT()` falls back to the key itself. Choice persists in
  `localStorage` (`pc-lang`), defaulting to Korean for `ko` browsers.
- Buttons are padding-based pills, so translated labels reflow without
  layout breaks; no RTL.
- Icon glyphs are baked into dictionary strings (`"+ Folder"`,
  `"⬆ Upload"`, `"🖼 Gallery"`, `"▤ List"`).
