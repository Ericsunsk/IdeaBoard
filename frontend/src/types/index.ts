// 用户类型
export interface User {
  id: number
  username: string
  role: string
  status: number
  created_at: string
  updated_at: string
}

// 创建用户请求
export interface CreateUserRequest {
  username: string
  password: string
}

// 登录请求
export interface LoginRequest {
  username: string
  password: string
}

// 注册请求
export interface RegisterRequest {
  username: string
  password: string
}

// 登录响应
export interface LoginResponse {
  token: string
  user: User
}

// 修改密码请求
export interface UpdatePasswordRequest {
  old_password: string
  new_password: string
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
