import { WorkflowFeatureSlug } from '../../shared/api/generated-query/model/workflowFeatureSlug'

export type PublicContentStatus = 'draft' | 'published' | 'retired'

export type PublicContentKind = 'homepage' | 'use-case' | 'capability'

export type PublicFact = {
  slug: string
  name: string
  kind: PublicContentKind
  status: PublicContentStatus
  path: string
  audience: readonly string[]
  inputs: readonly string[]
  outputs: readonly string[]
  limits: readonly string[]
  workflowSlugs?: readonly WorkflowFeatureSlug[]
  redirectTo?: string
}

export type PublicContentProjection =
  | { kind: 'publish'; path: string; fact: PublicFact }
  | { kind: 'redirect'; path: string; target: string; fact: PublicFact }
  | { kind: 'gone'; path: string; fact: PublicFact }
  | { kind: 'omit'; fact: PublicFact }

export const PUBLIC_SITE_ORIGIN = 'https://ideaboard.art'

export const PUBLIC_HOME_FACT: PublicFact = {
  slug: 'home',
  name: 'IdeaBoard AI Canvas',
  kind: 'homepage',
  status: 'published',
  path: '/',
  audience: ['Visual Designers', 'Pattern Designers'],
  inputs: ['prompts', 'sketches', 'reference images'],
  outputs: ['image directions to review and refine'],
  limits: ['Pattern ideation is exploratory; production-ready seamless repeats are not promised.'],
}

// These facts stay in the registry before their editorial pages are published.
// The lifecycle projection prevents draft capabilities from becoming public URLs.
export const PUBLIC_FACT_REGISTRY: readonly PublicFact[] = [
  PUBLIC_HOME_FACT,
  {
    slug: 'pattern-ideation',
    name: 'Pattern ideation',
    kind: 'use-case',
    status: 'draft',
    path: '/use-cases/pattern-ideation',
    audience: ['Pattern Designers', 'Visual Designers'],
    inputs: ['prompts', 'sketches', 'reference images'],
    outputs: ['pattern directions to compare and refine'],
    limits: ['The first release does not claim production-ready seamless repeats.'],
    workflowSlugs: ['pattern-gen', 'style-variation', 'style-fusion', 'partial-edit', 'hd-upscale'],
  },
]

function assertCanonicalPath(path: string): void {
  if (path === '/') return
  if (!/^\/[a-z0-9]+(?:-[a-z0-9]+)*(?:\/[a-z0-9]+(?:-[a-z0-9]+)*)*$/.test(path)) {
    throw new Error(`Public content path is not canonical: ${path}`)
  }
}

export function projectPublicContentLifecycle(fact: PublicFact): PublicContentProjection {
  assertCanonicalPath(fact.path)

  if (fact.status === 'draft') {
    return { kind: 'omit', fact }
  }

  if (fact.status === 'retired') {
    if (fact.redirectTo) {
      assertCanonicalPath(fact.redirectTo)
      return { kind: 'redirect', path: fact.path, target: fact.redirectTo, fact }
    }
    return { kind: 'gone', path: fact.path, fact }
  }

  return { kind: 'publish', path: fact.path, fact }
}

export function getPublishedPublicFacts(): readonly PublicFact[] {
  return PUBLIC_FACT_REGISTRY.filter(
    (fact) => projectPublicContentLifecycle(fact).kind === 'publish',
  )
}

export function getPublicFactBySlug(slug: string): PublicFact | undefined {
  return PUBLIC_FACT_REGISTRY.find((fact) => fact.slug === slug)
}

export function isExecutableWorkflowSlug(slug: string): slug is WorkflowFeatureSlug {
  return Object.values(WorkflowFeatureSlug).includes(slug as WorkflowFeatureSlug)
}
