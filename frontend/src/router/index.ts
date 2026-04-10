import { createRouter, createWebHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'
import type { User } from '../types'

const routes: RouteRecordRaw[] = [
  {
    path: '/',
    redirect: '/user/home'
  },
  {
    path: '/user/login',
    name: 'UserLogin',
    component: () => import('../views/user/Login.vue'),
    meta: { guest: true }
  },
  {
    path: '/user/register',
    name: 'UserRegister',
    component: () => import('../views/user/Register.vue'),
    meta: { guest: true }
  },
  {
    path: '/user',
    component: () => import('../views/user/Layout.vue'),
    children: [
      {
        path: 'home',
        name: 'UserHome',
        component: () => import('../views/user/Home.vue'),
      },
      {
        path: 'center',
        name: 'UserCenter',
        component: () => import('../views/user/Center.vue'),
        meta: { requiresAuth: true }
      },
      {
        path: 'line-draft',
        name: 'LineDraft',
        component: () => import('../views/user/LineDraft.vue'),
        meta: { requiresAuth: true }
      },
      // 服装实验室
      { path: 'sketch-to-design', name: 'SketchToDesign', component: () => import('../views/user/SketchToDesign.vue'), meta: { requiresAuth: true, title: '线稿成款', slug: 'sketch-to-design' } },
      { path: 'partial-edit', name: 'PartialEdit', component: () => import('../views/user/PartialEdit.vue'), meta: { requiresAuth: true, title: '局部改款', slug: 'partial-edit' } },
      { path: 'style-innovate', name: 'StyleInnovate', component: () => import('../views/user/StyleInnovate.vue'), meta: { requiresAuth: true, title: '款式创新', slug: 'style-innovate' } },
      { path: 'style-fusion', name: 'StyleFusion', component: () => import('../views/user/StyleFusion.vue'), meta: { requiresAuth: true, title: '风格融合', slug: 'style-fusion' } },
      { path: 'series-color', name: 'SeriesColor', component: () => import('../views/user/SeriesColor.vue'), meta: { requiresAuth: true, title: '系列配色', slug: 'series-color' } },
      { path: 'directed-recolor', name: 'DirectedRecolor', component: () => import('../views/user/DirectedRecolor.vue'), meta: { requiresAuth: true, title: '定向换色', slug: 'directed-recolor' } },
      { path: 'fabric-on-body', name: 'FabricOnBody', component: () => import('../views/user/FabricOnBody.vue'), meta: { requiresAuth: true, title: '面料上身', slug: 'fabric-on-body' } },
      { path: 'pattern-on-body', name: 'PatternOnBody', component: () => import('../views/user/PatternOnBody.vue'), meta: { requiresAuth: true, title: '图案上身', slug: 'pattern-on-body' } },
      // 摄影商拍
      { path: 'multi-angle', name: 'MultiAngle', component: () => import('../views/user/MultiAngle.vue'), meta: { requiresAuth: true, title: '模特多角度', slug: 'multi-angle' } },
      { path: 'virtual-3d', name: 'Virtual3D', component: () => import('../views/user/Virtual3D.vue'), meta: { requiresAuth: true, title: '虚拟 3D', slug: 'virtual-3d' } },
      { path: 'mannequin-to-model', name: 'MannequinToModel', component: () => import('../views/user/MannequinToModel.vue'), meta: { requiresAuth: true, title: '人台变模特', slug: 'mannequin-to-model' } },
      { path: 'face-swap', name: 'FaceSwap', component: () => import('../views/user/FaceSwap.vue'), meta: { requiresAuth: true, title: '定向换脸', slug: 'face-swap' } },
      { path: 'bg-swap', name: 'BgSwap', component: () => import('../views/user/BgSwap.vue'), meta: { requiresAuth: true, title: '换背景', slug: 'bg-swap' } },
      { path: 'garment-on-model', name: 'GarmentOnModel', component: () => import('../views/user/GarmentOnModel.vue'), meta: { requiresAuth: true, title: '款式上身', slug: 'garment-on-model' } },
      // 图案工作室
      { path: 'pattern-gen', name: 'PatternGen', component: () => import('../views/user/PatternStudio.vue'), meta: { requiresAuth: true, title: '图案生成', slug: 'pattern-gen' } },
      { path: 'style-clone', name: 'StyleClone', component: () => import('../views/user/StyleClone.vue'), meta: { requiresAuth: true, title: '画风复刻', slug: 'style-clone' } },
      { path: 'seamless-tile', name: 'SeamlessTile', component: () => import('../views/user/SeamlessTile.vue'), meta: { requiresAuth: true, title: '四方连续', slug: 'seamless-tile' } },
      { path: 'pattern-fusion', name: 'PatternFusion', component: () => import('../views/user/PatternFusion.vue'), meta: { requiresAuth: true, title: '图案融合', slug: 'pattern-fusion' } },
      { path: 'craft-fusion', name: 'CraftFusion', component: () => import('../views/user/CraftFusion.vue'), meta: { requiresAuth: true, title: '工艺融合', slug: 'craft-fusion' } },
      { path: 'pattern-inpaint', name: 'PatternInpaint', component: () => import('../views/user/PatternInpaint.vue'), meta: { requiresAuth: true, title: '图案涂改', slug: 'pattern-inpaint' } },
      { path: 'pattern-upscale', name: 'PatternUpscale', component: () => import('../views/user/PatternStudio.vue'), meta: { requiresAuth: true, title: '图案变清晰', slug: 'pattern-upscale' } },
      // AI 工具箱
      { path: 'to-vector', name: 'ToVector', component: () => import('../views/user/ToVector.vue'), meta: { requiresAuth: true, title: '转矢量图', slug: 'to-vector' } },
      { path: 'remove-bg', name: 'RemoveBg', component: () => import('../views/user/FeaturePage.vue'), meta: { requiresAuth: true, title: '抠图褪底', slug: 'remove-bg' } },
      { path: 'hd-upscale', name: 'HdUpscale', component: () => import('../views/user/FeaturePage.vue'), meta: { requiresAuth: true, title: '高清放大', slug: 'hd-upscale' } },
      { path: 'deblur', name: 'Deblur', component: () => import('../views/user/FeaturePage.vue'), meta: { requiresAuth: true, title: '模糊变清晰', slug: 'deblur' } },
      { path: 'canvas-extend', name: 'CanvasExtend', component: () => import('../views/user/FeaturePage.vue'), meta: { requiresAuth: true, title: '画布延伸', slug: 'canvas-extend' } },
    ]
  },
  {
    path: '/admin',
    component: () => import('../views/admin/Layout.vue'),
    children: [
      {
        path: '',
        redirect: '/admin/users'
      },
      {
        path: 'login',
        name: 'AdminLogin',
        component: () => import('../views/admin/Login.vue'),
        meta: { guest: true }
      },
      {
        path: 'users',
        name: 'AdminUsers',
        component: () => import('../views/admin/Users.vue'),
        meta: { requiresAuth: true }
      },
      {
        path: 'accounts',
        name: 'AdminAccounts',
        component: () => import('../views/admin/Accounts.vue'),
        meta: { requiresAuth: true }
      },
      {
        path: 'monitor',
        name: 'AdminMonitor',
        component: () => import('../views/admin/Monitor.vue'),
        meta: { requiresAuth: true }
      },
      {
        path: 'style-cards',
        name: 'AdminStyleCards',
        component: () => import('../views/admin/StyleCards.vue'),
        meta: { requiresAuth: true }
      },
      {
        path: 'drawing-records',
        name: 'AdminDrawingRecords',
        component: () => import('../views/admin/DrawingRecords.vue'),
        meta: { requiresAuth: true }
      },
      {
        path: 'channels',
        name: 'AdminChannels',
        component: () => import('../views/admin/Channels.vue'),
        meta: { requiresAuth: true }
      },
      {
        path: 'oss-config',
        name: 'AdminOSSConfig',
        component: () => import('../views/admin/OSSConfig.vue'),
        meta: { requiresAuth: true }
      },
      {
        path: 'seg-config',
        name: 'AdminSegConfig',
        component: () => import('../views/admin/SegConfig.vue'),
        meta: { requiresAuth: true }
      },
      {
        path: 'line-draft-config',
        name: 'AdminLineDraftConfig',
        component: () => import('../views/admin/LineDraftConfig.vue'),
        meta: { requiresAuth: true }
      },
      {
        path: 'face-library',
        name: 'AdminFaceLibrary',
        component: () => import('../views/admin/FaceLibrary.vue'),
        meta: { requiresAuth: true }
      },
      {
        path: 'bg-library',
        name: 'AdminBgLibrary',
        component: () => import('../views/admin/BgLibrary.vue'),
        meta: { requiresAuth: true }
      },
      {
        path: 'model-library',
        name: 'AdminModelLibrary',
        component: () => import('../views/admin/ModelLibrary.vue'),
        meta: { requiresAuth: true }
      },
      {
        path: 'pose-library',
        name: 'AdminPoseLibrary',
        component: () => import('../views/admin/PoseLibrary.vue'),
        meta: { requiresAuth: true }
      },
      {
        path: 'pattern-style-library',
        name: 'AdminPatternStyleLibrary',
        component: () => import('../views/admin/PatternStyleLibrary.vue'),
        meta: { requiresAuth: true }
      },
      {
        path: 'feature-prompts',
        name: 'AdminFeaturePrompts',
        component: () => import('../views/admin/FeaturePromptConfig.vue'),
        meta: { requiresAuth: true }
      }
    ]
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

function readStoredUser(): User | null {
  const userStr = localStorage.getItem('user')
  if (!userStr) {
    return null
  }

  try {
    return JSON.parse(userStr) as User
  } catch {
    localStorage.removeItem('user')
    return null
  }
}

router.beforeEach((to, from, next) => {
  const token = localStorage.getItem('token')
  const user = readStoredUser()

  const isAdminRoute = to.path.startsWith('/admin')

  if (to.meta.requiresAuth && !token) {
    if (isAdminRoute) {
      next('/admin/login')
    } else {
      next('/user/login')
    }
  } else if (to.meta.guest && token) {
    if (isAdminRoute) {
      next('/admin/users')
    } else {
      next('/user/home')
    }
  } else if (isAdminRoute && user && user.role !== 'admin') {
    next('/user/home')
  } else {
    next()
  }
})

export default router
