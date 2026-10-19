<!--
  Online presence roster (Y.js awareness-only).
  Header avatar cluster over `RemotePresencePeer[]`: each peer renders the
  page-chrome avatar button structure (non-interactive span).
-->
<template>
  <section v-if="hasMembers" class="presence-roster" :aria-label="t('canvas.presence.rosterLabel')">
    <ul class="presence-roster__stack gap-2">
      <li v-if="localEntry" class="presence-roster__item">
        <span
          class="presence-roster__self"
          role="img"
          :aria-label="t('canvas.presence.selfLabel', { name: localEntry.name })"
          :title="t('canvas.presence.selfLabel', { name: localEntry.name })"
        >
          <span :class="[pageChromeAccountMenuClasses.avatarButton, 'presence-roster__avatar']">
            <img
              v-if="localEntry.avatarSrc"
              :src="localEntry.avatarSrc"
              :class="pageChromeAccountMenuClasses.avatarImage"
              alt=""
            />
            <span v-else :class="pageChromeAccountMenuClasses.avatarPlaceholder">
              <User :class="pageChromeAccountMenuClasses.avatarIcon" />
            </span>
          </span>
        </span>
      </li>
      <li v-for="item in rosterItems" :key="item.clientId" class="presence-roster__item">
        <span class="presence-roster__member" :aria-label="item.name" :title="item.name" role="img">
          <span :class="[pageChromeAccountMenuClasses.avatarButton, 'presence-roster__avatar']">
            <img
              v-if="item.avatarSrc"
              :src="item.avatarSrc"
              :class="pageChromeAccountMenuClasses.avatarImage"
              alt=""
            />
            <span v-else :class="pageChromeAccountMenuClasses.avatarPlaceholder">
              <User :class="pageChromeAccountMenuClasses.avatarIcon" />
            </span>
          </span>
        </span>
      </li>
    </ul>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { User } from '@iconoir/vue'
import { pageChromeAccountMenuClasses } from '../../../../../core/lib/pageChrome'
import { resolveMemberAvatar } from '../../../../../core/media/avatarUrl'
import { type PresenceUser, type RemotePresencePeer } from '../../../session/collaboration'

const props = defineProps<{
  peers: RemotePresencePeer[]
  localUser: PresenceUser | null
}>()

const { t } = useI18n()

const localEntry = computed(() => {
  if (!props.localUser) return null
  const name = props.localUser.name.trim() || props.localUser.id
  const viewer = {
    public_id: props.localUser.id,
    avatar: props.localUser.avatarUrl ?? null,
  }
  return {
    name,
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
    return {
      clientId: peer.clientId,
      name,
      avatarSrc: resolveMemberAvatar(
        { userId: peer.state.user.id, avatar_url: peer.state.user.avatarUrl },
        viewer,
      ),
    }
  }),
)

const hasMembers = computed(() => (props.localUser ? 1 : 0) + props.peers.length >= 2)
</script>

<style scoped>
.presence-roster {
  pointer-events: auto;
  display: flex;
  align-items: center;
  background-color: transparent;
  border: 0;
  box-shadow: none;
  padding: 0;
  max-width: min(320px, calc(100vw - 32px));
}

.presence-roster__stack {
  display: flex;
  align-items: center;
  margin: 0;
  padding: 0;
  list-style: none;
}

.presence-roster__self,
.presence-roster__member {
  position: relative;
  display: inline-flex;
}

.presence-roster__avatar {
  cursor: default;
}

/* Non-interactive: hover keeps the resting chrome face so image and placeholder match. */
.presence-roster__avatar:hover {
  --round-control-bg-hover: var(--round-control-bg);
  --round-control-border-hover: var(--round-control-border);
  --round-control-color-hover: var(--round-control-color);
}
</style>
