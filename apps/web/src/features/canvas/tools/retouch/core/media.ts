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

function isAnimatedPngHeader(bytes: Uint8Array) {
  if (!startsWithSignature(bytes, PNG_SIGNATURE)) return false

  let offset = PNG_SIGNATURE.length
  while (offset + 8 <= bytes.length) {
    const chunkLength = readUint32(bytes, offset, false)
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
    const chunkLength = readUint32(bytes, offset + 4, true)
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
  return String.fromCharCode(...bytes.subarray(offset, offset + length))
}

function readUint32(bytes: Uint8Array, offset: number, littleEndian: boolean) {
  if (offset + 4 > bytes.length) return 0
  return new DataView(bytes.buffer, bytes.byteOffset + offset, 4).getUint32(0, littleEndian)
}
