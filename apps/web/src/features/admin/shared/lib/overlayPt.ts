import type { DialogPassThroughOptions } from 'primevue/dialog'
import type { MenuPassThroughOptions } from 'primevue/menu'
import type { PopoverPassThroughOptions } from 'primevue/popover'
import { createDialogMotionPt } from '../../../../shared/motion/dialogMotion'
import { createPopoverMotionPt } from '../../../../shared/motion/popoverMotion'
import { baseDialogPt } from '../../../../shared/theme/baseDialogPt'

type OverlayClass = string | undefined

function joinOverlayClasses(...classes: OverlayClass[]) {
  return classes.filter(Boolean).join(' ')
}

const adminOverlayMenuListClass = '!m-0 !p-1.5'

export const adminOverlayMenuRootClass =
  'min-w-[180px] overflow-hidden rounded-[var(--radius-ib-overlay)] border border-ib-border bg-ib-surface shadow-[var(--shadow-ib-overlay)]'

const adminOverlayMenuItemContentClass =
  'rounded-[var(--radius-ib-control)] hover:!bg-ib-surface-hover'

const adminOverlayMenuItemLinkClass =
  '!px-3 !py-2.5 !text-admin-body !font-[650] !text-ib-text-muted hover:!text-ib-text'

const adminOverlayPopoverRootClass =
  '!rounded-[var(--radius-ib-overlay)] !border !border-admin-border-soft !bg-white !shadow-xl before:!hidden after:!hidden'

const adminOverlayPopoverContentClass =
  '!overflow-clip !rounded-[calc(var(--radius-ib-overlay)-1px)]'

export const adminOverlayDialogRootClass =
  '!overflow-hidden !rounded-[var(--radius-ib-overlay)] !border !border-ib-border !bg-ib-surface !shadow-[var(--shadow-ib-overlay)]'

export function createAdminMenuPt(input: {
  rootClass: string
  listClass?: string
  itemContentClass?: string
  itemLinkClass?: string
}): MenuPassThroughOptions {
  return {
    ...createPopoverMotionPt(),
    root: { class: input.rootClass },
    list: { class: input.listClass ?? adminOverlayMenuListClass },
    itemContent: { class: input.itemContentClass ?? adminOverlayMenuItemContentClass },
    itemLink: { class: input.itemLinkClass ?? adminOverlayMenuItemLinkClass },
  }
}

export function createAdminPopoverPt(
  input: {
    rootClass?: string
    contentClass?: string
  } = {},
): PopoverPassThroughOptions {
  return {
    ...createPopoverMotionPt(),
    root: { class: joinOverlayClasses(adminOverlayPopoverRootClass, input.rootClass) },
    content: {
      class: joinOverlayClasses(adminOverlayPopoverContentClass, input.contentClass),
    },
  }
}

export function createAdminDialogPt(input: {
  rootClass?: string
  headerClass?: string
  contentClass?: string
  footerClass?: string
}): DialogPassThroughOptions {
  return {
    ...baseDialogPt,
    ...createDialogMotionPt(),
    ...(input.rootClass ? { root: { class: input.rootClass } } : {}),
    ...(input.headerClass ? { header: { class: input.headerClass } } : {}),
    ...(input.contentClass ? { content: { class: input.contentClass } } : {}),
    ...(input.footerClass ? { footer: { class: input.footerClass } } : {}),
  }
}
