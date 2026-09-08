# Library

Library 是用户的个人资产库：外部素材（Unsplash/Pexels）收藏、Folder 管理、Canvas/Search 共享的存取与桥接。不复制对象到 Media Storage，素材保留 provider 归属。

## Language

**Folder**:
`{ query_key, is_system, is_virtual, isMutable }`。`all` 是虚拟聚合，`quick-saves` 是系统默认，数字 id 为用户文件夹；`isMutable = !system && !virtual`。
_Avoid_: folder as plain string, mutable system folder

**Asset**:
`LibraryAsset`（`provider: unsplash|pexels`、`source_id`、`source/preview/original_url`、`title/author/w/h`、`folder_id`）。URL 仅用于展示。
_Avoid_: owned media asset, provider-less asset

**Asset Key**:
`provider:source_id`，跨 Feed/浮岛/桥接的同一性标识。
_Avoid_: numeric id as identity, url as identity

**Search Reference**:
与 Asset 同源的引用态，用于 vision/search 与 Canvas 之间的传递，不等同于已落库资产。
_Avoid_: reference as saved asset, saved asset as reference

**Materialize**:
把 URL 资产转成受管 Media 资产（得 `mediaAssetId`）供 Canvas 使用，是 Library 通向 Canvas 的唯一物化口。
_Avoid_: canvas-direct provider URL, unmaterialized import

**Folder Facts**:
文件夹集合的身份级快照（`folders/loading/load/refresh/reset/dispose` + revision 去重：`current/stale/disposed/error`）。多浮岛共享同一实例。
_Avoid_: per-island folder fetch, stale facts render

**Mutation Publisher**:
`save/remove` 后向所有订阅者广播（`asset-saved/removed`），驱动各 surface 的 `galleryEffect`（`none/reload-current/selection-will-reload`）。
_Avoid_: silent mutation, full reload on every change

**Transfer**:
拖拽 MIME（`application/x-ideaboard-{search-reference,library-folder-move,canvas-library-import}`）与 `copy/move/none` 语义。
_Avoid_: untyped drag payload, ambiguous drop effect

## Module Ownership

| Module                      | Responsibility                                                                                  | Public Interface                                                                                                               | Implementation Detail Boundary                                                          |
| --------------------------- | ----------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------- |
| `libraryResource.ts`        | 唯一的传输边界：folders/assets 分页、建/改名/删 folder、save/move/remove 资产、物化、key 查询。 | 底层调 `generated user-library/*`；上层只认 resource 函数。                                                                    | provider 卡片 URL（`w480`）组装在 resource，不进组件。                                  |
| `libraryFolderFacts.ts`     | 文件夹事实与 mutation 广播。                                                                    | `createLibraryFolderFacts` + `createLibraryMutationPublisher`（多 surface 共享同一实例）。                                     | revision 去重与 dispose 语义不外泄，调用方只看 `galleryEffect` + `affectedFolderKeys`。 |
| `useLibraryWorkspace.ts`    | 资产浏览用例：tabs/分页/选中/lightbox/删改/移动/重试。                                          | 入参 `{syncRoute/autoLoad/initialFolder/folderFacts/mutationPublisher}`；出 `{folders/assets/selectedFolder/... + commands}`。 | `?folder=` 归一（`all/quick-saves/<id>`）；`syncRoute:false` 时 folder 为内部态。       |
| `libraryTransfer.ts`        | DnD 读写与 effect 判定、Canvas 导入描述符。                                                     | `write/read/getLibraryDropEffect/toCanvasLibraryImportDescriptor`。                                                            | MIME 字符串是跨 surface 契约，改名必须同步三端。                                        |
| `libraryConfirmations.ts`   | 删 folder/删资产的 danger 确认。                                                                | `deleteFolderConfirmation/removeAssetConfirmation -> UserConfirmRequest`。                                                     | 文案与 intent 判定在此，不在调用方。                                                    |
| `LibraryFloatingIsland.vue` | 浮岛展示：open/expanded、引用选择、add-to-canvas。                                              | props `{open/folderFacts/mutationPublisher/referenceKeys?/presentation}` + emits。                                             | 删除选中文件夹回落 `quick-saves`；`activeAsset` lightbox 为内部态。                     |

## Rules

- 文件夹与资产的事实主是服务端（`user-library`，分页 `limit 24 + cursor`）；`selectedFolder` 平时走 `?folder=`，无路由场景才用内部态。
- 写操作必须经 mutation publisher 广播；各 surface 按 `galleryEffect` 响应，禁止每次全量 reload。
- Canvas 消费 Library 资产必须 materialize；Search 桥接只传引用（`libraryCoordinator`），不直接读写 gallery。
- Folder 删除是 danger 确认；选中文件夹被删后回落 `quick-saves`，不得悬空。
