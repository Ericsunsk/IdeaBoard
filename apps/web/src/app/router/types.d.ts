import 'vue-router'
import type { RouteSeoKey } from '../../shared/seo/routeSeo'

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
