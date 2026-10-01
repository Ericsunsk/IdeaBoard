/**
 * 用户侧对话框内容基类（ib 视觉）：居中定位、无内边距、ib 圆角/描边/阴影。
 * 调用方用 micro DialogContent 组合时以 cn() 合并各自宽度与表面样式。
 * p-0 覆盖 micro 基类的 p-6：所有调用方的内容区自带 padding。
 */
export const dialogContentBaseClass =
  'fixed top-[50%] left-[50%] z-50 grid max-h-[calc(100dvh-2rem)] translate-x-[-50%] translate-y-[-50%] gap-0 overflow-hidden rounded-ib-overlay border border-ib-border-soft bg-white p-0 shadow-xl data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95'
