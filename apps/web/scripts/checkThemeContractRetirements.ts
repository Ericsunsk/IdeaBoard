import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { parse as parseSfc } from '@vue/compiler-sfc'
import { NodeTypes, parse as parseTemplate } from '@vue/compiler-dom'

/**
 * Guard: theme contract retirements for Boards sorting and redeem error text.
 *
 * Replaces 7 source-string assertions retired from the theme contract specs:
 *
 *   - `src/theme/atoms/pageChrome.contract.spec.ts`
 *     ("uses the shared Select primitive for Boards sorting, never a command
 *     menu", 4 assertions) -> `source-policy/pageChrome.contract/a319f341`
 *     Ledger: source-2d852d08f7118359-1, source-788b512d57647892-1,
 *     source-7d479687bca2c193-1, source-6faf45e8bc066fa8-1.
 *   - `src/theme/formControls.contract.spec.ts`
 *     ("wires the redeem input to the error-text contract", 3 assertions) ->
 *     `source-policy/formControls.contract/5d0f9348`
 *     Ledger: source-251cf6984427346a-1, source-18e4419619623a52-1,
 *     source-c74ce5adb4b3f158-1.
 *
 * Why a script instead of the specs: the specs asserted raw substrings of the
 * implementation (`toContain`). That passes for dead code paths, commented-out
 * markup, or an unrelated `role="alert"` elsewhere in the file (RewardsPane
 * has three). The checks below keep the same intent but assert linkage:
 *
 *   - Boards: the sort control mounts `<Select>` from the shared
 *     `@/components/ui/select` primitive with the `sortSelectTrigger` class,
 *     and the page never mounts a command menu (`<DropdownMenu`) or the
 *     retired PrimeVue select. BoardCard's row-action DropdownMenu is a
 *     different widget (command menu, correct) and out of scope: this rule
 *     reads BoardsPage.vue only.
 *   - Redeem: the input's `aria-describedby` binding points at
 *     `account-reward-code-error` under the `redeemFailed` condition, and the
 *     element carrying that id also carries `role="alert"` in the same tag.
 *
 * The checks parse the SFC template rather than searching source text. This
 * keeps comments, strings, and unrelated controls from satisfying a rule.
 */

export type ThemeContractViolation = { rule: string; file: string; message: string }

const BOARDS_FILE = 'features/board/ui/gallery/BoardsPage.vue'
const REWARDS_FILE = 'features/account/ui/rewards/RewardsPane.vue'

export const BOARDS_SORT_SELECT_RULE = 'boards-sort-select'
export const REDEEM_ERROR_WIRING_RULE = 'redeem-error-wiring'

type TemplateNode = {
  type?: number
  tag?: string
  props?: TemplateProp[]
  children?: TemplateNode[]
}

type TemplateProp = {
  type?: number
  name?: string
  value?: { content?: string }
  exp?: { content?: string }
  arg?: { content?: string; isStatic?: boolean }
}

type ParsedSfc = { root: TemplateNode; script: string }

function parseSfcTemplate(source: string): ParsedSfc | undefined {
  try {
    const parsed = parseSfc(source, { filename: 'theme-contract-fixture.vue' })
    if (parsed.errors.length > 0 || !parsed.descriptor.template) return undefined
    const template = parseTemplate(parsed.descriptor.template.content)
    return {
      root: template as unknown as TemplateNode,
      script: [parsed.descriptor.script?.content, parsed.descriptor.scriptSetup?.content]
        .filter(Boolean)
        .join('\n'),
    }
  } catch {
    return undefined
  }
}

function walkTemplate(root: TemplateNode): TemplateNode[] {
  const nodes: TemplateNode[] = []
  const visit = (node: TemplateNode) => {
    if (node.type === NodeTypes.ELEMENT) nodes.push(node)
    for (const child of node.children ?? []) visit(child)
  }
  visit(root)
  return nodes
}

function staticProp(node: TemplateNode, name: string): string | undefined {
  const prop = (node.props ?? []).find(
    (item) => item.type === NodeTypes.ATTRIBUTE && item.name === name,
  )
  return prop?.value?.content
}

function boundProp(node: TemplateNode, name: string): string | undefined {
  const prop = (node.props ?? []).find(
    (item) =>
      item.type === NodeTypes.DIRECTIVE &&
      item.name === 'bind' &&
      item.arg?.isStatic === true &&
      item.arg.content === name,
  )
  return prop?.exp?.content
}

function eventProp(node: TemplateNode, name: string): string | undefined {
  const prop = (node.props ?? []).find(
    (item) =>
      item.type === NodeTypes.DIRECTIVE &&
      item.name === 'on' &&
      item.arg?.isStatic === true &&
      item.arg.content === name,
  )
  return prop?.exp?.content
}

function directiveProp(node: TemplateNode, name: string): string | undefined {
  const prop = (node.props ?? []).find(
    (item) => item.type === NodeTypes.DIRECTIVE && item.name === name,
  )
  return prop?.exp?.content
}

function normalizedExpression(expression: string | undefined): string {
  return (expression ?? '').replace(/\s+/g, '')
}

function hasSharedSelectImport(script: string): boolean {
  return [
    ...script.matchAll(/import\s*\{([^}]*)\}\s*from\s*['"]@\/components\/ui\/select['"]/g),
  ].some(([, names]) => /\bSelect\b/.test(names ?? '') && /\bSelectTrigger\b/.test(names ?? ''))
}

/** Boards sorting must mount the shared Select primitive, never a menu. */
export function validateBoardsSortSelect(source: string): string[] {
  const issues: string[] = []
  const parsed = parseSfcTemplate(source)
  if (!parsed) return ['Boards source must be a valid SFC with a parseable template.']
  const nodes = walkTemplate(parsed.root)
  const sortSelects = nodes.filter(
    (node) =>
      node.tag === 'Select' &&
      normalizedExpression(boundProp(node, 'model-value')) === 'sortMode' &&
      normalizedExpression(eventProp(node, 'update:model-value')) === 'onSortModeChange',
  )
  if (sortSelects.length !== 1)
    issues.push(
      'Boards sorting must mount exactly one <Select> bound to sortMode and onSortModeChange.',
    )
  const sortSelect = sortSelects[0]
  if (
    !sortSelect ||
    !walkTemplate(sortSelect).some(
      (node) =>
        node.tag === 'SelectTrigger' &&
        normalizedExpression(boundProp(node, 'class')).includes('catalogClasses.sortSelectTrigger'),
    )
  )
    issues.push(
      'Boards sorting must include a descendant SelectTrigger using catalogClasses.sortSelectTrigger.',
    )
  if (!hasSharedSelectImport(parsed.script))
    issues.push("Boards sorting must import the shared primitive from '@/components/ui/select'.")
  if (nodes.some((node) => node.tag === 'DropdownMenu'))
    issues.push(
      'Boards sorting must not mount <DropdownMenu>: sorting picks a value (Select), not a command menu.',
    )
  if (/from\s*['"]primevue\/select['"]/.test(parsed.script))
    issues.push('Boards sorting must not import the retired PrimeVue select.')
  return issues
}

/** Redeem input must wire its failure to the announced error-text element. */
export function validateRedeemErrorWiring(source: string): string[] {
  const issues: string[] = []
  const parsed = parseSfcTemplate(source)
  if (!parsed) return ['Rewards source must be a valid SFC with a parseable template.']
  const nodes = walkTemplate(parsed.root)
  const inputs = nodes.filter(
    (node) => node.tag === 'Input' && staticProp(node, 'id') === 'account-reward-code',
  )
  if (inputs.length !== 1)
    issues.push('Redeem source must contain exactly one Input with id="account-reward-code".')
  const redeemInput = inputs[0]
  if (
    normalizedExpression(boundProp(redeemInput ?? {}, 'aria-describedby')) !==
    "redeemFailed?'account-reward-code-error':undefined"
  )
    issues.push(
      "Redeem input must bind aria-describedby to 'account-reward-code-error' under the redeemFailed condition.",
    )
  const errorNodes = nodes.filter((node) => staticProp(node, 'id') === 'account-reward-code-error')
  if (errorNodes.length !== 1)
    issues.push('Redeem error text must render id="account-reward-code-error".')
  const errorNode = errorNodes[0]
  if (errorNode && staticProp(errorNode, 'role') !== 'alert')
    issues.push('The account-reward-code-error element must carry role="alert" in the same tag.')
  if (errorNode && normalizedExpression(directiveProp(errorNode, 'if')) !== 'redeemFailed')
    issues.push('The account-reward-code-error element must render only when redeemFailed is true.')
  return issues
}

export function checkThemeContractSources(
  sources: Record<string, string>,
): ThemeContractViolation[] {
  const violations: ThemeContractViolation[] = []
  for (const message of validateBoardsSortSelect(sources[BOARDS_FILE] ?? ''))
    violations.push({ rule: BOARDS_SORT_SELECT_RULE, file: BOARDS_FILE, message })
  for (const message of validateRedeemErrorWiring(sources[REWARDS_FILE] ?? ''))
    violations.push({ rule: REDEEM_ERROR_WIRING_RULE, file: REWARDS_FILE, message })
  return violations
}

/* ------------------------------------------------------------------ */
/* Self-test: every assertion above must reject the shape it exists     */
/* to reject. Runs before the production check so a green run proves    */
/* the rule is armed, not vacuous.                                      */
/* ------------------------------------------------------------------ */

const BOARDS_POSITIVE = `<template>
  <Select :model-value="sortMode" @update:model-value="onSortModeChange">
    <SelectTrigger :class="catalogClasses.sortSelectTrigger"><SelectValue /></SelectTrigger>
  </Select>
</template>
<script setup lang="ts">
import { Select, SelectTrigger, SelectValue } from '@/components/ui/select'
</script>`

const REDEEM_POSITIVE = `<template>
  <Input
    id="account-reward-code"
    :aria-describedby="redeemFailed ? 'account-reward-code-error' : undefined"
  />
  <p v-if="redeemFailed" id="account-reward-code-error" role="alert">{{ redeemErrorText }}</p>
</template>`

type Fixture = { name: string; rule: string; source: string; expectPass: boolean }

function boardsFixture(name: string, source: string, expectPass: boolean): Fixture {
  return { name, rule: BOARDS_SORT_SELECT_RULE, source, expectPass }
}

function redeemFixture(name: string, source: string, expectPass: boolean): Fixture {
  return { name, rule: REDEEM_ERROR_WIRING_RULE, source, expectPass }
}

const FIXTURES: Fixture[] = [
  boardsFixture('boards canonical Select passes', BOARDS_POSITIVE, true),
  boardsFixture(
    'boards missing shared import fails',
    BOARDS_POSITIVE.replace(" from '@/components/ui/select'", " from 'reka-ui'"),
    false,
  ),
  boardsFixture(
    'boards missing sortSelectTrigger fails',
    BOARDS_POSITIVE.replace('catalogClasses.sortSelectTrigger', 'catalogClasses.sortMenuTrigger'),
    false,
  ),
  boardsFixture(
    'boards DropdownMenu for sorting fails',
    `${BOARDS_POSITIVE}\n<template><DropdownMenu><DropdownMenuTrigger /></DropdownMenu></template>`,
    false,
  ),
  boardsFixture(
    'boards PrimeVue select fails',
    BOARDS_POSITIVE.replace("from '@/components/ui/select'", "from 'primevue/select'"),
    false,
  ),
  boardsFixture(
    'boards commented-out DropdownMenu prose passes',
    `<!-- sorting used to be a <DropdownMenu> command menu -->\n${BOARDS_POSITIVE}`,
    true,
  ),
  boardsFixture(
    'boards split Select and trigger controls fail',
    BOARDS_POSITIVE.replace(
      '<SelectTrigger :class="catalogClasses.sortSelectTrigger"><SelectValue /></SelectTrigger>',
      '',
    ).replace(
      '</template>',
      '<SelectTrigger :class="catalogClasses.sortSelectTrigger" /></template>',
    ),
    false,
  ),
  boardsFixture(
    'boards comment decoy without a real Select fails',
    `<!-- <Select :model-value="sortMode" @update:model-value="onSortModeChange"><SelectTrigger :class="catalogClasses.sortSelectTrigger" /></Select> -->`,
    false,
  ),
  redeemFixture('redeem canonical wiring passes', REDEEM_POSITIVE, true),
  redeemFixture(
    'redeem missing describedby wiring fails',
    REDEEM_POSITIVE.replace(
      `:aria-describedby="redeemFailed ? 'account-reward-code-error' : undefined"`,
      `:aria-label="codeLabel"`,
    ),
    false,
  ),
  redeemFixture(
    'redeem missing error id fails',
    REDEEM_POSITIVE.replace('id="account-reward-code-error"', 'id="account-reward-code-hint"'),
    false,
  ),
  redeemFixture(
    'redeem error without alert role fails',
    REDEEM_POSITIVE.replace(' role="alert"', ' role="presentation"'),
    false,
  ),
  redeemFixture(
    'redeem unrelated alert elsewhere does not satisfy the error element',
    REDEEM_POSITIVE.replace(' role="alert"', ' role="presentation"') +
      '\n<template><div role="alert">history load failed</div></template>',
    false,
  ),
  redeemFixture(
    'redeem static describedby fails',
    REDEEM_POSITIVE.replace(
      `:aria-describedby="redeemFailed ? 'account-reward-code-error' : undefined"`,
      `aria-describedby="account-reward-code-error"`,
    ),
    false,
  ),
  redeemFixture(
    'redeem unrelated input does not satisfy the contract',
    REDEEM_POSITIVE.replace(
      '<Input\n    id="account-reward-code"',
      '<Input id="other-input" :aria-describedby="account-reward-code-error" />\n  <Input\n    id="account-reward-code"',
    ).replace(
      `:aria-describedby="redeemFailed ? 'account-reward-code-error' : undefined"`,
      `:aria-label="codeLabel"`,
    ),
    false,
  ),
  redeemFixture(
    'redeem malformed SFC fails closed',
    `<template><Input id="account-reward-code" :aria-describedby="redeemFailed ? 'account-reward-code-error' : undefined">`,
    false,
  ),
  redeemFixture(
    'redeem comment decoy fails closed',
    `<!-- <Input id="account-reward-code" :aria-describedby="redeemFailed ? 'account-reward-code-error' : undefined" /> -->`,
    false,
  ),
]

function runSelfTest(): void {
  let failed = 0
  for (const fixture of FIXTURES) {
    const issues =
      fixture.rule === BOARDS_SORT_SELECT_RULE
        ? validateBoardsSortSelect(fixture.source)
        : validateRedeemErrorWiring(fixture.source)
    const passed = issues.length === 0
    if (passed !== fixture.expectPass) {
      failed += 1
      console.error(
        `[theme-contract-self-test] ${fixture.name}: expected ${fixture.expectPass ? 'pass' : 'fail'}, got ${passed ? 'pass' : `fail (${issues.join('; ')})`}.`,
      )
    }
  }
  if (failed > 0) {
    console.error(`\nTheme contract self-test failed: ${failed} case(s).`)
    process.exit(1)
  }
  console.log(`Theme contract self-test passed: ${FIXTURES.length} case(s).`)
}

function main(): void {
  runSelfTest()
  const root = join(import.meta.dirname, '..', 'src')
  const sources: Record<string, string> = {
    [BOARDS_FILE]: readFileSync(join(root, BOARDS_FILE), 'utf8'),
    [REWARDS_FILE]: readFileSync(join(root, REWARDS_FILE), 'utf8'),
  }
  const violations = checkThemeContractSources(sources)
  if (violations.length > 0) {
    console.error('Theme contract retirements failed.\n')
    for (const violation of violations)
      console.error(`- ${violation.file}: ${violation.rule}: ${violation.message}`)
    console.error('')
    process.exit(1)
  }
  console.log(
    'Theme contract retirements passed: Boards sorting mounts the shared Select primitive; redeem input wires the announced error text.',
  )
}

main()
