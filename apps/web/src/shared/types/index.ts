// 用户类型
export type UserUid = string // /^[A-Z0-9]{6}$/

export interface User {
  id: number
  uid: UserUid
  username: string
  auth_user_id?: string
  avatar?: string
  role: string
  status: number
  created_at: string
  updated_at: string
}

// 用户列表响应
export interface UserListResponse {
  total: number
  list: User[]
}

// API 响应
export interface ApiResponse<T = any> {
  code: number
  msg: string
  data?: T
}
