declare const canvasBoardAttachmentBrand: unique symbol
declare const canvasBoardSessionIdentityBrand: unique symbol

export type CanvasBoardSessionIdentity = Readonly<{
  [canvasBoardSessionIdentityBrand]: true
}>

/**
 * Opaque identity for one live Canvas Board attachment.
 *
 * It guards every asynchronous Board mutation, independent of the feature
 * which initiated the work (Task execution, Library materialization, etc.).
 */
export type CanvasBoardAttachment = Readonly<{
  boardId: string
  epoch: number
  sessionIdentity: CanvasBoardSessionIdentity
  [canvasBoardAttachmentBrand]: true
}>
