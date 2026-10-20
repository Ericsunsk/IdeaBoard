/**
 * 对话框内容唯一基类（ib 视觉）：居中定位、无内边距、ib 圆角/描边/阴影。
 * DialogContent.vue 与所有调用方共用此基类；调用方以 cn() 合并各自宽度与表面样式。
 * p-0 + overflow-hidden：内容区自带 padding（各调用方内部的 p-6 容器）。
 */
export const dialogContentBaseClass =
  'fixed top-[50%] left-[50%] z-admin-dialog grid max-h-[calc(100dvh-2rem)] translate-x-[-50%] translate-y-[-50%] gap-0 overflow-hidden rounded-overlay border border-border bg-card p-0 shadow-admin-dialog overlay-dialog-motion'
