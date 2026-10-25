import common from './common'
import auth from './auth'
import search from './search'
import canvas from './canvas'
import library from './library'
import billing from './billing'
import errorCodes from './errors'
import workspace from './workspace'
import account from './account'
import board from './board'
import admin from './admin'
import notification from './notification'

export default {
  common,
  auth,
  search,
  canvas,
  library,
  billing,
  errorCodes,
  workspace,
  account,
  board,
  admin,
  notification,
} as const

export {
  common,
  auth,
  search,
  canvas,
  library,
  billing,
  errorCodes,
  workspace,
  account,
  board,
  admin,
  notification,
}
