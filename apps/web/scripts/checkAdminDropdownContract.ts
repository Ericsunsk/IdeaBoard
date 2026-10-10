import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join, relative, sep } from 'node:path'

/**
 * Guard: dropdowns must share one keyboard model and, per surface, one skin.
 *
 * The Admin surface used to carry five dropdown implementations that disagreed
 * on everything:
 *
 *   - AdminSelect          reka Select     h-9  rounded-lg  admin-border
 *   - AdminOptionFilter    Popover + hand-rolled role="listbox"
 *   - AdminFilterPopover   Popover         h-9  rounded-xl  (no focus ring)
 *   - native <select>      HTML            h-8  rounded-xl  admin-border-soft
 *   - native <select>      HTML            (Canvas retouch skin)
 *
 * The Popover-based option filter was the serious one: it declared
 * `role="listbox"` and `aria-haspopup="listbox"` on a plain Popover, advertising
 * a composite widget it did not implement. There was no arrow-key navigation,
 * no typeahead, and no Home/End — keyboard and screen-reader users could not
 * actually operate it.
 *
 * Structure and keyboard behaviour now live in `@/components/ui/select`; this
 * check makes sure surfaces keep mounting that primitive instead of drifting
 * back onto Popover, and that surface skins stay in their own module.
 *
 * Command menus (an icon trigger that runs an action, e.g. UserLocaleSelect) are
 * a different widget and belong on DropdownMenu. Value pickers must not be.
 */

const SRC_ROOT = join(import.meta.dirname, '..', 'src')
/**
 * Scan root for the surface rules.
 *
 * It used to be `features/admin/shared`, which only contains the *wrappers*.
 * `role="listbox"` is declared by whoever renders the option list, i.e. by the
 * consumers, and consumers live in the admin verticals
 * (`admin/models/presentation/controls/`, `admin/monitor/presentation/components/`,
 * …). Two of them had already re-introduced the exact widget the check was
 * written to prevent — see section 2.
 */
const ADMIN_ROOT = join(SRC_ROOT, 'features/admin')
const ADMIN_SHARED = join(ADMIN_ROOT, 'shared')
const UI_SELECT = join(SRC_ROOT, 'components/ui/select')

const violations: string[] = []

/**
 * Retired listbox debt.
 *
 * Widening the scan root from `features/admin/shared` to `features/admin`, and
 * then to all of `src/`, surfaced two files that re-introduced exactly the
 * widget `AdminOptionFilter` was rebuilt to remove: a `<div role="listbox">`
 * wrapping bare `<button role="option">` children, with no arrow-key handling at
 * all. They were invisible only because the guard never looked outside the
 * wrapper directory.
 *
 *   - admin/models/.../AdminModelFilter.vue
 *   - admin/monitor/.../TaskControlToolbar.vue
 *
 * Both were ratcheted instead of left to fail the build, then migrated onto
 * `AdminSelect`, which is why there is no allowance map any more: the assertion
 * is armed again for those two files, and a regression fails the build. There is
 * deliberately nothing to re-add here — new debt must be fixed, not allowlisted.
 */

/** Every finding is recorded verbatim; section 2 applies the one real allowance. */
function recordViolation(rel: string, message: string): void {
  violations.push(`${rel}: ${message}`)
}

/** Files that may legitimately declare `role="listbox"` themselves, with the reason. */
const LISTBOX_IMPLEMENTATIONS: Record<string, string> = {
  'features/canvas/tools/retouch/ui/layers/LayersPanel.vue':
    'retouch layer list: roving tabindex + Arrow/Home/End, pinned by LayersPanel.component.spec.ts',
}

function listFiles(dir: string, extension: string): string[] {
  return readdirSync(dir).flatMap((entry) => {
    if (entry === 'node_modules' || entry === 'generated-query') return []
    const full = join(dir, entry)
    if (statSync(full).isDirectory()) return listFiles(full, extension)
    return entry.endsWith(extension) ? [full] : []
  })
}

function listVueFiles(dir: string): string[] {
  return listFiles(dir, '.vue')
}

function relOf(file: string): string {
  return relative(SRC_ROOT, file).split(sep).join('/')
}

/**
 * Strip comments before testing.
 *
 * Several components *document* the historical `role="listbox"` bug in prose
 * right next to the markup. Without this, widening the scan root to the admin
 * verticals would flag `AdminOptionFilter.vue`'s own doc comment.
 */
function stripComments(source: string): string {
  return source
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/^\s*\/\/.*$/gm, '')
}

/** A reka `Select` is mounted directly or through a surface adapter. */
const REKA_SELECT_MOUNT = /\b(?:SelectRoot|AdminSelect)\b|from ['"]@\/components\/ui\/select['"]/
/**
 * A Popover is mounted directly, through a surface adapter, or imported from
 * reka. The old test was literally `/from 'reka-ui'/`, so importing the same
 * Popover through `@/components/ui/popover` (or `<AdminPopover>`) sailed past.
 */
const POPOVER_MOUNT =
  /\b(?:PopoverRoot|PopoverContent|PopoverTrigger|Admin[A-Za-z]*Popover)\b|from ['"]@\/components\/ui\/popover['"]/

/** Any dropdown primitive in the file — used only to report how many were seen. */
const DROPDOWN_PRIMITIVE =
  /\b(?:SelectRoot|SelectTrigger|SelectContent|AdminSelect|PopoverRoot|PopoverContent|PopoverTrigger|Admin[A-Za-z]*Popover)\b/

/**
 * Hand-rolled composite-widget markers.
 *
 * A listbox that claims the APG composite model must have arrow keys, Home/End,
 * one Tab stop, and real options. Requiring all of them is what makes an
 * allowlist entry safe: it cannot rot back into a decorative div.
 */
const LISTBOX_MODEL_MARKERS = [
  { label: 'ArrowDown', pattern: /['"`]ArrowDown['"`]/ },
  { label: 'ArrowUp', pattern: /['"`]ArrowUp['"`]/ },
  { label: 'Home/End', pattern: /['"`](?:Home|End)['"`]/ },
  { label: 'roving tabindex', pattern: /:tabindex\s*=|v-bind:tabindex/ },
  { label: 'role="option"', pattern: /role=["']option["']/ },
]

function missingListboxModelMarkers(source: string): string[] {
  return LISTBOX_MODEL_MARKERS.filter((marker) => !marker.pattern.test(source)).map(
    (marker) => marker.label,
  )
}

/* ------------------------------------------------------------------ */
/* 1. One component, one dropdown primitive                             */
/* ------------------------------------------------------------------ */

/**
 * Scope: `features/admin/shared/components` only.
 *
 * These files *are* the wrappers, so a Popover and a Select in one file is
 * genuinely two primitives fighting over one trigger. A route page is a
 * different case — `AdminUsersPage` legitimately hosts a row-actions Popover
 * *and* a status Select as two separate widgets, and flagging that is noise,
 * not signal. The listbox assertions live in section 2, which runs over the
 * whole tree precisely because the consumers are not wrappers.
 */
const wrapperComponents = listVueFiles(join(ADMIN_SHARED, 'components'))

const dropdownComponents = wrapperComponents.filter((file) =>
  DROPDOWN_PRIMITIVE.test(stripComments(readFileSync(file, 'utf8'))),
)

for (const file of wrapperComponents) {
  const rel = relOf(file)
  const source = stripComments(readFileSync(file, 'utf8'))
  if (REKA_SELECT_MOUNT.test(source) && POPOVER_MOUNT.test(source)) {
    violations.push(`${rel}: mixes Popover and Select in one dropdown. Pick one primitive.`)
  }
}

/* ------------------------------------------------------------------ */
/* 2. Repo-wide: role="listbox" must be provided, not merely declared   */
/* ------------------------------------------------------------------ */

/**
 * `role="listbox"` is an APG composite promise (see apps/web/DESIGN.md,
 * "Listbox: Declare It Only If You Implement It"). AdminOptionFilter and
 * Retouch LayersPanel both broke it once; the Admin-only scan could only ever
 * see the first, because LayersPanel lives in Canvas and because the two
 * hand-rolled Admin regressions live in the *verticals*, not in
 * `features/admin/shared`.
 *
 * So this runs over the whole `src/` tree: either a reka Select supplies the
 * role, or the file is a documented implementation that still ships the model.
 */
const listboxDeclarations = new Map<string, string>()

for (const file of listVueFiles(SRC_ROOT)) {
  const rel = relOf(file)
  const source = stripComments(readFileSync(file, 'utf8'))
  if (!/role=["']listbox["']/.test(source)) continue
  listboxDeclarations.set(rel, source)
}

/**
 * `aria-haspopup="listbox"` makes the same promise from the trigger side, so it
 * is asserted here rather than in the Admin-only section: the one place that
 * shipped it (`TaskControlToolbar.vue`) sits in an admin vertical.
 */
for (const file of listVueFiles(SRC_ROOT)) {
  const rel = relOf(file)
  const source = stripComments(readFileSync(file, 'utf8'))
  if (!/aria-haspopup\s*=\s*["']listbox["']/.test(source)) continue
  if (REKA_SELECT_MOUNT.test(source)) continue
  recordViolation(
    rel,
    'declares `aria-haspopup="listbox"` without a reka Select behind it. The trigger advertises a composite widget with no arrow keys and no typeahead. Use AdminSelect/Select.',
  )
}

for (const [rel, source] of listboxDeclarations) {
  // A `role="listbox"` in the *source* is always a hand-declaration. reka's
  // Select renders the role at runtime and never spells it in the template, so
  // a file that both declares the role and mounts a Select is shipping two
  // listboxes: a hollow hand-written one plus the real portal one. That used to
  // `continue` out of the loop below and pass silently.
  const documented = LISTBOX_IMPLEMENTATIONS[rel]
  if (!documented) {
    recordViolation(
      rel,
      'declares `role="listbox"` in its own markup. That promises arrow-key navigation, Home/End and a single Tab stop. Use AdminSelect/Select (which render the role themselves), render plain buttons instead, or document the implementation in LISTBOX_IMPLEMENTATIONS.',
    )
    continue
  }

  // Keep the entry honest: an allowlist that survives after the keyboard model
  // was deleted is exactly the failure this section exists to prevent.
  const missing = missingListboxModelMarkers(source)
  if (missing.length > 0) {
    recordViolation(
      rel,
      `allowlisted listbox no longer implements the composite model (missing ${missing.join(', ')}). Either restore it or drop \`role="listbox"\`.`,
    )
  }
}

// Stale allowlist entries, mirroring checkStyleGovernance's checkStaleAllowances:
// an entry that no longer corresponds to a real listbox hides nothing and only
// teaches the next reader that the allowlist is decorative.
for (const rel of Object.keys(LISTBOX_IMPLEMENTATIONS)) {
  if (listboxDeclarations.has(rel)) continue
  violations.push(
    `${rel}: stale LISTBOX_IMPLEMENTATIONS entry — the file no longer declares \`role="listbox"\`. Remove it from the guard.`,
  )
}

/* ------------------------------------------------------------------ */
/* 3. Geometry must come from the shared skin, not be re-inlined         */
/* ------------------------------------------------------------------ */

const skinFile = join(ADMIN_SHARED, 'lib/adminSelectSkin.ts')
const skin = readFileSync(skinFile, 'utf8')

const skinConsumers = [...listVueFiles(ADMIN_ROOT), skinFile]
for (const file of skinConsumers) {
  const rel = relOf(file)
  if (rel.endsWith('lib/adminSelectSkin.ts')) continue
  const source = readFileSync(file, 'utf8')
  if (!/from ['"].*adminSelectSkin/.test(source)) continue

  // Only judge markup that is actually a dropdown. A page that also renders
  // cards or list rows must not be flagged for their own radius choices.
  for (const match of source.matchAll(/<(select|AdminSelect|SelectTrigger)\b([^>]*)>/g)) {
    const [, tag = 'dropdown', attributes = ''] = match
    if (/\brounded-(xl|2xl|md|full)\b/.test(attributes)) {
      violations.push(
        `${rel}: <${tag}> re-inlines geometry (\`rounded-xl\` etc.) while importing the shared skin. Read it from adminSelectSkin instead.`,
      )
    }
  }
}

/* ------------------------------------------------------------------ */
/* 4. The skin must actually carry the pieces that used to be missing   */
/* ------------------------------------------------------------------ */

const requiredSkinPieces = ['focus-visible:ring-2', 'rounded-lg', 'ADMIN_SELECT_SIDE_OFFSET']
for (const piece of requiredSkinPieces) {
  if (!skin.includes(piece)) {
    violations.push(
      `lib/adminSelectSkin.ts: missing \`${piece}\`. Every dropdown trigger must expose a visible focus indicator.`,
    )
  }
}

const optionFilter = readFileSync(join(ADMIN_SHARED, 'components/AdminOptionFilter.vue'), 'utf8')
if (!/AdminSelect/.test(optionFilter)) {
  violations.push(
    'components/AdminOptionFilter.vue: must build on AdminSelect so the filter dropdown inherits the full keyboard model.',
  )
}

/* ------------------------------------------------------------------ */
/* 5. The neutral primitive must stay neutral, and must stay the base   */
/* ------------------------------------------------------------------ */

for (const file of listVueFiles(UI_SELECT)) {
  const rel = relOf(file)
  const source = readFileSync(file, 'utf8')

  // A shared primitive must not carry surface semantics, or the surfaces stop
  // being able to choose their own level.
  if (/admin-|board-|canvas-/i.test(source)) {
    violations.push(
      `${rel}: the shared Select primitive references a surface token. Surface skin belongs in the surface's own adapter.`,
    )
  }
}

const adminSelect = readFileSync(join(ADMIN_SHARED, 'components/AdminSelect.vue'), 'utf8')
if (!/from '@\/components\/ui\/select'/.test(adminSelect)) {
  violations.push(
    'components/AdminSelect.vue: must mount `@/components/ui/select` instead of reka directly, so both surfaces share one keyboard model.',
  )
}
if (/\bfrom 'reka-ui'/.test(adminSelect)) {
  violations.push(
    'components/AdminSelect.vue: must not import reka Select parts directly; go through `@/components/ui/select`.',
  )
}

/* ------------------------------------------------------------------ */
/* 6. Self-test: the assertions above must reject the shapes they exist */
/*    to reject. The audit that motivated this file found two of them     */
/*    shipped while the guard was green.                                  */
/* ------------------------------------------------------------------ */

function runSelfTest(): void {
  /** Bare `<div role="listbox">` over `<button role="option">` — the shape that regressed. */
  const HAND_ROLLED_LISTBOX = `<template>
  <AdminPopover v-model:open="open">
    <AdminPopoverTrigger as-child><button aria-haspopup="listbox">x</button></AdminPopoverTrigger>
    <AdminPopoverContent>
      <div role="listbox">
        <button v-for="o in options" :key="o" role="option" :aria-selected="o === modelValue">{{ o }}</button>
      </div>
    </AdminPopoverContent>
  </AdminPopover>
</template>
<script setup lang="ts">
import { AdminPopover, AdminPopoverTrigger, AdminPopoverContent } from './atoms'
</script>`

  const cases: Array<[string, unknown, unknown]> = [
    [
      'hand-rolled listbox is rejected (no reka Select)',
      [REKA_SELECT_MOUNT.test(HAND_ROLLED_LISTBOX), /role="listbox"/.test(HAND_ROLLED_LISTBOX)],
      [false, true],
    ],
    [
      'hand-rolled listbox is missing the composite model',
      missingListboxModelMarkers(HAND_ROLLED_LISTBOX),
      ['ArrowDown', 'ArrowUp', 'Home/End', 'roving tabindex'],
    ],
    [
      'a real listbox implementation passes the model check',
      missingListboxModelMarkers(
        `<div role="listbox"><div role="option" :tabindex="t ? 0 : -1" @keydown="onKey"></div></div>` +
          `<script setup lang="ts">` +
          `if (k === 'ArrowDown' || k === 'ArrowUp' || k === 'Home' || k === 'End') move(k)</script>`,
      ),
      [],
    ],
    [
      'AdminPopover counts as Popover even without a reka-ui import',
      POPOVER_MOUNT.test(`<AdminPopover><AdminPopoverContent /></AdminPopover>`),
      true,
    ],
    [
      'the shared popover alias counts as Popover',
      POPOVER_MOUNT.test(`import { PopoverContent } from '@/components/ui/popover'`),
      true,
    ],
    [
      'AdminSelect counts as a reka Select mount',
      REKA_SELECT_MOUNT.test(`import AdminSelect from './AdminSelect.vue'`),
      true,
    ],
    [
      'a documented prose mention is not a declaration',
      /role=["']listbox["']/.test(
        stripComments(`<!-- the old implementation declared \`role="listbox"\` -->`),
      ),
      false,
    ],
  ]

  let failed = 0
  for (const [name, actual, expected] of cases) {
    const actualValue = JSON.stringify(actual)
    if (actualValue !== JSON.stringify(expected)) {
      failed += 1
      console.error(
        `[admin-dropdown-self-test] ${name}: expected ${JSON.stringify(expected)}, got ${actualValue}`,
      )
    }
  }
  if (failed > 0) {
    console.error(`\nAdmin dropdown self-test failed: ${failed} case(s).`)
    process.exit(1)
  }
  console.log(`Admin dropdown self-test passed: ${cases.length} case(s).`)
}

runSelfTest()

/* ------------------------------------------------------------------ */

if (violations.length > 0) {
  console.error('Admin dropdown contract failed.\n')
  for (const violation of violations) console.error(`- ${violation}`)
  console.error('')
  process.exit(1)
}

console.log(
  `Admin dropdown contract passed: ${dropdownComponents.length} dropdown surfaces share one skin and one keyboard model; ` +
    `${listboxDeclarations.size} listbox declaration(s) accounted for.`,
)
