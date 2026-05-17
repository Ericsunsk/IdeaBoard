// 用户类型（复用 generated model，避免手写重复）
export type { User } from '../api/generated-query/model'

// API 响应
export interface ApiResponse<T = unknown> {
  code: number
  msg: string
  data?: T
}
