import fs from 'node:fs'
import path from 'node:path'
import ts from 'typescript'
import { loadSeamInventory } from './seamInventory.ts'

const seams = loadSeamInventory().seams
const canonicalEntries = new Set(
  seams.filter((seam) => seam.kind === 'canonical').map((seam) => seam.path),
)
const baselinePath = new URL('./booleanNaming.baseline.json', import.meta.url)
const baseline = new Set(JSON.parse(fs.readFileSync(baselinePath, 'utf8')).declarations)

function relativeFile(context) {
  return path.relative(context.cwd, context.filename).split(path.sep).join('/')
}

function resolvedLocal(context, specifier) {
  const base = specifier.startsWith('@/')
    ? path.join(context.cwd, 'src', specifier.slice(2))
    : specifier.startsWith('.')
      ? path.resolve(path.dirname(context.filename), specifier)
      : null
  if (!base) return null
  return [base, `${base}.ts`, `${base}.vue`, path.join(base, 'index.ts')].find(
    (file) => fs.existsSync(file) && fs.statSync(file).isFile(),
  )
}

export const noBarrelImportRule = {
  meta: {
    type: 'suggestion',
    schema: [],
    messages: {
      cascading:
        'Cascading barrel {{entry}} forwards another ownership layer; use its declared canonical entry or direct owned module.',
    },
  },
  create(context) {
    function check(source) {
      if (typeof source?.value !== 'string') return
      const resolved = resolvedLocal(context, source.value)
      if (!resolved) return
      const entry = path.relative(context.cwd, resolved).split(path.sep).join('/')
      if (!entry.endsWith('/index.ts') || canonicalEntries.has(entry)) return
      context.report({ node: source, messageId: 'cascading', data: { entry } })
    }
    return {
      ImportDeclaration(node) {
        check(node.source)
      },
      ImportExpression(node) {
        check(node.source)
      },
    }
  },
}

function declarationOwner(node) {
  const owners = []
  for (let current = node.parent; current; current = current.parent) {
    if (
      [
        'FunctionDeclaration',
        'FunctionExpression',
        'ClassDeclaration',
        'TSInterfaceDeclaration',
        'TSTypeAliasDeclaration',
      ].includes(current.type) &&
      current.id?.name
    )
      owners.push(`${current.type}:${current.id.name}`)
    else if (
      current.type === 'VariableDeclarator' &&
      current.id.type === 'Identifier' &&
      current.id !== node
    )
      owners.push(`binding:${current.id.name}`)
    else if (current.type === 'Property' && !current.computed && current.key.type === 'Identifier')
      owners.push(`property:${current.key.name}`)
  }
  return owners.reverse().join('/') || '<module>'
}

function isBooleanType(type) {
  const members = type.isUnion()
    ? type.types.filter((part) => !(part.flags & (ts.TypeFlags.Undefined | ts.TypeFlags.Null)))
    : [type]
  return (
    members.length > 0 && members.every((part) => Boolean(part.flags & ts.TypeFlags.BooleanLike))
  )
}

function isExternalDeclaration(declaration) {
  const file = declaration.getSourceFile().fileName
  if (file.includes('/node_modules/') || file.includes('/src/api/generated-query/')) return true
  for (let owner = declaration; owner; owner = owner.parent) {
    if (ts.getJSDocTags(owner).some((tag) => tag.tagName.text === 'externalContract')) return true
    if (ts.isTypeAliasDeclaration(owner) || ts.isInterfaceDeclaration(owner)) break
  }
  return false
}

function hasExternalProperty(type, name) {
  if (!type) return false
  if (type.isUnionOrIntersection())
    return type.types.some((member) => hasExternalProperty(member, name))
  return type.getProperty(name)?.declarations?.some(isExternalDeclaration) ?? false
}

function isDictionaryEntry(checker, type, name) {
  if (!type) return false
  if (type.getProperty(name)) return false
  if (type.isUnionOrIntersection())
    return type.types.some((member) => isDictionaryEntry(checker, member, name))
  return Boolean(checker.getIndexTypeOfType(type, ts.IndexKind.String))
}

function isVuePropOption(checker, node, name) {
  if (name !== 'default' && name !== 'required') return false
  const option = node.parent?.parent
  const prop = option?.parent
  const props = prop?.parent?.parent
  if (
    !prop ||
    !ts.isPropertyAssignment(prop) ||
    !props ||
    !ts.isPropertyAssignment(props) ||
    props.name.getText() !== 'props'
  )
    return false
  const call = props.parent?.parent
  if (!call || !ts.isCallExpression(call)) return false
  const declaration = checker.getResolvedSignature(call)?.declaration
  return (
    declaration?.name?.getText() === 'defineComponent' &&
    declaration.getSourceFile().fileName.includes('/node_modules/@vue/runtime-core/')
  )
}

function propsDefaultType(checker, object) {
  const call = object.parent
  if (!ts.isCallExpression(call) || call.expression.getText() !== 'withDefaults') return undefined
  const props = call.arguments[0]
  return props && ts.isCallExpression(props) && props.typeArguments?.[0]
    ? checker.getTypeFromTypeNode(props.typeArguments[0])
    : undefined
}

export const booleanPrefixRule = {
  meta: {
    type: 'suggestion',
    schema: [],
    messages: {
      prefix:
        'New boolean declaration {{identity}} requires an is/has/can/should or equivalent predicate prefix.',
    },
  },
  create(context) {
    const services = context.sourceCode.parserServices
    if (!services?.program || !services.esTreeNodeToTSNodeMap)
      throw new Error('boolean-prefix requires type-aware parser services')
    const checker = services.program.getTypeChecker()
    const occurrences = new Map()
    function check(node, declarationKind) {
      if (
        !node ||
        node.type !== 'Identifier' ||
        /^_/.test(node.name) ||
        /^(?:is|has|can|should|will|did|does|was|were|are|needs|allows|supports)[A-Z_]/.test(
          node.name,
        ) ||
        /^(?:IS|HAS|CAN|SHOULD|WILL|DID|DOES|WAS|WERE|ARE|NEEDS|ALLOWS|SUPPORTS)_/.test(node.name)
      )
        return
      const tsNode = services.esTreeNodeToTSNodeMap.get(node)
      if (declarationKind === 'member') {
        if (isExternalDeclaration(tsNode.parent)) return
        const container = tsNode.parent?.parent
        if (container && hasExternalProperty(checker.getTypeAtLocation(container), node.name))
          return
      }
      // A contextual package/DOM options key is somebody else's public API.
      if (
        declarationKind === 'property' &&
        tsNode.parent &&
        (ts.isPropertyAssignment(tsNode.parent) || ts.isShorthandPropertyAssignment(tsNode.parent))
      ) {
        const object = tsNode.parent.parent
        const contextual = checker.getContextualType(object)
        if (
          hasExternalProperty(contextual, node.name) ||
          isDictionaryEntry(checker, contextual, node.name) ||
          hasExternalProperty(propsDefaultType(checker, object), node.name) ||
          isVuePropOption(checker, tsNode, node.name)
        )
          return
      }
      const type = checker.getTypeAtLocation(tsNode)
      const value = type.getProperty('value')
      const valueType = value
        ? checker.getTypeOfSymbolAtLocation(value, services.esTreeNodeToTSNodeMap.get(node))
        : null
      if (!isBooleanType(type) && !(valueType && isBooleanType(valueType))) return
      const base = `${relativeFile(context)}|${declarationOwner(node)}|${declarationKind}:${node.name}`
      const ordinal = (occurrences.get(base) ?? 0) + 1
      occurrences.set(base, ordinal)
      const identity = `${base}|${ordinal}`
      if (baseline.has(identity)) return
      context.report({ node, messageId: 'prefix', data: { identity } })
    }
    function parameters(node) {
      for (const parameter of node.params) bindings(parameter, 'parameter')
    }
    function bindings(node, kind) {
      if (!node) return
      if (node.type === 'Identifier') check(node, kind)
      else if (node.type === 'AssignmentPattern') bindings(node.left, kind)
      else if (node.type === 'RestElement') bindings(node.argument, kind)
      else if (node.type === 'TSParameterProperty') bindings(node.parameter, kind)
      else if (node.type === 'ArrayPattern')
        node.elements.forEach((element) => bindings(element, kind))
      else if (node.type === 'ObjectPattern')
        node.properties.forEach((property) =>
          bindings(property.type === 'RestElement' ? property.argument : property.value, kind),
        )
    }
    return {
      VariableDeclarator(node) {
        bindings(node.id, 'variable')
      },
      FunctionDeclaration: parameters,
      FunctionExpression: parameters,
      ArrowFunctionExpression: parameters,
      TSPropertySignature(node) {
        if (!node.computed) check(node.key, 'member')
      },
      PropertyDefinition(node) {
        if (!node.computed) check(node.key, 'member')
      },
      Property(node) {
        if (!node.computed && node.parent.type === 'ObjectExpression') check(node.key, 'property')
      },
    }
  },
}
