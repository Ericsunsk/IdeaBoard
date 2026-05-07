  迁移路线

  1. 保存/加载闭环
     /user/picture-flow 支持真实 boardId 加载、修改后保存回现有 board state。
     先只保存节点位置、连线、prompt 文本这类低风险字段。
  2. 节点状态更新
     把 edit/enhance/color/fusion/wear/model/viewpoint 的参数变更写回
     snapshotNode，不接生成。目标是“拖动、改参数、保存、刷新后还原”。
  3. 节点 CRUD
     接新增、删除、复制、拖入图片、拖入节点。这里要同步处理 asset 引用清理，避
     免你之前遇到的数据库/存储膨胀。
  4. 执行链路
     逐个接生成：edit -> enhance -> color -> fusion -> wear -> model ->
     viewpoint。每接一个节点就验证输入收集、尺寸参数、mask 参数、系统 prompt、
     错误提示。
  5. Mask 生命周期
     单独处理 mask 绘制、预览、裁剪重绘、padding、保存清理。这个复杂度最高，不
     要混在普通节点 CRUD 里。
  6. 画布交互等价
     接多选、框选、快捷键、撤销/重做、导出、右键工具、缩放平移不计入历史、离开
     确认。
  7. Boards 集成
     接缩略图快照、保存后更新预览、删除画布清理存储、从 /user/boards 跳转到
     flow 页。
  8. 替换准备
     做性能对比、回归测试、保留 feature flag。确认稳定后再把 /user/picture 指
     到新实现，旧实现保留一段时间便于回滚。
