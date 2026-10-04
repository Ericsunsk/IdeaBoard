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
    <DropdownMenuContent align="end" :side-offset="8" class="w-36">
      <DropdownMenuItem class="cursor-pointer gap-2" @select="setTheme('light')">
        <SunLight class="size-4 text-muted-foreground" />
        <span class="flex-1">明亮</span>
        <Check v-if="mode === 'light'" class="size-4 text-primary" />
      </DropdownMenuItem>
      <DropdownMenuItem class="cursor-pointer gap-2" @select="setTheme('dark')">
        <HalfMoon class="size-4 text-muted-foreground" />
        <span class="flex-1">暗黑</span>
        <Check v-if="mode === 'dark'" class="size-4 text-primary" />
      </DropdownMenuItem>
      <DropdownMenuItem class="cursor-pointer gap-2" @select="setTheme('auto')">
        <Laptop class="size-4 text-muted-foreground" />
        <span class="flex-1">跟随系统</span>
        <Check v-if="mode === 'auto'" class="size-4 text-primary" />
      </DropdownMenuItem>
    </DropdownMenuContent>
  </DropdownMenu>
</template>

<script setup lang="ts">
import { Check, HalfMoon, Laptop, SunLight } from '@iconoir/vue'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useTheme } from '@/shared/theme/composables/useTheme'

const { mode, isDark, setTheme } = useTheme()
</script>
