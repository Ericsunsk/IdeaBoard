<!--
  Online presence roster (Y.js awareness-only).
  Header avatar cluster over `RemotePresencePeer[]`: each peer renders the
  page-chrome avatar button structure (non-interactive span).
-->
<template>
  <Transition name="presence-roster" appear>
    <section
      v-if="hasMembers"
      class="presence-roster"
      :aria-label="t('canvas.presence.rosterLabel')"
    >
      <TransitionGroup
        name="presence-avatar"
        tag="ul"
        class="presence-roster__stack gap-2"
        @before-leave="pinLeavingAvatar"
        @after-leave="releaseLeavingAvatar"
        @leave-cancelled="releaseLeavingAvatar"
      >
        <li v-for="item in rosterItems" :key="item.clientId" class="presence-roster__item">
          <span
            class="presence-roster__member"
            :aria-label="item.label"
            :title="item.label"
            role="img"
          >
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
            <Transition name="presence-activity">
              <span v-if="item.isEditing" class="presence-roster__activity" aria-hidden="true" />
            </Transition>
          </span>
        </li>
      </TransitionGroup>
    </section>
  </Transition>
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

const rosterItems = computed(() =>
  props.peers
    .filter((peer) => peer.state.user.id !== props.localUser?.id)
    .map((peer) => {
      const name = peer.state.user.name.trim() || peer.state.user.id
      return {
        clientId: peer.clientId,
        isEditing: peer.isEditing === true,
        label: peer.isEditing ? t('canvas.presence.editingLabel', { name }) : name,
        avatarSrc: resolveMemberAvatar(
          { userId: peer.state.user.id, avatar_url: peer.state.user.avatarUrl },
          null,
        ),
      }
    }),
)

const hasMembers = computed(() => rosterItems.value.length > 0)

function pinLeavingAvatar(element: Element) {
  if (!(element instanceof HTMLElement)) return
  element.style.left = `${element.offsetLeft}px`
  element.style.top = `${element.offsetTop}px`
}

function releaseLeavingAvatar(element: Element) {
  if (!(element instanceof HTMLElement)) return
  element.style.removeProperty('left')
  element.style.removeProperty('top')
}
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
  transform-origin: right center;
  max-width: min(320px, calc(100vw - 32px));
}

.presence-roster__stack {
  position: relative;
  display: flex;
  align-items: center;
  margin: 0;
  padding: 0;
  list-style: none;
}

.presence-roster__member {
  position: relative;
  display: inline-flex;
}

.presence-roster__avatar {
  cursor: default;
}

.presence-roster__activity {
  position: absolute;
  right: 0;
  bottom: 0;
  width: calc(var(--spacing) * 2);
  height: calc(var(--spacing) * 2);
  border: 2px solid var(--color-page-chrome-bg);
  border-radius: var(--radius-pill);
  background: var(--success);
  pointer-events: none;
}

.presence-activity-enter-active,
.presence-activity-leave-active {
  transition:
    opacity var(--motion-feedback-duration) var(--motion-feedback-easing),
    transform var(--motion-feedback-duration) var(--motion-feedback-easing);
}

.presence-activity-enter-from {
  opacity: 0;
  transform: scale(0.8);
}

.presence-activity-leave-to {
  opacity: 0;
}

.presence-roster-enter-active,
.presence-avatar-enter-active {
  transition:
    opacity var(--motion-enter-duration) var(--motion-enter-easing),
    transform var(--motion-enter-duration) var(--motion-enter-easing);
}

.presence-roster-leave-active,
.presence-avatar-leave-active {
  transition:
    opacity var(--motion-exit-duration) var(--motion-exit-easing),
    transform var(--motion-exit-duration) var(--motion-exit-easing);
}

.presence-roster-enter-from,
.presence-roster-leave-to,
.presence-avatar-enter-from,
.presence-avatar-leave-to {
  opacity: 0;
  transform: scale(0.9);
}

.presence-avatar-move {
  transition: transform var(--motion-control-duration) var(--motion-control-easing);
}

.presence-avatar-leave-active {
  position: absolute;
}

@media (prefers-reduced-motion: reduce) {
  .presence-activity-enter-active,
  .presence-activity-leave-active,
  .presence-roster-enter-active,
  .presence-roster-leave-active,
  .presence-avatar-enter-active,
  .presence-avatar-leave-active,
  .presence-avatar-move {
    transition: none;
  }

  .presence-activity-enter-from,
  .presence-roster-enter-from,
  .presence-roster-leave-to,
  .presence-avatar-enter-from,
  .presence-avatar-leave-to {
    transform: none;
  }
}

/* Non-interactive: hover keeps the resting chrome face so image and placeholder match. */
.presence-roster__avatar:hover {
  --round-control-bg-hover: var(--round-control-bg);
  --round-control-border-hover: var(--round-control-border);
  --round-control-color-hover: var(--round-control-color);
}
</style>
