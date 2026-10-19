<!--
  Online presence roster (Y.js awareness-only).
  Header avatar cluster over `RemotePresencePeer[]`: each peer renders the
  shared Avatar with an in-field presence color dot. Activating a peer
  toggles Follow (500ms viewport tween); activating it again exits. The
  trailing control toggles Spotlight for the local presenter with a live
  follower count.
-->
<template>
  <section v-if="hasMembers" class="presence-roster" :aria-label="t('canvas.presence.rosterLabel')">
    <ul class="presence-roster__stack">
      <li v-if="localEntry" class="presence-roster__item">
        <span
          class="presence-roster__self"
          :aria-label="t('canvas.presence.selfLabel', { name: localEntry.name })"
          :title="t('canvas.presence.selfLabel', { name: localEntry.name })"
        >
          <Avatar class="size-8">
            <AvatarImage
              v-if="localEntry.avatarSrc"
              :src="localEntry.avatarSrc"
              :alt="localEntry.name"
            />
            <AvatarFallback :delay-ms="300" class="presence-roster__initial">
              {{ localEntry.initial }}
            </AvatarFallback>
          </Avatar>
          <span
            class="presence-roster__dot"
            aria-hidden="true"
            :style="{ backgroundColor: localEntry.color }"
          />
        </span>
      </li>
      <li v-for="item in rosterItems" :key="item.clientId" class="presence-roster__item">
        <button
          type="button"
          class="presence-roster__member"
          :class="{ 'is-following': item.isFollowing, 'is-spotlighting': item.isSpotlighting }"
          :aria-label="item.actionLabel"
          :aria-pressed="item.isFollowing"
          :title="item.actionLabel"
          @click="emit('follow-toggle', item.clientId)"
        >
          <Avatar class="size-8">
            <AvatarImage v-if="item.avatarSrc" :src="item.avatarSrc" :alt="item.name" />
            <AvatarFallback :delay-ms="300" class="presence-roster__initial">
              {{ item.initial }}
            </AvatarFallback>
          </Avatar>
          <span
            class="presence-roster__dot"
            aria-hidden="true"
            :style="{ backgroundColor: item.color }"
          />
          <span v-if="item.isSpotlighting" class="presence-roster__live" aria-hidden="true" />
        </button>
      </li>
    </ul>
    <div class="presence-roster__spotlight">
      <button
        type="button"
        class="presence-roster__spotlight-button"
        :aria-pressed="isSpotlighting"
        :aria-label="
          isSpotlighting ? t('canvas.presence.spotlightStop') : t('canvas.presence.spotlightStart')
        "
        :title="
          isSpotlighting ? t('canvas.presence.spotlightStop') : t('canvas.presence.spotlightStart')
        "
        @click="emit('spotlight-toggle')"
      >
        <Presentation class="size-4" />
      </button>
      <span v-if="isSpotlighting" class="presence-roster__followers" aria-live="polite">
        {{
          followerCount > 0
            ? t('canvas.presence.followerCount', { count: followerCount })
            : t('canvas.presence.followerCountNone')
        }}
      </span>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Presentation } from '@iconoir/vue'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { resolveMemberAvatar } from '../../../../../core/media/avatarUrl'
import { isFollowingDevice, isSpotlightPresenter } from './follow'
import {
  resolvePeerAvatarUrl,
  resolvePeerColor,
  type PresenceUser,
  type RemotePresencePeer,
} from '../../../session/collaboration'

const props = defineProps<{
  peers: RemotePresencePeer[]
  localUser: PresenceUser | null
  followingClientId: number | null
  isSpotlighting: boolean
  followerCount: number
}>()

const emit = defineEmits<{
  'follow-toggle': [clientId: number]
  'spotlight-toggle': []
}>()

const { t } = useI18n()

function toInitial(name: string): string {
  return name.charAt(0).toUpperCase() || 'U'
}

const localEntry = computed(() => {
  if (!props.localUser) return null
  const name = props.localUser.name.trim() || props.localUser.id
  const viewer = {
    public_id: props.localUser.id,
    avatar: props.localUser.avatarUrl ?? null,
  }
  return {
    name,
    initial: toInitial(name),
    color: props.localUser.color?.trim() || 'var(--color-border)',
    avatarSrc: resolveMemberAvatar(
      {
        userId: props.localUser.id,
        avatar_url: props.localUser.avatarUrl ?? undefined,
      },
      viewer,
    ),
  }
})

const rosterItems = computed(() =>
  props.peers.map((peer) => {
    const name = peer.state.user.name.trim() || peer.state.user.id
    const viewer = props.localUser
      ? {
          public_id: props.localUser.id,
          avatar: props.localUser.avatarUrl ?? null,
        }
      : null
    const isFollowing = isFollowingDevice(props.followingClientId, peer.clientId)
    return {
      clientId: peer.clientId,
      name,
      initial: toInitial(name),
      color: resolvePeerColor(peer.state),
      avatarSrc: resolveMemberAvatar(
        { userId: peer.state.user.id, avatar_url: resolvePeerAvatarUrl(peer.state) },
        viewer,
      ),
      isFollowing,
      isSpotlighting: isSpotlightPresenter(peer),
      actionLabel: isFollowing
        ? t('canvas.presence.unfollowMember', { name })
        : t('canvas.presence.followMember', { name }),
    }
  }),
)

const hasMembers = computed(() => localEntry.value !== null || rosterItems.value.length > 0)
</script>

<style scoped>
.presence-roster {
  pointer-events: auto;
  display: flex;
  align-items: center;
  gap: 8px;
  border-radius: var(--radius-pill);
  background-color: var(--color-card);
  border: 1px solid var(--color-border);
  box-shadow: var(--shadow-floating);
  padding: 4px 8px 4px 4px;
  max-width: min(320px, calc(100vw - 32px));
}

.presence-roster__stack {
  display: flex;
  align-items: center;
  margin: 0;
  padding: 0;
  list-style: none;
}

.presence-roster__item {
  margin-left: -8px;
}

.presence-roster__item:first-child {
  margin-left: 0;
}

.presence-roster__self,
.presence-roster__member {
  position: relative;
  display: inline-flex;
  border-radius: var(--radius-pill);
}

.presence-roster__member {
  border: 2px solid transparent;
  background: transparent;
  padding: 0;
  cursor: pointer;
  transition: border-color var(--motion-control-duration) var(--motion-control-easing);
}

.presence-roster__member:hover {
  border-color: var(--color-border);
}

.presence-roster__member:focus-visible {
  outline: none;
  border-color: var(--color-page-chrome-focus-ring);
  box-shadow: 0 0 0 3px var(--color-page-chrome-focus-ring);
}

.presence-roster__member.is-following {
  border-color: var(--color-primary);
}

.presence-roster__member.is-spotlighting {
  border-color: var(--color-destructive);
}

.presence-roster__dot {
  position: absolute;
  right: 0;
  bottom: 0;
  width: 10px;
  height: 10px;
  border: 2px solid var(--color-card);
  border-radius: var(--radius-pill);
}

.presence-roster__live {
  position: absolute;
  top: 0;
  right: 0;
  width: 10px;
  height: 10px;
  border: 2px solid var(--color-card);
  border-radius: var(--radius-pill);
  background-color: var(--color-destructive);
}

.presence-roster__initial {
  font-size: var(--canvas-font-size-micro);
  font-weight: 600;
  text-transform: uppercase;
}

.presence-roster__spotlight {
  display: flex;
  align-items: center;
  gap: 6px;
}

.presence-roster__spotlight-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-pill);
  background: transparent;
  color: var(--color-foreground);
  cursor: pointer;
}

.presence-roster__spotlight-button[aria-pressed='true'] {
  background: var(--color-primary);
  border-color: var(--color-primary);
  color: var(--color-primary-foreground);
}

.presence-roster__spotlight-button:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px var(--color-page-chrome-focus-ring);
}

.presence-roster__followers {
  font-size: 11px;
  font-weight: 600;
  line-height: 1.2;
  color: var(--color-foreground);
  white-space: nowrap;
}
</style>
