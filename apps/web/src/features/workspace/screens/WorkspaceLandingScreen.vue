<template>
  <div
    class="relative h-full w-full overflow-hidden [scroll-behavior:smooth] bg-[#ffffff] bg-[radial-gradient(circle,#e2e4e8_1.5px,transparent_1.5px)] bg-[length:24px_24px] [background-attachment:fixed] bg-center [font-family:-apple-system,BlinkMacSystemFont,'SF_Pro_Display','Segoe_UI',Roboto,'Helvetica_Neue',sans-serif] text-[#1d1d1f] [--theme-primary:#3b82f6] [--theme-secondary:#a855f7] [--theme-tertiary:#ec4899]"
  >
    <!-- 苹果风 Mesh Gradient 动态背景 -->
    <div
      class="pointer-events-none fixed inset-0 z-0 h-screen w-screen overflow-hidden bg-transparent"
    >
      <div
        class="absolute top-[10%] left-[20%] h-[70vw] w-[70vw] rounded-[9999px] bg-[#e0e7ff] opacity-40 mix-blend-normal blur-[140px] transition-[background-color] duration-1000 ease-in-out will-change-transform"
      ></div>
      <div
        class="absolute right-[10%] bottom-[10%] h-[80vw] w-[80vw] rounded-[9999px] bg-[#fae8ff] opacity-40 mix-blend-normal blur-[140px] transition-[background-color] duration-1000 ease-in-out will-change-transform"
      ></div>
      <div
        class="absolute top-[40%] left-[40%] h-[60vw] w-[60vw] rounded-[9999px] bg-[#f3e8ff] opacity-40 mix-blend-normal blur-[140px] transition-[background-color] duration-1000 ease-in-out will-change-transform"
      ></div>
      <div
        class="absolute top-[-10%] right-[-10%] h-[60vw] w-[60vw] rounded-[9999px] bg-[#ede9fe] opacity-40 mix-blend-normal blur-[140px] transition-[background-color] duration-1000 ease-in-out will-change-transform"
      ></div>
    </div>

    <!-- 顶部 Hero 区域 -->
    <div
      class="relative z-[1] flex h-full min-h-full items-center justify-center overflow-hidden p-0"
    >
      <div class="relative h-[900px] w-[1440px] shrink-0">
        <!-- 核心文字内容 -->
        <div
          class="home-fade-up absolute top-[350px] left-[320px] z-10 flex w-[800px] flex-col items-center text-center opacity-0"
        >
          <h1
            class="m-0 [font-family:'Satoshi',-apple-system,'SF_Pro_Display',sans-serif] text-[90px] leading-none font-semibold tracking-[-0.02em] text-[#202124] max-[768px]:text-[3.5rem] max-[480px]:text-[2.5rem]"
          >
            IdeaBoard
          </h1>
          <p
            class="mt-[18px] [font-family:'Comic_Sans_MS','Chalkboard_SE','Bradley_Hand','Caveat',cursive,sans-serif] text-[15px] font-normal tracking-[0.1em] text-[#202124] uppercase"
          >
            EXPLORE, EXPAND, AND REFINE YOUR IDEAS
          </p>
          <div
            class="mt-[56px] flex gap-4 max-[768px]:w-full max-[768px]:max-w-[300px] max-[768px]:flex-col"
          >
            <button
              class="h-[43px] cursor-pointer rounded-[9999px] border-none bg-[#9278e6] px-[40px] text-[15px] font-medium text-[#ffffff] shadow-none transition-[transform,background] duration-200 ease-[cubic-bezier(0.2,0.8,0.2,1)] hover:scale-[1.04] hover:bg-[#8065d6]"
              @click="startJourney"
            >
              Get started
            </button>
          </div>
        </div>

        <!-- 漂浮装饰元素 -->
        <div class="pointer-events-none absolute inset-0 z-[5] h-full w-full">
          <div
            class="absolute top-[172px] left-[380px] w-[220px] rotate-[1.5deg] overflow-hidden rounded-[16px] bg-[#e6ded5] p-[32px] [font-family:-apple-system,BlinkMacSystemFont,'Segoe_UI',Roboto,sans-serif] text-[12px] leading-[1.5] text-[#202124] shadow-[0_10px_30px_rgba(0,0,0,0.05)]"
          >
            IdeaBoard is an AI-powered concepting board that helps you explore, expand, and refine
            your ideas.
          </div>

          <div
            class="absolute top-[349px] left-[328px] z-[5] flex h-[100px] w-[100px] items-center justify-center overflow-visible bg-transparent shadow-none"
          >
            <div
              class="absolute top-[13px] left-[2px] h-[75px] w-[75px] rotate-[-7.5deg] rounded-[12px] bg-[linear-gradient(135deg,#a855f7,#3b82f6)] shadow-[0_4px_12px_rgba(0,0,0,0.1)]"
            ></div>
            <div
              class="absolute top-0 left-0 h-[75px] w-[75px] rotate-[15.6deg] rounded-[12px] border-[2px] border-[rgba(0,0,0,0.2)] bg-[rgba(255,255,255,0.5)] shadow-[0_4px_12px_rgba(0,0,0,0.1)] backdrop-blur-[4px]"
            ></div>
          </div>

          <div
            class="absolute top-[540px] left-[394px] h-[124px] w-[124px] overflow-hidden rounded-[9999px] border-[4px] border-[#ffffff] bg-[linear-gradient(135deg,#34d399_0%,#3a92e3_73%,#3b82f6_100%)] shadow-[0_24px_48px_rgba(0,0,0,0.06),0_4px_12px_rgba(0,0,0,0.04)]"
          ></div>
          <div
            class="absolute top-[166px] left-[978px] h-[143px] w-[92px] rotate-[-21.5deg] overflow-hidden rounded-[16px] border-[4px] border-[#ffffff] bg-[linear-gradient(180deg,#fbbf24,#f97316)] shadow-[0_24px_48px_rgba(0,0,0,0.06),0_4px_12px_rgba(0,0,0,0.04)]"
          ></div>
          <div
            class="absolute top-[548px] left-[910px] h-[150px] w-[150px] rotate-[8deg] overflow-hidden rounded-[20px] border-[4px] border-[#ffffff] bg-[conic-gradient(from_135deg,#d946ef,#3b82f6,#ec4899)] shadow-[0_24px_48px_rgba(0,0,0,0.06),0_4px_12px_rgba(0,0,0,0.04)]"
          ></div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'

const router = useRouter()

function startJourney() {
  router.push('/user/boards')
}
</script>

<style scoped>
.home-fade-up {
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
