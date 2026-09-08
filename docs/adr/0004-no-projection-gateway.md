# ADR-0004: No Projection Gateway over Canvas Flow Adapters

- Status: Accepted
- Date: 2026-09-07

## Context

The 2026-09-07 architecture review proposed routing all Flow-projection reads
(`utils/flow*` adapters, group membership/resize/drag runtimes) through a
single "projection gateway" module, on the theory that the `animated`-is-ephemeral
invariant (see `features/canvas/CONTEXT.md`, Execution Projection Contract) had
no enforcement point and that adapter scatter required assembly knowledge.

Investigation showed the premise was wrong:

- The invariant is already enforced at two seam points:
  `vue-flow-adapter/canvasEdgeProjection.ts` (`CanvasSemanticEdge = Omit<Edge,
'animated'>`, type + runtime stripping) and
  `vue-flow-adapter/canvasGraphCommitProjection.ts` (deliberately drops
  `animated` on commit).
- History cannot record `animated` by construction: commit patches are built
  from `CanvasGraphStore` facts, which never carry the flag.
- The `utils/flow*` modules are individually focused, each with focused specs;
  `getImageNodeDisplayPayload` reads the adapter projection's embedded
  `snapshotNode`, which mirrors the store — the sanctioned pattern, not a seam
  violation.
- `useCanvasGroupRuntime` (590 lines) is cohesive group-command logic
  (selection model + create/disband/delete + click handling), not an
  assembler; the mechanics underneath were already split
  (membership/resize/drag).

## Decision

Do not build a projection gateway. A gateway over ~10 working, tested modules
would move call sites without concentrating any complexity, which the
governance plan's Non-Goals explicitly forbid
("create abstractions without a real dependency or behavior problem").

`useCanvasGroupRuntime` stays as the group-command owner. Revisit only when a
planned group-behavior change makes the facade's size block that change
(YAGNI: deepening pays off on the next edit, not in advance).

## Consequences

- Future reviews must not re-propose the gateway without new evidence of an
  unenforced projection invariant.
- Drag/selection/marquee code keeps its current shape; its regression cost
  (interaction feel, uncatchable by unit tests) stays contained behind the
  existing module seams.
- If a third `animated`-like ephemeral projection appears, prefer extending
  `canvasEdgeProjection.ts` / the commit projection over a new layer.
