# User Workspace Layout & Routes

`user` 是用户端主要业务界面的路由装配与外壳布局上下文（`/user/*`）。它将全局 `user-shell` 与具体的业务页面（Boards、Share Join 等）组合在一起，并负责用户端路由安全守卫。

## Language

**User Layout**:
用户端路由的主包裹容器（`UserLayout.vue`）。集成顶栏导航、全局反馈容器（Toast / Confirm Dialog）与主内容视图插槽。
_Avoid_: 业务页面重复声明全局通用容器

**User Routes**:
用户端路由定义（`app/routes.ts` 导出 `userRoutes`），声明 `/user/boards`、`/user/join/:share_token` 等受保护路由，包含认证元信息（`requiresAuth: true`）与按需懒加载。
_Avoid_: 强行将非用户私有路由（如公开落地页、管理后台）混入用户端路由表

**Share Join Page**:
协同分享加入流程（`UserShareJoinPage.vue`）：解析 URL 中的 `share_token`，完成看板加入工作流并引导跳转至画布。
_Avoid_: 在通用页面中无序散落协作加入逻辑

## Module Ownership

| Module | Responsibility | Public Interface | Implementation Detail Boundary |
| :--- | :--- | :--- | :--- |
| `app/routes.ts` | 导出用户端所有路由记录数组（`userRoutes`），供全局 Vue Router 统一挂载。 | `userRoutes: RouteRecordRaw[]`。 | 内部使用动态 `import()` 懒加载对应页面。 |
| `layout/UserLayout.vue` | 页面外壳：注入 `UserWorkspaceNavigation`，管理主要内容区域的视口高宽与滚动条。 | 默认布局组件。 | 纯布局结构，不承载特定页面的业务状态。 |
| `pages/UserShareJoinPage.vue` | 协同邀请落脚点：校验 Token、展示确认界面并执行加入。 | 路由页面。 | 调用 `canvas-board` 域的 `joinSharedBoardWorkflow`，不自建传输。 |

## Rules

- 所有 `/user/*` 下的路由均必须具备登录态守卫校验。
- 业务内容由子模块（如 `board-catalog`、`canvas-board`）提供，`user` 域只负责装配与外壳。
