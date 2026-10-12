export function createCanvasWorkspaceAutosave(options: {
  flushViewportPersistence: () => void
  flushBoardAutosave: (options?: { isSilent?: boolean }) => Promise<void>
}) {
  return async function flushAutosave(autosaveOptions?: { isSilent?: boolean }) {
    options.flushViewportPersistence()
    await options.flushBoardAutosave(autosaveOptions)
  }
}
