<!--
  Remote presence cursors overlay (Y.js awareness-only).
  Pure view over `RemotePresencePeer[]`: each peer with a live cursor renders
  a colored pointer plus a name flag. The flag reuses the board owner-avatar
  flag language (size-5 Avatar + fallback initial + name text). Decorative and
  non-interactive: aria-hidden, pointer-events none, so flow gestures,
  minimap, menus, and dialogs keep their behavior.
-->
<template>
  <div class="presence-cursors" aria-hidden="true">
    <div
      v-for="peer in positionedPeers"
      :key="peer.clientId"
      class="presence-cursors__peer"
      :style="{ left: peer.left, top: peer.top }"
    >
      <svg
        class="presence-cursors__arrow drop-shadow-sm"
        :style="{ color: peer.color }"
        viewBox="0 0 16 20"
        focusable="false"
      >
        <path
          d="M5 2.5 L5 15.2 L8.7 11.8 L11.1 17.3 L13.7 16.1 L11.3 10.8 L15.6 10.8 Z"
          fill="currentColor"
          stroke-linejoin="round"
        />
      </svg>
      <span class="presence-cursors__flag">
        <Avatar class="size-5" :style="{ boxShadow: `0 0 0 2px ${peer.color}` }">
          <AvatarImage v-if="peer.avatarSrc" :src="peer.avatarSrc" :alt="peer.name" />
          <!-- Explicit delay: reka treats delayMs 0 (the wrapper default)
            as "never render", so the initial needs a real timer. -->
          <AvatarFallback :delay-ms="300" class="presence-cursors__initial">
            {{ peer.initial }}
          </AvatarFallback>
        </Avatar>
        <span class="presence-cursors__name">{{ peer.name }}</span>
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { resolveMemberAvatar } from '../../../../../core/media/avatarUrl'
import { useUserStore } from '../../../../auth'
import {
  resolvePeerAvatarUrl,
  resolvePeerColor,
  resolvePeerCursorPoint,
  flowPointToScreenPoint,
  type RemotePresencePeer,
} from '../../../session/collaboration'

const props = defineProps<{
  peers: RemotePresencePeer[]
  viewport: { x: number; y: number; zoom: number }
}>()

const userStore = useUserStore()

const positionedPeers = computed(() =>
  props.peers.flatMap((peer) => {
    const cursor = resolvePeerCursorPoint(peer.state)
    if (!cursor) return []
    const screen = flowPointToScreenPoint(cursor, props.viewport)
    if (!Number.isFinite(screen.x) || !Number.isFinite(screen.y)) return []
    const name = peer.state.user.name.trim() || peer.state.user.id
    const avatarUrl = resolvePeerAvatarUrl(peer.state)
    const avatarSrc = resolveMemberAvatar(
      { userId: peer.state.user.id, avatar_url: avatarUrl },
      userStore.user,
    )
    return [
      {
        clientId: peer.clientId,
        name,
        avatarSrc,
        initial: name.charAt(0).toUpperCase() || 'U',
        color: resolvePeerColor(peer.state),
        left: `${screen.x}px`,
        top: `${screen.y}px`,
      },
    ]
  }),
)
</script>

<style scoped>
.presence-cursors {
  position: absolute;
  inset: 0;
  overflow: hidden;
  pointer-events: none;
  /* Local canvas stacking only: above the Vue Flow pane/viewport/selection
    layers, below route chrome (header, menus, dialogs) which mounts outside
    the flow wrap or later in DOM order. */
  z-index: 6;
}

.presence-cursors__peer {
  position: absolute;
  transform: translate(-1px, -2px);
}

.presence-cursors__arrow {
  display: block;
  width: 14px;
  height: 18px;
}

.presence-cursors__arrow path {
  stroke: var(--color-card);
  stroke-width: 1.2;
}

.presence-cursors__initial {
  font-size: var(--canvas-font-size-micro);
  font-weight: 600;
  text-transform: uppercase;
}

.presence-cursors__flag {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  margin: 1px 0 0 12px;
  padding: 2px 8px 2px 2px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-pill);
  background-color: var(--color-card);
  box-shadow: var(--shadow-floating);
  white-space: nowrap;
}

.presence-cursors__name {
  font-size: 11px;
  font-weight: 600;
  line-height: 1.2;
  color: var(--color-foreground);
}
</style>
