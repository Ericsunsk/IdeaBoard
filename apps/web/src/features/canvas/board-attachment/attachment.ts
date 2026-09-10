declare const boardAttachmentBrand: unique symbol
declare const boardSessionIdentityBrand: unique symbol

export type BoardSessionIdentity = Readonly<{
  [boardSessionIdentityBrand]: true
}>

/**
 * Opaque identity for one live Canvas Board attachment.
 *
 * It guards every asynchronous Board mutation, independent of the feature
 * which initiated the work (Task execution, Library materialization, etc.).
 */
export type BoardAttachment = Readonly<{
  boardId: string
  epoch: number
  sessionIdentity: BoardSessionIdentity
  [boardAttachmentBrand]: true
}>
