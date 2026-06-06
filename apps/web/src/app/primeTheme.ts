import { definePreset } from '@primeuix/themes'
import Aura from '@primeuix/themes/aura'
import { adminThemeTokens } from '../shared/theme/adminThemeTokens'

export const ideaBoardPrimePreset = definePreset(Aura, {
  semantic: {
    primary: {
      50: adminThemeTokens.surfaceMuted,
      100: adminThemeTokens.surfacePressed,
      200: adminThemeTokens.border,
      300: adminThemeTokens.borderStrong,
      400: adminThemeTokens.textSecondaryStrong,
      500: adminThemeTokens.accentPressed,
      600: '#111827',
      700: adminThemeTokens.textPrimary,
      800: adminThemeTokens.accent,
      900: '#344054',
      950: adminThemeTokens.accentPressed,
    },
    colorScheme: {
      light: {
        surface: {
          0: '#ffffff',
          50: adminThemeTokens.controlBg,
          100: adminThemeTokens.surfaceHover,
          200: adminThemeTokens.borderSoft,
          300: adminThemeTokens.border,
          400: adminThemeTokens.textDisabled,
          500: adminThemeTokens.textTertiary,
          600: adminThemeTokens.textSecondary,
          700: adminThemeTokens.textSecondaryStrong,
          800: '#344054',
          900: adminThemeTokens.textPrimary,
          950: '#111827',
        },
        formField: {
          borderColor: adminThemeTokens.controlBorder,
          hoverBorderColor: adminThemeTokens.textTertiary,
          focusBorderColor: '#000000',
        },
      },
    },
  },
})
