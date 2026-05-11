import { apiDelete, apiPost } from './client'
import type { ApiResponse } from './types'

export const uploadApi = {
  upload: (file: File, boardId?: string): Promise<ApiResponse<{ url: string }>> => {
    const form = new FormData()
    form.append('file', file)
    const normalizedBoardId = `${boardId ?? ''}`.trim()
    if (normalizedBoardId) {
      form.append('board_id', normalizedBoardId)
    }
    return apiPost<{ url: string }, FormData>('/user/upload', form, {
      headers: { 'Content-Type': 'multipart/form-data' },
    })
  },
  recycle: (url: string, boardId: string): Promise<ApiResponse<{ deleted: boolean }>> =>
    apiDelete<{ deleted: boolean }>('/user/upload', { data: { url, board_id: boardId.trim() } }),
}
