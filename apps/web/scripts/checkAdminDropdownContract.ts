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
const ADMIN_SHARED = join(SRC_ROOT, 'features/admin/shared')
const UI_SELECT = join(SRC_ROOT, 'components/ui/select')

const violations: string[] = []

function listVueFiles(dir: string): string[] {
  return readdirSync(dir).flatMap((entry) => {
    const full = join(dir, entry)
    if (statSync(full).isDirectory()) return listVueFiles(full)
    return entry.endsWith('.vue') ? [full] : []
  })
}

function relOf(file: string): string {
  return relative(SRC_ROOT, file).split(sep).join('/')
}

/* ------------------------------------------------------------------ */
/* 1. Option lists must build on reka Select, never Popover             */
/* ------------------------------------------------------------------ */

const dropdownComponents = listVueFiles(ADMIN_SHARED).filter((file) => {
  const source = readFileSync(file, 'utf8')
  return /SelectRoot|PopoverRoot/.test(source)
})

for (const file of dropdownComponents) {
  const rel = relOf(file)
  const source = readFileSync(file, 'utf8')
  const usesPopover = /from 'reka-ui'/.test(source) && /\bPopover(Content|Root|Trigger)\b/.test(source)
  const usesSelect = /\bSelectRoot\b/.test(source)

  // A Popover that hosts anything list-like must not claim listbox semantics.
  if (/aria-haspopup\s*=\s*["']listbox["']/.test(source)) {
    violations.push(
      `${rel}: declares \`aria-haspopup="listbox"\` on a Popover. That advertises a composite widget the Popover does not implement (no arrow keys, no typeahead). Use reka Select.`,
    )
  }
  if (/role="listbox"/.test(source) && !usesSelect) {
    violations.push(
      `${rel}: hand-rolls \`role="listbox"\` outside of a reka Select. Use AdminSelect so keyboard navigation comes from the primitive.`,
    )
  }
  if (usesPopover && usesSelect) {
    violations.push(
      `${rel}: mixes Popover and Select in one dropdown. Pick one primitive.`,
    )
  }
}

/* ------------------------------------------------------------------ */
/* 2. Geometry must come from the shared skin, not be re-inlined         */
/* ------------------------------------------------------------------ */

const skinFile = join(ADMIN_SHARED, 'lib/adminSelectSkin.ts')
const skin = readFileSync(skinFile, 'utf8')

const skinConsumers = [...listVueFiles(join(SRC_ROOT, 'features/admin')), skinFile]
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
/* 3. The skin must actually carry the pieces that used to be missing   */
/* ------------------------------------------------------------------ */

const requiredSkinPieces = [
  'focus-visible:ring-2',
  'rounded-lg',
  'ADMIN_SELECT_SIDE_OFFSET',
]
for (const piece of requiredSkinPieces) {
  if (!skin.includes(piece)) {
    violations.push(
      `lib/adminSelectSkin.ts: missing \`${piece}\`. Every dropdown trigger must expose a visible focus indicator.`,
    )
  }
}

const optionFilter = readFileSync(
  join(ADMIN_SHARED, 'components/AdminOptionFilter.vue'),
  'utf8',
)
if (!/AdminSelect/.test(optionFilter)) {
  violations.push(
    'components/AdminOptionFilter.vue: must build on AdminSelect so the filter dropdown inherits the full keyboard model.',
  )
}

/* ------------------------------------------------------------------ */
/* 4. The neutral primitive must stay neutral, and must stay the base   */
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

if (violations.length > 0) {
  console.error('Admin dropdown contract failed.\n')
  for (const violation of violations) console.error(`- ${violation}`)
  console.error('')
  process.exit(1)
}

console.log(
  `Admin dropdown contract passed: ${dropdownComponents.length} dropdown surfaces share one skin and one keyboard model.`,
)
