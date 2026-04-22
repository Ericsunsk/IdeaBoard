export type EyeDropperResult = { sRGBHex: string }
export type EyeDropperInstance = { open: () => Promise<EyeDropperResult> }
export type EyeDropperConstructor = new () => EyeDropperInstance

export function getEyeDropperConstructor(): EyeDropperConstructor | null {
  const constructorCandidate = (window as unknown as { EyeDropper?: EyeDropperConstructor }).EyeDropper
  return typeof constructorCandidate === 'function' ? constructorCandidate : null
}

export function isEyeDropperSupported(): boolean {
  return getEyeDropperConstructor() !== null
}

export async function pickHexColor(): Promise<string | null> {
  const EyeDropperCtor = getEyeDropperConstructor()
  if (!EyeDropperCtor) return null
  const eyeDropper = new EyeDropperCtor()
  const result = await eyeDropper.open()
  return result.sRGBHex || null
}
