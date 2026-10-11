import {
  CreditCard,
  Database,
  Headset,
  Home,
  Key,
  Mail,
  MediaImage,
  Server,
  ShieldCheck,
} from '@iconoir/vue'
import { SETTINGS_MESSAGES as M } from './messages'
import { STORAGE_MESSAGES } from '../cards/storage/messages'

export const SETTINGS_SECTIONS = {
  branding: { title: M.brandingTitle, icon: Home },
  support: { title: M.supportTitle, icon: Headset },
  storage: { title: STORAGE_MESSAGES.storageTitle, icon: Database },
  image: { title: M.imageLibraryTitle, icon: MediaImage },
  mail: { title: M.mailTitle, icon: Mail },
  oauth: { title: M.oauthLoginTitle, icon: Key },
  payment: { title: M.paymentTitle, icon: CreditCard },
  ratelimit: { title: M.registrationTitle, icon: ShieldCheck },
  outbound: { title: M.outboundTitle, icon: Server },
} as const
