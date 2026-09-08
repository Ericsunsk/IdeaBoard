# User Shell

User Shell 是用户端的主导航外壳与通用布局容器。它承载顶栏导航、全局快捷操作入口、用户头像菜单、额度展示挂件、以及触发登录/账户弹窗的跨域协调。

## Language

**Workspace Navigation**:
用户登录后的顶栏/主导航条（`UserWorkspaceNavigation.vue`），承载看板入口、灵感搜索、额度余额、用户头像及全局设置菜单。
_Avoid_: 在具体业务页面中重复实现全局导航栏

**User Account Controls**:
由 `useUserAccountControls.ts` 拥有的顶层账户交互编排：协调将点击事件安全路由到 `accountOverlay.open(tab)`、登出清理（`signOut`）以及会话刷新。
_Avoid_: 导航组件直接调用底层认证存储进行深层状态修改

**Global Brand / Logo**:
遵循 `useSiteBranding` 的站点品牌与动态 Logo 呈现，兼顾响应式抽屉折叠。
_Avoid_: 硬编码站点名称或 Logo 图片链接

## Module Ownership

| Module | Responsibility | Public Interface | Implementation Detail Boundary |
| :--- | :--- | :--- | :--- |
| `components/UserWorkspaceNavigation.vue` | 顶栏导航条：Logo、路由高亮、Idea 余额挂件、用户头像与下拉菜单。 | 全局布局壳组件，无外部复杂传参。 | 折叠菜单展开态属于本地交互态。 |
| `useUserAccountControls.ts` | 顶栏用户行为控制器：打开 Account 对话框特定标签页、退出登录。 | `openAccountTab`、`handleSignOut`、`userMenuItems`。 | 聚合 Auth Store 与 Account Overlay，提供声明式操作菜单项。 |

## Rules

- 导航条只负责发起意图（Intent），如需打开充值或账单，必须通过 `accountOverlay` 唤起，不得直接内联账单 DOM。
- 登出后必须清理敏感本地缓存，并重定向至落地页或登录页。
- 导航项根据当前用户角色（`user / admin`）动态呈现后台管理入口。
