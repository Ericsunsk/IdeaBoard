# Workspace

`workspace` 是用户端主要业务界面的路由装配与外壳布局上下文，拥有 workspace routes、layout composition、全局导航、shell controls 与 share-join 入口流程。Public `/user/*` URL 是外部产品契约，保持不变（不是兼容 shim）。

## Language

**Workspace Layout**:
用户端路由的主包裹容器（`WorkspaceLayout.vue`）。集成顶栏导航、全局反馈容器（Toast / Confirm Dialog）与主内容视图插槽。
_Avoid_: 业务页面重复声明全局通用容器

**Workspace Routes**:
用户端路由定义（`routes/workspaceRoutes.ts` 导出 `workspaceRoutes`），声明 `/user/boards`、`/user/join/:share_token` 等受保护路由，包含认证元信息（`requiresAuth: true`）与按需懒加载。
_Avoid_: 强行将非用户私有路由（如公开落地页、管理后台）混入用户端路由表

**Workspace Navigation**:
用户登录后的顶栏/主导航条（`WorkspaceNavigation.vue`），承载看板入口、灵感搜索、额度余额、用户头像及全局设置菜单。
_Avoid_: 在具体业务页面中重复实现全局导航栏

**Workspace Account Controls**:
由 `useWorkspaceAccountControls.ts` 拥有的顶层账户交互编排：协调将点击事件安全路由到 `accountOverlay.open(tab)`、登出清理（`signOut`）以及会话刷新。
_Avoid_: 导航组件直接调用底层认证存储进行深层状态修改

**Share Join Page**:
协同分享加入流程（`WorkspaceShareJoinPage.vue`）：解析 URL 中的 `share_token`，完成看板加入工作流并引导跳转至画布。
_Avoid_: 在通用页面中无序散落协作加入逻辑

## Module Ownership

| Module                                     | Responsibility                                                                | Public Interface                                     | Implementation Detail Boundary                             |
| :----------------------------------------- | :---------------------------------------------------------------------------- | :--------------------------------------------------- | :--------------------------------------------------------- |
| `routes/workspaceRoutes.ts`                | 导出用户端所有路由记录数组（`workspaceRoutes`），供全局 Vue Router 统一挂载。 | `workspaceRoutes: RouteRecordRaw[]`。                | 内部使用动态 `import()` 懒加载对应页面。                   |
| `layout/WorkspaceLayout.vue`               | 页面外壳：注入 `WorkspaceNavigation`，管理主要内容区域的视口高宽与滚动条。    | 默认布局组件。                                       | 纯布局结构，不承载特定页面的业务状态。                     |
| `pages/WorkspaceShareJoinPage.vue`         | 协同邀请落脚点：校验 Token、展示确认界面并执行加入。                          | 路由页面。                                           | 调用 `board` 域的 `joinSharedBoardWorkflow`，不自建传输。  |
| `shell/components/WorkspaceNavigation.vue` | 顶栏导航条：Logo、路由高亮、Idea 余额挂件、用户头像与下拉菜单。               | 全局布局壳组件，无外部复杂传参。                     | 折叠菜单展开态属于本地交互态。                             |
| `shell/useWorkspaceAccountControls.ts`     | 顶栏用户行为控制器：打开 Account 对话框特定标签页、退出登录。                 | `openAccountTab`、`handleSignOut`、`userMenuItems`。 | 聚合 Auth Store 与 Account Overlay，提供声明式操作菜单项。 |

跨 feature 经 root `workspace/index.ts` 消费。`shared/components` 的 `UserAccountMenu` 等身份概念组件不属于本 boundary。

## Rules

- 所有 `/user/*` 下的路由均必须具备登录态守卫校验；URL 不得更名。
- 业务内容由子模块（如 `board`）提供，`workspace` 只负责装配与外壳。
- 导航条只负责发起意图（Intent），如需打开充值或账单，必须通过 `accountOverlay` 唤起，不得直接内联账单 DOM。
- 登出后必须清理敏感本地缓存，并重定向至落地页或登录页。
- 导航项根据当前用户角色动态呈现后台管理入口。
