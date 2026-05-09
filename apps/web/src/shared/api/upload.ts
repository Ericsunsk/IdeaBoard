import api from './client'
import type { ApiResponse } from './types'

export const uploadApi = {
  upload: (file: File, boardId?: string): Promise<ApiResponse<{ url: string }>> => {
    const form = new FormData()
    form.append('file', file)
    const normalizedBoardId = `${boardId ?? ''}`.trim()
    if (normalizedBoardId) {
      form.append('board_id', normalizedBoardId)
    }
    return api.post('/user/upload', form, { headers: { 'Content-Type': 'multipart/form-data' } }) as any
  },
  recycle: (url: string, boardId: string): Promise<ApiResponse<{ deleted: boolean }>> =>
    api.delete('/user/upload', {
      data: { url, board_id: boardId.trim() },
    }) as any,
}
