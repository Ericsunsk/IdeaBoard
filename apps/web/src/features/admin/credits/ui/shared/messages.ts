// Colocated admin messages for the credits shared dialogs (zh-only, no `t()` wrapping).
//
// Copy Policy (`core/CONTEXT.md`): admin/ops surfaces keep their literals next
// to the component that owns them instead of locale bundles. Toast literals stay
// in `feedback.ts` next to the feedback factory; resource error literals stay in
// `api.ts` / the owning `resource.ts`; validation copy stays in the shared
// `actionReasonContract`.

export const CREDITS_SHARED_MESSAGES = {
  actionReasonDescription: '请填写本次操作的内部原因后确认提交。',
  actionReasonLabel: '操作原因',
} as const
