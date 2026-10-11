import { watch, type Ref } from 'vue'

// 弹窗关闭后刷新摘要，保持板块状态与弹窗一致。
// OAuth/支付两卡同形，收拢于此。
export function useRefetchOnClose(visible: Ref<boolean>, refetch: () => unknown): void {
  watch(visible, (open) => {
    if (!open) void refetch()
  })
}
