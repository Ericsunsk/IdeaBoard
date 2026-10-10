export function readCurrentBoardIdRef(source: { value: string | null | undefined }): string {
  return `${source.value ?? ''}`.trim()
}
