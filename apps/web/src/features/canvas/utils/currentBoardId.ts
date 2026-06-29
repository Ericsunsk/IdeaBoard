export type CurrentBoardIdReader = () => string | null | undefined

export function readCurrentBoardId(reader?: CurrentBoardIdReader): string {
  return `${reader?.() ?? ''}`.trim()
}

export function readCurrentBoardIdRef(source: { value: string | null | undefined }): string {
  return `${source.value ?? ''}`.trim()
}
