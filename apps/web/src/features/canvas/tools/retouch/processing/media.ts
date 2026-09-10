const PNG_SIGNATURE = [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]
const ANIMATION_HEADER_SCAN_BYTES = 2 * 1024 * 1024
const ANIMATED_IMAGE_MIME_TYPES = new Set(['image/gif', 'image/apng'])

export async function isAnimatedImageBlob(blob: Blob) {
  const mime = blob.type.trim().toLowerCase()
  if (ANIMATED_IMAGE_MIME_TYPES.has(mime)) return true
  if (mime !== 'image/png' && mime !== 'image/webp') return false

  const headerBytes = new Uint8Array(
    await blob.slice(0, Math.min(blob.size, ANIMATION_HEADER_SCAN_BYTES)).arrayBuffer(),
  )
  if (mime === 'image/png') return isAnimatedPngHeader(headerBytes)
  return isAnimatedWebpHeader(headerBytes)
}

export function releaseCanvas(canvas: HTMLCanvasElement | null) {
  if (!canvas) return
  canvas.width = 0
  canvas.height = 0
}

function isAnimatedPngHeader(bytes: Uint8Array) {
  if (!startsWithSignature(bytes, PNG_SIGNATURE)) return false

  let offset = PNG_SIGNATURE.length
  while (offset + 8 <= bytes.length) {
    const chunkLength = readUint32BigEndian(bytes, offset)
    const chunkType = readAscii(bytes, offset + 4, 4)
    if (chunkType === 'acTL') return true
    if (chunkType === 'IDAT') return false
    offset += 8 + chunkLength + 4
  }
  return false
}

function isAnimatedWebpHeader(bytes: Uint8Array) {
  if (readAscii(bytes, 0, 4) !== 'RIFF' || readAscii(bytes, 8, 4) !== 'WEBP') return false

  let offset = 12
  while (offset + 8 <= bytes.length) {
    const chunkType = readAscii(bytes, offset, 4)
    const chunkLength = readUint32LittleEndian(bytes, offset + 4)
    if (chunkType === 'ANIM' || chunkType === 'ANMF') return true
    if (chunkType === 'VP8X' && offset + 9 <= bytes.length) {
      const featureFlags = bytes[offset + 8] ?? 0
      if ((featureFlags & 0x02) !== 0) return true
    }
    offset += 8 + chunkLength + (chunkLength % 2)
  }
  return false
}

function startsWithSignature(bytes: Uint8Array, signature: readonly number[]) {
  if (bytes.length < signature.length) return false
  return signature.every((value, index) => bytes[index] === value)
}

function readAscii(bytes: Uint8Array, offset: number, length: number) {
  if (offset + length > bytes.length) return ''
  let value = ''
  for (let index = offset; index < offset + length; index += 1) {
    value += String.fromCharCode(bytes[index] ?? 0)
  }
  return value
}

function readUint32BigEndian(bytes: Uint8Array, offset: number) {
  if (offset + 4 > bytes.length) return 0
  return (
    (bytes[offset] ?? 0) * 0x1000000 +
    ((bytes[offset + 1] ?? 0) << 16) +
    ((bytes[offset + 2] ?? 0) << 8) +
    (bytes[offset + 3] ?? 0)
  )
}

function readUint32LittleEndian(bytes: Uint8Array, offset: number) {
  if (offset + 4 > bytes.length) return 0
  return (
    (bytes[offset] ?? 0) +
    ((bytes[offset + 1] ?? 0) << 8) +
    ((bytes[offset + 2] ?? 0) << 16) +
    (bytes[offset + 3] ?? 0) * 0x1000000
  )
}
