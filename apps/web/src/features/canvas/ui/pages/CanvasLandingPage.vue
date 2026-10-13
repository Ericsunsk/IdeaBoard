<template>
  <div
    class="canvas-landing-page relative flex min-h-dvh w-full flex-col overflow-x-hidden bg-background bg-fixed text-[var(--color-home-hero-text)] scroll-smooth [font-family:-apple-system,BlinkMacSystemFont,'SF_Pro_Display','Segoe_UI',Roboto,'Helvetica_Neue',sans-serif]"
  >
    <header
      class="absolute top-[var(--spacing-page-chrome-inset)] left-[var(--spacing-page-chrome-inset)] z-20 inline-flex min-w-0 max-w-[calc(100%-64px)] items-center"
    >
      <StudioName />
    </header>

    <div class="ib-board-page-background page-dot-bg"></div>

    <!-- 顶部 Hero 区域 -->
    <div
      class="home-hero-shell relative z-[1] flex min-h-dvh flex-1 items-center justify-center p-0"
    >
      <div
        class="home-hero-stage relative aspect-[1440/900] w-[clamp(960px,min(100vw,160vh),1440px)] shrink-0"
      >
        <!-- 核心文字内容 -->
        <div
          class="hero-content home-hero-content absolute left-[22.2%] top-[38.9%] z-10 flex w-[55.6%] flex-col items-center gap-6 text-center"
        >
          <h1
            data-home-hero-safe
            class="home-hero-title m-0 max-w-full break-words text-[90px] leading-none font-semibold tracking-normal text-[var(--color-home-hero-text)] font-display"
          >
            {{ projectName }}
          </h1>
          <p
            data-home-hero-safe
            class="home-hero-tagline mt-4 text-admin-title-sm font-normal uppercase tracking-[0.1em] text-[var(--color-home-hero-text)] font-hand"
          >
            AI CANVAS FOR VISUAL DESIGNERS
          </p>
          <div class="home-hero-cta-row mt-6 flex gap-4">
            <button
              data-home-hero-safe
              class="home-hero-cta h-[43px] cursor-pointer rounded-full border-0 bg-primary px-10 text-admin-title-sm font-medium text-primary-foreground shadow-none transition-[transform,background-color] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:scale-[1.04] hover:bg-primary/90 focus-visible:outline-none focus-visible:shadow-[0_0_0_3px_var(--ring)]"
              @click="startJourney"
            >
              Get started
            </button>
          </div>
        </div>

        <!-- 漂浮装饰元素 -->
        <div class="home-hero-decorations pointer-events-none absolute inset-0 z-[5] h-full w-full">
          <div
            data-home-hero-safe
            class="home-hero-note absolute left-[20%] top-[16.9%] w-[228px] rotate-[5deg] overflow-hidden rounded-[18px] border border-[var(--color-home-note-border)] bg-[var(--color-home-note-bg)] px-8 py-7 text-admin-meta leading-[1.5] text-[var(--color-home-note-text)] shadow-[0_18px_48px_rgba(78,70,58,0.08)] [font-family:-apple-system,BlinkMacSystemFont,'Segoe_UI',Roboto,sans-serif]"
          >
            {{ projectName }} is an AI Canvas for Visual Designers to explore, compare, and refine
            image directions in one connected workspace.
          </div>
          <img
            :src="homeHeroAssets.starburst"
            class="home-hero-starburst absolute left-[43%] top-[28%] w-[6.1%] rotate-[-4deg] object-contain drop-shadow-[0_12px_20px_rgba(224,195,42,0.18)]"
            alt=""
            aria-hidden="true"
          />
          <img
            :src="homeHeroAssets.socks"
            class="home-hero-socks absolute left-[8%] top-[38%] w-[29%] rotate-[-1.5deg] object-contain drop-shadow-[0_26px_28px_rgba(35,38,46,0.08)]"
            alt=""
            aria-hidden="true"
          />
          <img
            :src="homeHeroAssets.boots"
            class="home-hero-boots absolute left-[66.8%] top-[10.6%] w-[20.2%] rotate-[3deg] object-contain drop-shadow-[0_26px_28px_rgba(35,38,46,0.08)]"
            alt=""
            aria-hidden="true"
          />
          <div
            class="home-hero-ring absolute left-[75.2%] top-[50.6%] h-[2.8%] w-[8.8%] rotate-[8deg] rounded-full border-[4px] border-white/95 shadow-[0_0_18px_rgba(255,255,255,0.86)]"
            aria-hidden="true"
          ></div>
          <img
            :src="homeHeroAssets.portrait"
            class="home-hero-portrait absolute left-[65%] top-[48%] w-[22.4%] rotate-[0.5deg] object-contain drop-shadow-[0_28px_26px_rgba(26,28,34,0.1)]"
            alt=""
            aria-hidden="true"
          />
        </div>
      </div>

      <nav
        :aria-label="t('canvas.homeHero.legalNavAria')"
        class="absolute right-4 bottom-4 z-20 flex max-w-[calc(100%-3rem)] flex-wrap justify-end gap-x-3 gap-y-1 text-admin-meta font-medium text-[var(--color-home-hero-subtle)] sm:right-6 sm:bottom-5 sm:gap-x-5 sm:text-admin-body"
      >
        <RouterLink
          v-for="link in PUBLIC_LEGAL_LINKS"
          :key="link.to"
          :to="link.to"
          class="inline-flex min-h-8 items-center no-underline transition-colors hover:text-[var(--color-home-hero-text)] focus-visible:outline-none focus-visible:underline"
        >
          {{ link.shortLabel }}
        </RouterLink>
      </nav>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import bootsAsset from './assets/boots.svg'
import portraitAsset from './assets/portrait.svg'
import socksAsset from './assets/socks.svg'
import starburstAsset from './assets/starburst.svg'
import { useSiteBranding } from '../../../../core/composables/useSiteBranding'
import StudioName from '../../../../core/components/StudioName.vue'
import { USER_SEARCH_PATH } from '../../../workspace/routePaths'
import { PUBLIC_LEGAL_LINKS } from '../../../legal/legalLinks'

const router = useRouter()
const { t } = useI18n()
const { projectName } = useSiteBranding({ fallback: t('account.branding.loadFailed') })

const homeHeroAssets = {
  boots: bootsAsset,
  portrait: portraitAsset,
  socks: socksAsset,
  starburst: starburstAsset,
} as const

function startJourney() {
  router.push(USER_SEARCH_PATH)
}
</script>

<style scoped>
.hero-content {
  opacity: 0;
  animation: fadeUp var(--motion-enter-duration) var(--motion-enter-easing) forwards;
}

@media (max-width: 720px) {
  .home-hero-shell {
    align-items: flex-start;
    min-height: 100dvh;
    padding: clamp(48px, 8vh, 72px) 20px 36px;
  }

  .home-hero-stage {
    width: min(100%, 430px);
    height: min(100%, 720px);
    min-height: 650px;
    aspect-ratio: auto;
  }

  .home-hero-content {
    right: 0;
    left: 0;
    top: 38%;
    width: 100%;
    gap: 18px;
    padding: 0 6px;
  }

  .home-hero-title {
    font-size: clamp(56px, 17vw, 70px);
    line-height: 0.95;
    overflow-wrap: anywhere;
  }

  .home-hero-tagline {
    max-width: min(100%, 344px);
    margin-top: 0;
    font-size: clamp(12px, 3.3vw, 14px);
    line-height: 1.45;
    letter-spacing: 0.08em;
  }

  .home-hero-cta-row {
    margin-top: 10px;
  }

  .home-hero-cta {
    width: min(320px, 82vw);
    height: 44px;
    padding-inline: 30px;
  }

  .home-hero-note {
    top: 11%;
    left: 0;
    width: min(226px, 58vw);
    padding: 20px 22px;
    transform: rotate(-4deg);
  }

  .home-hero-starburst {
    top: 25%;
    left: 58%;
    width: max(54px, 15%);
  }

  .home-hero-socks {
    top: 55%;
    left: -26px;
    width: min(165px, 40%);
  }

  .home-hero-boots {
    top: 14%;
    right: -20px;
    left: auto;
    width: min(124px, 28%);
  }

  .home-hero-ring {
    top: 61%;
    right: 6%;
    left: auto;
    width: 54px;
    height: 18px;
    border-width: 3px;
  }

  .home-hero-portrait {
    top: 69%;
    right: -42px;
    left: auto;
    width: min(150px, 36%);
  }
}

@media (max-width: 420px) {
  .home-hero-shell {
    padding-inline: 16px;
  }

  .home-hero-stage {
    width: 100%;
    height: min(100%, 700px);
    min-height: 640px;
  }

  .home-hero-content {
    top: 40%;
  }

  .home-hero-title {
    font-size: clamp(50px, 16.6vw, 64px);
  }

  .home-hero-note {
    left: -8px;
    width: 198px;
  }

  .home-hero-boots {
    right: -44px;
    width: 112px;
  }

  .home-hero-socks {
    top: 57%;
    left: -40px;
    width: 154px;
  }

  .home-hero-portrait {
    right: -54px;
    width: 142px;
  }
}

@media (max-width: 360px) {
  .home-hero-title {
    font-size: 48px;
  }

  .home-hero-tagline {
    max-width: 300px;
    font-size: 11px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .hero-content {
    animation: none;
    opacity: 1;
    transform: none;
  }
}

@keyframes fadeUp {
  0% {
    opacity: 0;
    transform: translateY(30px);
  }
  100% {
    opacity: 1;
    transform: translateY(0);
  }
}
</style>
