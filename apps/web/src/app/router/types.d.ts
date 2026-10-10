import 'vue-router'
import type { RouteSeoKey } from '../../core/seo/routeSeo'

declare module 'vue-router' {
  interface RouteMeta {
    requiresAuth?: boolean
    guest?: boolean
    title?: string
    seoKey?: RouteSeoKey
    hidePageHeader?: boolean
    fullWidth?: boolean
  }
}
