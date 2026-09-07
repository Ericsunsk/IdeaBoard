function isDescribeCallback(node, sourceCode, filename) {
  if (!/\.spec\.[cm]?[jt]sx?$/.test(filename)) return false
  const parent = node.parent
  if (parent?.type !== 'CallExpression' || !parent.arguments.includes(node)) return false
  let callee = parent.callee
  while (callee.type === 'CallExpression' || callee.type === 'MemberExpression') {
    callee = callee.type === 'CallExpression' ? callee.callee : callee.object
  }
  if (callee.type !== 'Identifier') return false
  let scope = sourceCode.getScope(callee)
  while (scope && !scope.set.has(callee.name)) scope = scope.upper
  const variable = scope?.set.get(callee.name)
  return (
    variable?.defs.some(
      (definition) =>
        definition.type === 'ImportBinding' &&
        definition.parent.source.value === 'vitest' &&
        definition.node.imported?.name === 'describe',
    ) ?? false
  )
}

/** Count each function's own code; nested bodies have their own independent budget. */
export const functionLengthRule = {
  meta: {
    type: 'suggestion',
    schema: [
      {
        type: 'object',
        properties: { max: { type: 'integer', minimum: 1 } },
        additionalProperties: false,
      },
    ],
    messages: {
      tooLong:
        'Function has {{count}} own code lines (maximum {{max}}); extract a cohesive responsibility.',
    },
  },
  create(context) {
    const sourceCode = context.sourceCode
    const max = context.options[0]?.max ?? 100
    const stack = []
    return {
      ':function'(node) {
        if (stack.length && node.body) stack.at(-1).children.push(node.body.range)
        stack.push({ children: [] })
      },
      ':function:exit'(node) {
        const frame = stack.pop()
        if (!node.body || isDescribeCallback(node, sourceCode, context.filename)) return
        const lines = new Set()
        let childIndex = 0
        for (const token of sourceCode.getTokens(node)) {
          while (frame.children[childIndex]?.[1] <= token.range[0]) childIndex++
          const child = frame.children[childIndex]
          if (child && token.range[0] >= child[0] && token.range[1] <= child[1]) continue
          for (let line = token.loc.start.line; line <= token.loc.end.line; line++) lines.add(line)
        }
        if (lines.size > max)
          context.report({ node, messageId: 'tooLong', data: { count: lines.size, max } })
      },
    }
  },
}
