/**
 * P0 naming subset: unicorn/name-replacements equivalent (plan §3.5, warn-only).
 *
 * `eslint-plugin-unicorn` is intentionally not added as a dependency in P0
 * (toolchain freeze: keep the current formatter/governance, see DEVELOPMENT.md
 * "P0 工具链冻结"). This inline `governance` rule enforces the same intent —
 * abbreviated identifiers must use the full word — on declarations only, so
 * backend-shaped object keys and DTO fields never warn.
 *
 * Kept subset (whole identifiers only): `err` -> `error`, `req` -> `request`,
 * `res` -> `response`, `cb` -> `callback`, `opts` -> `options`. `args` and
 * `e` stay out: rest/event names are idiomatic and would only add noise.
 */

const REPLACEMENTS = new Map([
  ['err', 'error'],
  ['req', 'request'],
  ['res', 'response'],
  ['cb', 'callback'],
  ['opts', 'options'],
])

function reportName(context, node, name) {
  const replacement = REPLACEMENTS.get(name)
  if (!replacement) return
  context.report({
    node,
    messageId: 'useFullWord',
    data: { name, replacement },
  })
}

function reportParam(context, param) {
  if (param.type === 'Identifier') {
    reportName(context, param, param.name)
    return
  }
  // `const { err } = ...` / `function f({ err })`: shorthand properties bind
  // the abbreviated name, so judge the bound key, not the property syntax.
  if (param.type === 'ObjectPattern') {
    for (const property of param.properties) {
      if (
        property.type === 'Property' &&
        property.shorthand &&
        property.key.type === 'Identifier'
      ) {
        reportName(context, property.key, property.key.name)
      }
    }
  }
}

export const nameReplacementsRule = {
  meta: {
    type: 'suggestion',
    schema: [],
    messages: {
      useFullWord: "Identifier '{{name}}' is abbreviated; use '{{replacement}}' instead.",
    },
  },
  create(context) {
    return {
      VariableDeclarator(node) {
        if (node.id.type === 'Identifier') reportName(context, node.id, node.id.name)
      },
      FunctionDeclaration(node) {
        if (node.id) reportName(context, node.id, node.id.name)
        for (const param of node.params) reportParam(context, param)
      },
      FunctionExpression(node) {
        for (const param of node.params) reportParam(context, param)
      },
      ArrowFunctionExpression(node) {
        for (const param of node.params) reportParam(context, param)
      },
      CatchClause(node) {
        if (node.param?.type === 'Identifier') reportName(context, node.param, node.param.name)
      },
    }
  },
}
