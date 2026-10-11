import common from './common'
import auth from './auth'
import search from './search'
import canvas from './canvas'
import library from './library'
import billing from './billing'
import errors from './errors'
import workspace from './workspace'
import account from './account'
import board from './board'
import admin from './admin'

export default {
  common,
  auth,
  search,
  canvas,
  library,
  billing,
  errorCodes: errors,
  workspace,
  account,
  board,
  admin,
} as const

export { common, auth, search, canvas, library, billing, errors, workspace, account, board, admin }
