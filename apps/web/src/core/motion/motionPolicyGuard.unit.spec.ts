import { describe, expect, it } from 'vitest'
import { findMotionPolicyViolations } from '../../../scripts/motionPolicyGuard'

describe('Motion Policy style guard', () => {
  it('accepts semantic Motion Policy consumers', () => {
    expect(
      findMotionPolicyViolations(
        'src/features/auth/components/AuthAction.vue',
        '.action { transition: opacity var(--motion-feedback-duration) var(--motion-feedback-easing); }',
      ),
    ).toEqual([])
  })

  it('ignores TypeScript chart animation options', () => {
    expect(
      findMotionPolicyViolations(
        'src/features/account/session/chart.ts',
        'const options = { animation: { duration: 0 } }',
      ),
    ).toEqual([])
  })

  it('ignores policy fixtures in test sources', () => {
    expect(
      findMotionPolicyViolations(
        'src/shared/motion/motionPolicyGuard.unit.spec.ts',
        '.fixture { transition: all 200ms ease; color: var(--canvas-motion-fast); }',
      ),
    ).toEqual([])
  })

  it.each(['--canvas-motion-fast', '--duration-ib-standard', '--ease-ib-standard'])(
    'rejects the legacy %s path',
    (legacyToken) => {
      expect(
        findMotionPolicyViolations(
          'src/features/canvas/components/Legacy.vue',
          `.legacy { transition: opacity var(${legacyToken}) ease; }`,
        ).map((violation) => violation.rule),
      ).toContain('legacy-motion-token')
    },
  )

  it('rejects broad transitions and raw interaction timing', () => {
    const violations = findMotionPolicyViolations(
      'src/features/canvas/components/Legacy.vue',
      `
        .wide { transition: all 200ms ease; }
        .tailwind { @apply transition-all duration-200; }
      `,
    )

    expect(violations.map((violation) => violation.rule)).toEqual(
      expect.arrayContaining(['broad-transition', 'raw-motion-timing']),
    )
  })

  it('requires animations to provide a reduced-motion fallback', () => {
    expect(
      findMotionPolicyViolations(
        'src/features/canvas/components/Activity.vue',
        '.activity { animation: activity-cycle var(--activity-cycle-duration) linear infinite; }',
      ).map((violation) => violation.rule),
    ).toContain('missing-reduced-motion')
  })

  it('allows registered Canvas continuous motion with a local semantic and fallback', () => {
    expect(
      findMotionPolicyViolations(
        'src/features/canvas/presentation/nodes/styles/node-shared.css',
        `
          .workflow-node.running {
            --node-running-border-cycle: 2.4s;
            animation: node-running-border-flow var(--node-running-border-cycle) linear infinite;
          }
          @media (prefers-reduced-motion: reduce) {
            .workflow-node.running { animation: none; }
          }
        `,
      ),
    ).toEqual([])
  })

  it('rejects unregistered local raw timing aliases', () => {
    expect(
      findMotionPolicyViolations(
        'src/features/canvas/components/Legacy.vue',
        '.legacy { --legacy-motion-speed: 180ms; transition: opacity var(--legacy-motion-speed); }',
      ).map((violation) => violation.rule),
    ).toContain('unregistered-local-motion-timing')
  })
})
