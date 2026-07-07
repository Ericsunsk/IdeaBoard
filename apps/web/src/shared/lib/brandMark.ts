function joinClassNames(...classes: Array<string | false | null | undefined>) {
  return classes.filter(Boolean).join(' ')
}

export const brandMarkClasses = {
  root: 'ib-brand-mark',
  logo: 'ib-brand-mark__logo',
  sizes: {
    compact: 'ib-brand-mark--compact',
    account: 'ib-brand-mark--account',
    login: 'ib-brand-mark--login',
    fill: 'ib-brand-mark--fill',
  },
  states: {
    image: 'ib-brand-mark--image',
    fallbackStrong: 'ib-brand-mark--fallback-strong',
    fallbackSoft: 'ib-brand-mark--fallback-soft',
  },
} as const

export function createBrandMarkClass(...extraClasses: Array<string | false | null | undefined>) {
  return joinClassNames(brandMarkClasses.root, ...extraClasses)
}
