import { getAdminMonitor } from './generated-query/admin-monitor/admin-monitor'

export const monitorApi = {
  get: () => getAdminMonitor(),
}
