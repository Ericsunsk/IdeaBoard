export type CurrentBoardIdReader = () => string | null | undefined

export function readCurrentBoardIdRef(source: { value: string | null | undefined }): string {
  return `${source.value ?? ''}`.trim()
}
