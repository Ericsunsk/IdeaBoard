export function clonePixelBuffer(source: ImageData): ImageData {
  const data = new Uint8ClampedArray(source.data)
  const ImageDataCtor = source.constructor as {
    new (data: Uint8ClampedArray, width: number, height: number): ImageData
  }
  return new ImageDataCtor(data, source.width, source.height)
}
