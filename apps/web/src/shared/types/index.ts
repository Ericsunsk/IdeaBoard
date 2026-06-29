export interface User {
  id: number
  uid: string
  username: string
  auth_user_id?: string
  avatar?: string
  role: string
  status: number
  created_at: string
  updated_at: string
}

// API 响应
export interface ApiResponse<T = unknown> {
  code: number
  msg: string
  data?: T
}
