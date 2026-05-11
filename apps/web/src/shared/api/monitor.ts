import { getAdminMonitor } from './generated/admin-monitor/admin-monitor'
import type { GetAdminMonitorResult } from './generated/admin-monitor/admin-monitor'

export const monitorApi = {
  get: (): Promise<GetAdminMonitorResult> => getAdminMonitor().getAdminMonitor(),
}
