<template>
  <DropdownMenu>
    <DropdownMenuTrigger as-child>
      <button
        type="button"
        title="外观设置"
        aria-label="切换主题外观"
        class="flex size-9 shrink-0 cursor-pointer items-center justify-center rounded-lg border border-border bg-card text-foreground shadow-xs transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        <HalfMoon v-if="isDark" class="size-4" />
        <SunLight v-else class="size-4" />
      </button>
    </DropdownMenuTrigger>
    <DropdownMenuContent align="end" :side-offset="8" class="w-36 p-1">
      <div
        role="radiogroup"
        aria-label="主题外观"
        tabindex="-1"
        class="flex h-8 w-full items-center rounded-lg border border-border bg-muted/60 p-0.5"
        @keydown="handleThemeKeydown"
      >
        <button
          type="button"
          role="radio"
          :aria-checked="mode === 'light'"
          :tabindex="mode === 'light' ? 0 : -1"
          title="明亮"
          aria-label="明亮"
          class="flex flex-1 h-full items-center justify-center rounded-md text-xs transition-colors cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          :class="
            mode === 'light'
              ? 'bg-card font-semibold text-foreground shadow-xs'
              : 'text-muted-foreground hover:text-foreground'
          "
          @click="setTheme('light')"
        >
          <SunLight class="size-4" />
        </button>
        <button
          type="button"
          role="radio"
          :aria-checked="mode === 'dark'"
          :tabindex="mode === 'dark' ? 0 : -1"
          title="暗黑"
          aria-label="暗黑"
          class="flex flex-1 h-full items-center justify-center rounded-md text-xs transition-colors cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          :class="
            mode === 'dark'
              ? 'bg-card font-semibold text-foreground shadow-xs'
              : 'text-muted-foreground hover:text-foreground'
          "
          @click="setTheme('dark')"
        >
          <HalfMoon class="size-4" />
        </button>
        <button
          type="button"
          role="radio"
          :aria-checked="mode === 'auto'"
          :tabindex="mode === 'auto' ? 0 : -1"
          title="跟随系统"
          aria-label="跟随系统"
          class="flex flex-1 h-full items-center justify-center rounded-md text-xs transition-colors cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          :class="
            mode === 'auto'
              ? 'bg-card font-semibold text-foreground shadow-xs'
              : 'text-muted-foreground hover:text-foreground'
          "
          @click="setTheme('auto')"
        >
          <Laptop class="size-4" />
        </button>
      </div>
    </DropdownMenuContent>
  </DropdownMenu>
</template>

<script setup lang="ts">
import { HalfMoon, Laptop, SunLight } from '@iconoir/vue'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useTheme } from '@/shared/theme/composables/useTheme'
import { createThemeKeyboardHandler } from '@/shared/theme/composables/themeKeyboard'

const { mode, isDark, setTheme } = useTheme()

const handleThemeKeydown = createThemeKeyboardHandler({ mode, setTheme })
</script>
