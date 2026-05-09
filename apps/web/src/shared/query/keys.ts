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
    usersAll: () => ['admin', 'users'] as const,
    users: (params?: { keyword?: string; page?: number; page_size?: number }) =>
      ['admin', 'users', params ?? {}] as const,
    registrationSetting: () => ['admin', 'registration-setting'] as const,
    rateLimitSettings: () => ['admin', 'rate-limit-settings'] as const,
    featurePrompts: () => ['admin', 'feature-prompts'] as const,
    statistics: (range: string) => ['admin', 'statistics', range] as const,
    channels: () => ['admin', 'channels'] as const,
    accountsAll: () => ['admin', 'accounts'] as const,
    accounts: (params?: { keyword?: string; platform?: string; page?: number; page_size?: number }) =>
      ['admin', 'accounts', params ?? {}] as const,
    storage: {
      providers: () => ['admin', 'storage', 'providers'] as const,
      profiles: () => ['admin', 'storage', 'profiles'] as const,
      bucketInfo: (profileId: string) => ['admin', 'storage', 'bucket-info', profileId] as const,
    },
  },
} as const
