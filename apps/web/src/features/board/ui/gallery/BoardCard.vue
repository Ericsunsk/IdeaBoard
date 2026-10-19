<template>
  <article
    ref="cardRootEl"
    class="board-card relative h-[244px] cursor-pointer overflow-hidden rounded-panel border border-border bg-card text-left hover:border-border/80 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    role="button"
    tabindex="0"
    :aria-label="board.name"
    @click="emit('click-board')"
    @keydown.enter.prevent="onCardKeyboardActivate"
    @keydown.space.prevent="onCardKeyboardActivate"
  >
    <div
      class="relative z-0 flex h-full w-full items-center justify-center overflow-hidden bg-[linear-gradient(135deg,var(--color-board-card-preview-from)_0%,var(--color-board-card-preview-to)_100%)]"
    >
      <SceneThumbnail
        v-if="sceneEntry?.scene && workspaceId"
        :scene="sceneEntry.scene"
        :workspace-id="workspaceId"
        :board-id="board.id"
        :content-version="sceneEntry.contentVersion"
      />
      <div
        v-else
        class="h-full w-full"
        :class="blankBoardPreviewSurfaceClass"
        aria-hidden="true"
      ></div>
      <div
        class="pointer-events-none absolute inset-x-0 bottom-0 h-[92px] bg-[linear-gradient(to_top,var(--color-board-card-preview-fade)_0%,var(--color-board-card-preview-fade-clear)_100%)]"
      ></div>
    </div>

    <div class="pointer-events-none absolute right-14 bottom-3 left-3 z-[2]">
      <input
        v-if="isRenaming"
        ref="renameInputEl"
        :value="renameDraft"
        type="text"
        :aria-label="t('common.actions.rename')"
        class="pointer-events-auto h-[30px] w-full rounded-control border border-input bg-background/90 px-2 text-admin-body text-foreground outline-none focus:border-ring focus:ring-1 focus:ring-ring"
        maxlength="60"
        @click.stop
        @input="onRenameInput"
        @blur="onRenameBlur"
        @keyup.enter.stop.prevent="emit('commit-rename')"
        @keyup.esc.stop.prevent="emit('cancel-rename')"
      />
      <button
        v-else
        type="button"
        class="pointer-events-auto block max-w-full cursor-pointer truncate border-0 bg-transparent p-0 text-left text-sm font-[650] text-foreground hover:underline focus-visible:outline-none focus-visible:underline"
        :aria-label="board.name"
        @click.stop="emit('click-board')"
      >
        {{ board.name }}
      </button>
      <div class="mt-[3px] text-xs text-muted-foreground">
        {{ t('board.edited', { date: displayDate }) }}
      </div>
      <div v-if="accessBadge" :class="catalogClasses.accessBadge">
        <Avatar v-if="ownerAvatarEntry" class="size-5">
          <AvatarImage v-if="ownerAvatarSrc" :src="ownerAvatarSrc" :alt="ownerAvatarEntry.userId" />
          <!-- Explicit delay: reka treats delayMs 0 (the wrapper default)
            as "never render", so the initial needs a real timer. -->
          <AvatarFallback :delay-ms="300" class="text-admin-micro font-semibold uppercase">
            {{ ownerAvatarInitial }}
          </AvatarFallback>
        </Avatar>
        <component
          :is="accessBadge.icon"
          v-else-if="accessBadge.icon"
          :class="catalogClasses.accessBadgeIcon"
          aria-hidden="true"
        />
        <span>{{ accessBadge.label }}</span>
      </div>
    </div>

    <div class="absolute right-3 bottom-3 z-[3]" @click.stop @pointerdown.stop>
      <DropdownMenu v-model:open="isMenuOverlayOpen" @update:open="onMenuOpenChange">
        <DropdownMenuTrigger as-child>
          <Button
            variant="ghost"
            size="icon"
            :title="t('board.moreActions')"
            :aria-label="t('board.moreActions')"
            class="size-[34px] border-0 bg-transparent text-muted-foreground hover:text-foreground hover:bg-muted/60"
            @click="emit('toggle-menu')"
          >
            <MoreHoriz class="size-[18px]" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent side="top" align="end" @close-auto-focus="onMenuCloseAutoFocus">
          <DropdownMenuItem
            v-for="item in menuItems"
            :key="item.label"
            :class="item.tone === 'danger' ? 'text-destructive focus:text-destructive' : ''"
            @select="item.command?.()"
          >
            {{ item.label }}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  </article>
</template>

<script setup lang="ts">
import { MoreHoriz, User } from '@iconoir/vue'
import { computed, nextTick, ref, watch, type Component } from 'vue'
import { Button } from '@/components/ui/button'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useI18n } from 'vue-i18n'
import { resolveMemberAvatar } from '../../../../core/media/avatarUrl'
import { useUserStore } from '../../../auth'
import type { CatalogEntry } from './contract'
import type { BoardListSceneEntry } from '../../session/sync/boardQueries'
import SceneThumbnail from './SceneThumbnail.vue'
import { blankBoardPreviewSurfaceClass, catalogClasses } from './skin'

const props = defineProps<{
  board: CatalogEntry
  sceneEntry: BoardListSceneEntry | null
  workspaceId: string | null
  isRenaming: boolean
  renameDraft: string
  isMenuOpen: boolean
  displayDate: string
}>()

const emit = defineEmits<{
  (e: 'click-board'): void
  (e: 'toggle-menu'): void
  (e: 'rename-action'): void
  (e: 'duplicate-action'): void
  (e: 'share-action'): void
  (e: 'delete-action'): void
  (e: 'rename-input', value: string): void
  (e: 'commit-rename'): void
  (e: 'cancel-rename'): void
  (e: 'menu-hidden'): void
}>()

const { t } = useI18n()
const userStore = useUserStore()

// Owner avatar row (bare size-5 avatar, no ring): team
// (workspace-posture) cards render the creator avatar; private cards and
// boards without server access facts render no avatar at all (the row is
// conditionally hidden, never an empty placeholder). Owner-is-self also
// renders (uniform row; the session avatar is the fallback when the
// payload lacks a URL). Facts are display-only, never permission inputs.
const ownerAvatarEntry = computed(() => {
  if (props.board.accessState !== 'workspace') return null
  const userId = props.board.creatorUserId?.trim()
  if (!userId) return null
  return { userId, avatar_url: props.board.ownerAvatarUrl ?? undefined }
})
const ownerAvatarSrc = computed(() =>
  ownerAvatarEntry.value ? resolveMemberAvatar(ownerAvatarEntry.value, userStore.user) : undefined,
)
const ownerAvatarInitial = computed(() => {
  const userId = ownerAvatarEntry.value?.userId ?? ''
  return userId.replace(/^usr_/, '').charAt(0).toUpperCase() || 'U'
})

// Server access posture as text (never color alone): the card renders the
// `access_state` fact verbatim and never derives it locally. Boards without
// the fact (never-synced local drafts) show no badge until the server mints
// their identity. Workspace cards pair the text with the owner avatar (no
// icon); private cards pair the text with the badge icon and no avatar.
const accessBadge = computed((): { icon?: Component; label: string } | null => {
  switch (props.board.accessState) {
    case 'workspace':
      return { label: t('board.accessWorkspace') }
    case 'private':
      return { icon: User, label: t('board.accessPrivate') }
    default:
      return null
  }
})

// Capability gating consumes the server flags verbatim: rename and delete
// (archive) need write, Share needs manage. Boards without capabilities
// (never-synced local drafts) keep their local rename/duplicate/delete so
// offline creation never loses its actions; Share stays hidden until the
// server confirms manage.
const canWriteBoard = computed(() =>
  props.board.capabilities === undefined ? true : props.board.capabilities.write === true,
)
const canShareBoard = computed(() => props.board.capabilities?.manage === true)

const cardRootEl = ref<HTMLElement | null>(null)
// Timestamp of the last menu-driven rename request. Reka fires
// closeAutoFocus after the select event, before the parent's prop update
// lands, so the child tracks the request locally to suppress the focus
// steal. One-shot: consumed by the next closeAutoFocus handler.
const renameRequestAt = ref(0)

function requestRename() {
  renameRequestAt.value = Date.now()
  emit('rename-action')
}

const menuItems = computed(() => [
  ...(canWriteBoard.value
    ? [
        {
          key: 'rename',
          label: t('common.actions.rename'),
          command: requestRename,
        },
      ]
    : []),
  {
    key: 'duplicate',
    label: t('board.duplicate'),
    command: () => emit('duplicate-action'),
  },
  ...(canShareBoard.value
    ? [
        {
          key: 'share',
          label: t('board.share'),
          command: () => emit('share-action'),
        },
      ]
    : []),
  ...(canWriteBoard.value
    ? [
        {
          key: 'delete',
          label: t('common.actions.delete'),
          tone: 'danger',
          command: () => emit('delete-action'),
        },
      ]
    : []),
])

const renameInputEl = ref<HTMLInputElement | null>(null)

watch(
  () => props.isRenaming,
  (isRenaming) => {
    if (!isRenaming) return
    void nextTick(() => {
      renameInputEl.value?.focus()
      renameInputEl.value?.select()
    })
  },
)

function onRenameInput(event: Event) {
  emit('rename-input', (event.target as HTMLInputElement).value)
}

// Blur commits only when focus truly leaves the card. The Reka menu close
// returns focus to its trigger (inside the card); that spurious blur must
// not enter the async PATCH chain. An explicit relatedTarget decides; only
// when it is null (runtime quirk) do we fall back to document.activeElement.
function onRenameBlur(event: FocusEvent) {
  const next = event.relatedTarget as Node | null
  if (next instanceof Node) {
    if (cardRootEl.value?.contains(next)) return
    emit('commit-rename')
    return
  }
  if (typeof document !== 'undefined') {
    const active = document.activeElement
    if (active instanceof Node && active !== document.body && cardRootEl.value?.contains(active)) {
      return
    }
  }
  emit('commit-rename')
}

// Suppress the Reka closeAutoFocus focus steal for the rename-triggering
// menu close so the rename input keeps focus. Normal menu closes (no rename
// pending, not renaming) keep the default focus-restore behavior.
function onMenuCloseAutoFocus(event: Event) {
  const isRecentRenameRequest = Date.now() - renameRequestAt.value < 500
  if (!props.isRenaming && !isRecentRenameRequest) return
  event.preventDefault()
  renameRequestAt.value = 0
}

function onCardKeyboardActivate(event: KeyboardEvent) {
  if (event.target !== event.currentTarget) return
  emit('click-board')
}

const isMenuOverlayOpen = ref(false)

watch(
  () => props.isMenuOpen,
  (isMenuOpen) => {
    if (!isMenuOpen && isMenuOverlayOpen.value) {
      isMenuOverlayOpen.value = false
    }
  },
)

function onMenuOpenChange(open: boolean) {
  if (!open) emit('menu-hidden')
}
</script>

<style scoped>
.board-card {
  transition:
    border-color var(--motion-control-duration) var(--motion-control-easing),
    transform var(--motion-control-duration) var(--motion-control-easing);
}

.board-card:hover,
.board-card:focus-visible {
  transform: translateY(-2px) scale(1.006);
}

.board-card-enter-active,
.board-card-appear-active {
  transition:
    opacity var(--motion-enter-duration) var(--motion-enter-easing),
    transform var(--motion-enter-duration) var(--motion-enter-easing);
}

.board-card-leave-active {
  transition:
    opacity var(--motion-exit-duration) var(--motion-exit-easing),
    transform var(--motion-exit-duration) var(--motion-exit-easing);
}

.board-card-enter-from,
.board-card-appear-from,
.board-card-leave-to {
  opacity: 0;
  transform: translateY(12px) scale(0.985);
}

.board-card-enter-to,
.board-card-appear-to,
.board-card-leave-from {
  opacity: 1;
  transform: translateY(0) scale(1);
}

@media (prefers-reduced-motion: reduce) {
  .board-card {
    transition: border-color var(--motion-control-duration) var(--motion-control-easing);
  }

  .board-card:hover,
  .board-card:focus-visible {
    transform: none;
  }

  .board-card-enter-active,
  .board-card-appear-active,
  .board-card-leave-active {
    transition: none;
  }

  .board-card-enter-from,
  .board-card-appear-from,
  .board-card-leave-to {
    opacity: 1;
    transform: none;
  }
}
</style>
