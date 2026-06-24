<template>
  <div
    class="relative h-full w-full overflow-hidden bg-white bg-[radial-gradient(circle,#e2e4e8_1px,transparent_1px)] bg-[length:12px_12px] bg-center bg-fixed text-[#1d1d1f] scroll-smooth [font-family:-apple-system,BlinkMacSystemFont,'SF_Pro_Display','Segoe_UI',Roboto,'Helvetica_Neue',sans-serif]"
  >
    <!-- 轻量背景洗色，文字区排版保持稳定。 -->
    <div
      class="pointer-events-none fixed inset-0 z-0 h-screen w-screen overflow-hidden bg-[linear-gradient(115deg,rgba(255,255,255,0)_0%,rgba(245,241,255,0.72)_48%,rgba(255,255,255,0)_100%)]"
    >
      <div
        class="absolute inset-0 bg-[linear-gradient(180deg,rgba(255,255,255,0.86)_0%,rgba(255,255,255,0)_38%,rgba(255,255,255,0.7)_100%)]"
      ></div>
    </div>

    <!-- 顶部 Hero 区域 -->
    <div
      class="relative z-[1] flex h-full min-h-full items-center justify-center overflow-hidden p-0"
    >
      <div class="relative aspect-[1440/900] w-[clamp(960px,min(100vw,160vh),1440px)] shrink-0">
        <!-- 核心文字内容 -->
        <div
          class="hero-content absolute left-[22.2%] top-[38.9%] z-10 flex w-[55.6%] flex-col items-center gap-6 text-center"
        >
          <SiteBrandLogo
            :src="logoURL"
            class="h-16 w-16 rounded-2xl drop-shadow-[0_14px_22px_rgba(32,33,36,0.12)]"
          />
          <h1
            class="m-0 max-w-full break-words text-[90px] leading-none font-semibold tracking-normal text-[#202124] [font-family:'Satoshi',-apple-system,'SF_Pro_Display',sans-serif]"
          >
            {{ projectName }}
          </h1>
          <p
            class="mt-4 text-[15px] font-normal uppercase tracking-[0.1em] text-[#202124] [font-family:'Comic_Sans_MS','Chalkboard_SE','Bradley_Hand','Caveat',cursive,sans-serif]"
          >
            EXPLORE, EXPAND, AND REFINE YOUR IDEAS
          </p>
          <div class="mt-6 flex gap-4">
            <button
              class="h-[43px] cursor-pointer rounded-full border-0 bg-[#9278e6] px-10 text-[15px] font-medium text-white shadow-none [transition:transform_0.2s_cubic-bezier(0.2,0.8,0.2,1),background_0.2s] hover:scale-[1.04] hover:bg-[#8065d6]"
              @click="startJourney"
            >
              Get started
            </button>
          </div>
        </div>

        <!-- 漂浮装饰元素 -->
        <div class="pointer-events-none absolute inset-0 z-[5] h-full w-full">
          <div
            class="absolute left-[20%] top-[16.9%] w-[228px] rotate-[5deg] overflow-hidden rounded-[18px] bg-[#e6ded5] px-8 py-7 text-[12px] leading-[1.5] text-[#202124] shadow-[0_18px_48px_rgba(78,70,58,0.08)] [font-family:-apple-system,BlinkMacSystemFont,'Segoe_UI',Roboto,sans-serif]"
          >
            {{ projectName }} is an AI-powered concepting board that helps you explore, expand, and
            refine your ideas.
          </div>
          <img
            :src="homeHeroAssets.starburst"
            class="absolute left-[43%] top-[28%] w-[6.1%] rotate-[-4deg] object-contain drop-shadow-[0_12px_20px_rgba(224,195,42,0.18)]"
            alt=""
            aria-hidden="true"
          />
          <img
            :src="homeHeroAssets.socks"
            class="absolute left-[8%] top-[38%] w-[29%] rotate-[-1.5deg] object-contain drop-shadow-[0_26px_28px_rgba(35,38,46,0.08)]"
            alt=""
            aria-hidden="true"
          />
          <img
            :src="homeHeroAssets.boots"
            class="absolute left-[66.8%] top-[10.6%] w-[20.2%] rotate-[3deg] object-contain drop-shadow-[0_26px_28px_rgba(35,38,46,0.08)]"
            alt=""
            aria-hidden="true"
          />
          <div
            class="absolute left-[75.2%] top-[50.6%] h-[2.8%] w-[8.8%] rotate-[8deg] rounded-full border-[4px] border-white/95 shadow-[0_0_18px_rgba(255,255,255,0.86)]"
            aria-hidden="true"
          ></div>
          <img
            :src="homeHeroAssets.portrait"
            class="absolute left-[65%] top-[48%] w-[22.4%] rotate-[0.5deg] object-contain drop-shadow-[0_28px_26px_rgba(26,28,34,0.1)]"
            alt=""
            aria-hidden="true"
          />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import bootsAsset from '../assets/home/boots.svg'
import portraitAsset from '../assets/home/portrait.svg'
import socksAsset from '../assets/home/socks.svg'
import starburstAsset from '../assets/home/starburst.svg'
import SiteBrandLogo from '../../../shared/components/SiteBrandLogo.vue'
import { useSiteBranding } from '../../../shared/composables/useSiteBranding'

const router = useRouter()
const { logoURL, projectName } = useSiteBranding()

const homeHeroAssets = {
  boots: bootsAsset,
  portrait: portraitAsset,
  socks: socksAsset,
  starburst: starburstAsset,
} as const

function startJourney() {
  router.push('/user/boards')
}
</script>

<style scoped>
.hero-content {
  opacity: 0;
  animation: fadeUp 1s cubic-bezier(0.16, 1, 0.3, 1) forwards;
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
