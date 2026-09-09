# Board Catalog

Board Catalog 是用户的项目看板管理工作区（路由 `/user/boards`）。它负责看板画廊展示、筛选排序、新建/重命名/删除/克隆看板用例编排，以及封面卡片交互。

## Language

**Board Card**:
画廊中的单个看板卡片（`BoardCard.vue`），承载看板缩略图、标题、修改时间、协作身份标记与操作快捷菜单。保持固定尺寸与防抖动悬浮动效。
_Avoid_: 复杂的卡片级内部状态混入父列表

**Catalog Controller**:
由 `controller/controller.ts` 拥有的看板用例编排机：负责集中协调 TanStack Query 数据获取、排序（`updated_at / name`）、模糊搜索过滤、新建向导与错误重试。
_Avoid_: 在 `BoardsPage.vue` 中直接编写 API 调用与分页过滤算法

**Board Mutation Commands**:
看板的 CRUD 原子命令集合（由 `controller/commandHandlers.ts` 实现）：创建、重命名、删除（带二次确认）、克隆与复制。写操作成功后严格触发 Query Cache 失效。
_Avoid_: 静默失败，无错误回滚与 Toast 反馈

**Catalog Skin & Motion**:
遵循 `lib/skin.ts` 的视觉与动效契约。卡片悬停位移、卡片菜单弹出遵循 Canvas/Board 一致的 Motion tokens。
_Avoid_: 引入组件私有的杂乱 CSS 变量与未受管 Transition

## Module Ownership

| Module | Responsibility | Public Interface | Implementation Detail Boundary |
| :--- | :--- | :--- | :--- |
| `index.ts` | 公开出口：`createBoardCatalog`/`useBoardCatalog`、catalog 类型、`formatCatalogDate`、`BoardsPage`。 | 外部只 import `features/board-catalog`；内部模块互引保持相对路径。 |  |
| `contract.ts` | 领域契约与类型定义：`BoardCatalogEntry`、`BoardCatalog`、`BoardCatalogSortMode` 等。 | 纯类型导出。 | 纯契约无运行时副作用。 |
| `useBoardCatalog.ts` | 画廊主 Composable 与工厂函数装配。 | `createBoardCatalog`、`useBoardCatalog`。 | 组合 controller、preview refresh 与 feedback。 |
| `pages/BoardsPage.vue` | 薄路由装配页：组合 SearchBar、CreateButton、排序下拉与 Card Grid。 | 路由懒加载出口。 | 纯视图组合，不拥有过滤排序底层算法。 |
| `components/BoardCard.vue` | 独立看板卡片：封面展示、菜单指令分发（`rename / clone / delete`）。 | Props: `{ board, isOwner, disabled }` + Emits: `{ click, action }`。 | 卡片内部交互状态（如操作菜单开关）局部自闭环。 |
| `controller/` | 状态机与用例编排（`controller.ts`、`commandHandlers.ts`）。 | `useBoardCatalog()` 暴露响应式列表、筛选参数与命令集。 | 隔离 Query 与底层传输层调用。 |
| `feedback/` | 用户反馈边界（`useFeedback.ts`）。 | 错误与成功 Toast 提示、确认弹窗契约。 | 统一通过 App Toast 出口，屏蔽底层 i18n key 拼接。 |

## Rules

- `BoardsPage.vue` 只能做组合，业务命令与列表状态必须由 `controller` 承载。
- 删除看板属于破坏性操作，必须具备明确的确认流程（Confirm Dialog）。
- 增删改操作完成后，必须精确刷新或局部失效看板列表 Query Cache，保持多页面数据一致。
