export const queryKeys = {
  tasks: {
    all: ['tasks'] as const,
    detail: (taskId: string) => ['tasks', 'detail', taskId] as const,
    list: (params: { page?: number; page_size?: number }) => ['tasks', 'list', params] as const,
  },
  admin: {
    tasks: (params: { page?: number; page_size?: number; status?: string; user_id?: number; keyword?: string }) =>
      ['admin', 'tasks', params] as const,
    monitor: () => ['admin', 'monitor'] as const,
  },
} as const
