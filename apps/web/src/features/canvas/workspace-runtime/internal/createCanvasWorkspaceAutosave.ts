export function createCanvasWorkspaceAutosave(options: {
  flushViewportPersistence: () => void
  flushBoardAutosave: (options?: { silent?: boolean }) => Promise<void>
}) {
  return async function flushAutosave(autosaveOptions?: { silent?: boolean }) {
    options.flushViewportPersistence()
    await options.flushBoardAutosave(autosaveOptions)
  }
}
