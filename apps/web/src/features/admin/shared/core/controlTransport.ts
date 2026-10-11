// Control Plane transport prefix for Admin feature URL building (#863).
//
// Single owner for the `/control/v1` prefix on the feature side: Admin
// resources that build absolute Control URLs (logo binaries, raw protocol
// exceptions outside the JSON envelope) import it from here, never from
// `api/controlMutator`. That module stays a private orval codegen
// transport entry (pinned by `orval.config.ts` and
// `scripts/checkApiGeneratedDrift.ts`): `shared` must not export
// Admin-only control mutation helpers, and `shared` must not import
// concrete feature modules, so the literal intentionally appears in both
// places. `controlTransport.unit.spec.ts` pins this value; if the Control
// prefix ever changes, update both modules together.
export const CONTROL_API_PREFIX = '/control/v1'
