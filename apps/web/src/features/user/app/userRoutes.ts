import type { RouteRecordRaw } from 'vue-router'

export const USER_DEFAULT_PATH = '/user/home'
export const USER_LOGIN_PATH = '/user/login'

export const userRoutes: RouteRecordRaw[] = [
  {
    path: '/share',
    name: 'ShareJoin',
    component: () => import('../pages/UserShareJoinPage.vue'),
    meta: { requiresAuth: true },
  },
  {
    path: '/user',
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
      {
        path: 'center',
        name: 'UserCenter',
        component: () => import('../pages/UserCenterPage.vue'),
        meta: { requiresAuth: true },
      },
    ],
  },
  {
    path: '/user/workspace',
    name: 'UserWorkspace',
    component: () => import('../pages/UserWorkspacePage.vue'),
    meta: { requiresAuth: true },
  },
]
