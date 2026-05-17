import {
  createAdminAccount,
  deleteAdminAccount,
  listAdminAccounts,
  startAdminAccount,
  stopAdminAccount,
  updateAdminAccount,
} from './generated-query/admin-accounts/admin-accounts'
import type { AdminAccount } from './generated-query/model'

export type ApiAccount = AdminAccount

export const createAccount = createAdminAccount

export const updateAccount = updateAdminAccount

export const deleteAccount = deleteAdminAccount

export const listAccounts = listAdminAccounts

export const startAccount = startAdminAccount

export const stopAccount = stopAdminAccount

export const accountApi = {
  createAccount,
  updateAccount,
  deleteAccount,
  listAccounts,
  startAccount,
  stopAccount,
}

export default accountApi
