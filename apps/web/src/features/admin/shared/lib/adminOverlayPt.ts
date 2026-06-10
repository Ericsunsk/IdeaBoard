import type { DialogPassThroughOptions } from 'primevue/dialog'
import type { MenuPassThroughOptions } from 'primevue/menu'
import type { PopoverPassThroughOptions } from 'primevue/popover'

type OverlayClass = string | undefined

function joinOverlayClasses(...classes: OverlayClass[]) {
  return classes.filter(Boolean).join(' ')
}

export const adminOverlayMenuListClass = '!m-0 !p-1.5'

export const adminOverlayMenuItemContentClass = 'rounded-[10px] hover:!bg-admin-surface-hover'

export const adminOverlayMenuItemLinkClass =
  '!px-3 !py-2.5 !text-admin-body !font-[650] !text-admin-text-2 hover:!text-admin-text-1'

export const adminOverlayPopoverRootClass =
  '!rounded-lg !border-admin-border !bg-admin-card !shadow-none'

export function createAdminMenuPt(input: {
  rootClass: string
  listClass?: string
  itemContentClass?: string
  itemLinkClass?: string
}): MenuPassThroughOptions {
  return {
    root: { class: input.rootClass },
    list: { class: input.listClass ?? adminOverlayMenuListClass },
    itemContent: { class: input.itemContentClass ?? adminOverlayMenuItemContentClass },
    itemLink: { class: input.itemLinkClass ?? adminOverlayMenuItemLinkClass },
  }
}

export function createAdminPopoverPt(input: {
  rootClass?: string
  contentClass?: string
} = {}): PopoverPassThroughOptions {
  return {
    root: { class: joinOverlayClasses(adminOverlayPopoverRootClass, input.rootClass) },
    ...(input.contentClass ? { content: { class: input.contentClass } } : {}),
  }
}

export const adminPrimeDialogPt = {
  header: { class: '!items-start !gap-3 !px-6 !pt-5 !pb-3.5' },
  headerActions: { class: '!mt-1 !shrink-0' },
  pcCloseButton: {
    root: {
      class:
        '!h-8 !w-8 !rounded-lg !border-0 !bg-transparent !p-0 !text-admin-text-3 !shadow-none hover:!bg-admin-surface-muted hover:!text-admin-text-1 focus:!shadow-none focus-visible:!outline-none focus-visible:!ring-0',
    },
    icon: { class: '!h-4 !w-4' },
  },
} satisfies DialogPassThroughOptions

export function createAdminDialogPt(input: {
  rootClass?: string
  headerClass?: string
  contentClass?: string
}): DialogPassThroughOptions {
  return {
    ...(input.rootClass ? { root: { class: input.rootClass } } : {}),
    ...(input.headerClass ? { header: { class: input.headerClass } } : {}),
    ...(input.contentClass ? { content: { class: input.contentClass } } : {}),
  }
}
