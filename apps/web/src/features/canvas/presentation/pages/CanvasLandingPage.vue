<template>
  <div
    class="canvas-landing-page relative flex min-h-dvh w-full flex-col overflow-x-hidden bg-ib-page text-ib-text"
  >
    <header
      class="relative z-10 flex min-w-0 items-center px-[var(--spacing-ib-page-chrome-inset)] pt-[var(--spacing-ib-page-chrome-inset)] sm:px-6 sm:pt-6"
    >
      <StudioName />
    </header>

    <div class="ib-board-page-background ib-page-dot-bg" aria-hidden="true"></div>

    <main
      class="relative z-[1] flex min-h-0 flex-1 items-center justify-center px-5 py-12 sm:px-8 sm:py-16"
    >
      <section
        aria-labelledby="canvas-landing-title"
        class="w-full max-w-2xl rounded-ib-panel border border-ib-border-soft bg-ib-surface px-6 py-10 text-center shadow-ib-control sm:px-12 sm:py-14"
      >
        <p
          class="m-0 text-ib-caption font-semibold uppercase tracking-[0.14em] text-ib-creative-accent"
        >
          {{ t('canvas.homeHero.kicker') }}
        </p>
        <h1
          id="canvas-landing-title"
          class="mt-3 mb-0 font-ib-display text-3xl leading-tight font-semibold tracking-tight text-ib-text sm:text-4xl"
        >
          {{ projectName }}
        </h1>
        <p class="mx-auto mt-4 mb-0 max-w-xl text-ib-content leading-6 text-ib-text-muted">
          {{ t('canvas.homeHero.description') }}
        </p>
        <Button
          type="button"
          class="mt-8 min-w-36 rounded-ib-control bg-board-primary px-6 text-ib-content font-semibold text-ib-text-inverse shadow-none hover:bg-board-primary-hover focus-visible:shadow-ib-focus"
          @click="startJourney"
        >
          {{ t('canvas.homeHero.start') }}
        </Button>
      </section>
    </main>

    <nav
      :aria-label="t('canvas.homeHero.legalNavAria')"
      class="relative z-10 flex max-w-full flex-wrap justify-center gap-x-4 gap-y-1 px-5 pb-5 text-ib-meta font-medium text-ib-text-muted sm:gap-x-5 sm:px-6 sm:pb-6 sm:text-ib-body"
    >
      <RouterLink
        v-for="link in PUBLIC_LEGAL_LINKS"
        :key="link.to"
        :to="link.to"
        class="rounded-ib-control no-underline transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:text-ib-text focus-visible:outline-none focus-visible:shadow-ib-focus"
      >
        {{ link.shortLabel }}
      </RouterLink>
    </nav>
  </div>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import { useSiteBranding } from '../../../../shared/composables/useSiteBranding'
import StudioName from '../../../../shared/components/StudioName.vue'
import { USER_SEARCH_PATH } from '../../../../shared/constants/routePaths'
import { PUBLIC_LEGAL_LINKS } from '../../../../shared/constants/legalLinks'

const router = useRouter()
const { t } = useI18n()
const { projectName } = useSiteBranding({ fallback: t('account.branding.loadFailed') })

function startJourney() {
  router.push(USER_SEARCH_PATH)
}
</script>
