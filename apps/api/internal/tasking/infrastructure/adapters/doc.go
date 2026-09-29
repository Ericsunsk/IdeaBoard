// Package adapters holds the workspace-task infrastructure bridges.
//
// Application code stays port-driven: it depends on the workspacetasks
// seams (Quoter/Holder/Enqueuer) and tasking ports, never on these
// concrete implementations. The runtime composition root wires them:
//
//   - WorkspaceTaskQuoter adapts the billing catalog (server-derived SKU,
//     QuoteAtResolution/QuoteAtResolutionInTx).
//   - WorkspaceTaskHolder adapts credits.Commands, the sole ledger writer.
//   - WorkspaceTaskEnqueuer adapts the River client, joining the ambient
//     settlement transaction via InsertTx.
//
// Telemetry enters only here (trace propagation on enqueue) and in the
// River worker adapter; application services never touch it.
package adapters
