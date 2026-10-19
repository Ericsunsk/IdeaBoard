<!--
  Spotlight invite banner (Y.js awareness-only).
  Shown to every collaborator while a peer broadcasts `isSpotlighting`.
  `role="alertdialog"` is intentional here: the banner is a room-wide
  strong reminder that needs an immediate accept/ignore decision. Focus
  lands on the accept control; Escape ignores the invite.
-->
<template>
  <div
    class="spotlight-banner"
    role="alertdialog"
    aria-modal="false"
    :aria-labelledby="titleId"
    :aria-describedby="bodyId"
    @keydown.esc.stop="emit('ignore')"
  >
    <span
      class="spotlight-banner__dot"
      aria-hidden="true"
      :style="{ backgroundColor: spotlighterColor }"
    />
    <div class="spotlight-banner__copy">
      <p :id="titleId" class="spotlight-banner__title">
        {{ t('canvas.presence.inviteTitle', { name: spotlighterName }) }}
      </p>
      <p :id="bodyId" class="spotlight-banner__body">
        {{ t('canvas.presence.inviteBody', { name: spotlighterName }) }}
      </p>
    </div>
    <div class="spotlight-banner__actions">
      <button
        ref="acceptRef"
        type="button"
        class="spotlight-banner__accept"
        @click="emit('accept')"
      >
        {{ t('canvas.presence.inviteAccept') }}
      </button>
      <button
        type="button"
        class="spotlight-banner__ignore"
        :aria-label="t('canvas.presence.inviteIgnore')"
        @click="emit('ignore')"
      >
        {{ t('canvas.presence.inviteIgnore') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, useId, useTemplateRef } from 'vue'
import { useI18n } from 'vue-i18n'

defineProps<{
  spotlighterName: string
  spotlighterColor: string
}>()

const emit = defineEmits<{
  accept: []
  ignore: []
}>()

const { t } = useI18n()
const acceptRef = useTemplateRef<HTMLButtonElement>('acceptRef')
const titleId = `spotlight-invite-title-${useId()}`
const bodyId = `spotlight-invite-body-${useId()}`

onMounted(() => {
  acceptRef.value?.focus({ preventScroll: true })
})
</script>

<style scoped>
.spotlight-banner {
  pointer-events: auto;
  display: flex;
  align-items: center;
  gap: 12px;
  max-width: min(420px, calc(100vw - 32px));
  border: 1px solid var(--color-border);
  border-radius: var(--radius-panel);
  background-color: var(--color-card);
  box-shadow: var(--shadow-floating);
  padding: 12px 12px 12px 14px;
}

.spotlight-banner__dot {
  width: 12px;
  height: 12px;
  flex: none;
  border-radius: var(--radius-pill);
}

.spotlight-banner__copy {
  min-width: 0;
  flex: 1;
}

.spotlight-banner__title {
  margin: 0;
  font-size: 13px;
  font-weight: 600;
  line-height: 1.3;
  color: var(--color-foreground);
}

.spotlight-banner__body {
  margin: 2px 0 0;
  font-size: 12px;
  line-height: 1.4;
  color: var(--color-muted-foreground);
}

.spotlight-banner__actions {
  display: flex;
  flex: none;
  align-items: center;
  gap: 8px;
}

.spotlight-banner__accept {
  min-height: 32px;
  border: 0;
  border-radius: var(--radius-control);
  background: var(--color-primary);
  padding: 0 14px;
  color: var(--color-primary-foreground);
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
}

.spotlight-banner__accept:focus-visible,
.spotlight-banner__ignore:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px var(--color-page-chrome-focus-ring);
}

.spotlight-banner__ignore {
  min-height: 32px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-control);
  background: transparent;
  padding: 0 14px;
  color: var(--color-foreground);
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
}
</style>
