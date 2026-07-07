import type { RouteRecordRaw } from 'vue-router'
import {
  SHARE_JOIN_PATH,
  USER_BOARDS_PATH,
  USER_HOME_PATH,
  USER_LOGIN_PATH as USER_LOGIN_ROUTE_PATH,
  USER_REGISTER_PATH,
  USER_ROOT_PATH,
  USER_CANVAS_PATH,
} from '../../../shared/constants/routePaths'

export { USER_BOARDS_PATH, USER_HOME_PATH, USER_REGISTER_PATH, USER_CANVAS_PATH }
export const USER_DEFAULT_PATH = USER_HOME_PATH
export const USER_LOGIN_PATH = USER_LOGIN_ROUTE_PATH

export const userRoutes: RouteRecordRaw[] = [
  {
    path: SHARE_JOIN_PATH,
    name: 'ShareJoin',
    component: () => import('../pages/UserShareJoinPage.vue'),
    meta: { requiresAuth: true },
  },
  {
    path: USER_ROOT_PATH,
    component: () => import('../pages/UserLayoutPage.vue'),
    children: [
      {
        path: 'home',
        name: 'UserHome',
        component: () => import('../pages/UserHomePage.vue'),
      },
      {
        path: 'login',
        name: 'UserLogin',
        component: () => import('../pages/UserHomePage.vue'),
        meta: { guest: true },
      },
      {
        path: 'register',
        name: 'UserRegister',
        component: () => import('../pages/UserHomePage.vue'),
        meta: { guest: true },
      },
      {
        path: 'boards',
        name: 'UserBoards',
        component: () => import('../pages/UserBoardsPage.vue'),
        meta: { requiresAuth: true },
      },
    ],
  },
  {
    path: USER_CANVAS_PATH,
    name: 'UserCanvas',
    component: () => import('../pages/UserCanvasPage.vue'),
    meta: { requiresAuth: true },
  },
]
