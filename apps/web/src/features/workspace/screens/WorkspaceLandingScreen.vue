<template>
  <div
    class="relative h-full w-full overflow-hidden bg-white bg-[radial-gradient(circle,#e2e4e8_1px,transparent_1px)] bg-[length:12px_12px] bg-center bg-fixed text-[#1d1d1f] scroll-smooth [font-family:-apple-system,BlinkMacSystemFont,'SF_Pro_Display','Segoe_UI',Roboto,'Helvetica_Neue',sans-serif]"
  >
    <!-- 苹果风 Mesh Gradient 动态背景 -->
    <div
      class="pointer-events-none fixed inset-0 z-0 h-screen w-screen overflow-hidden bg-transparent"
    >
      <div
        class="absolute left-[20%] top-[10%] h-[70vw] w-[70vw] rounded-full bg-[#e0e7ff] opacity-40 blur-[140px] mix-blend-normal transition-colors duration-1000 will-change-transform"
      ></div>
      <div
        class="absolute bottom-[10%] right-[10%] h-[80vw] w-[80vw] rounded-full bg-[#fae8ff] opacity-40 blur-[140px] mix-blend-normal transition-colors duration-1000 will-change-transform"
      ></div>
      <div
        class="absolute left-[40%] top-[40%] h-[60vw] w-[60vw] rounded-full bg-[#f3e8ff] opacity-40 blur-[140px] mix-blend-normal transition-colors duration-1000 will-change-transform"
      ></div>
      <div
        class="absolute -right-[10%] -top-[10%] h-[60vw] w-[60vw] rounded-full bg-[#ede9fe] opacity-40 blur-[140px] mix-blend-normal transition-colors duration-1000 will-change-transform"
      ></div>
    </div>

    <!-- 顶部 Hero 区域 -->
    <div
      class="relative z-[1] flex h-full min-h-full items-center justify-center overflow-hidden p-0"
    >
      <div class="relative aspect-[1440/900] w-full max-w-[1440px] shrink-0">
        <!-- 核心文字内容 -->
        <div
          class="hero-content absolute left-[22.2%] top-[38.9%] z-10 flex w-[55.6%] flex-col items-center gap-6 text-center max-[768px]:left-[10%] max-[768px]:w-4/5"
        >
          <h1
            class="m-0 text-[90px] font-semibold leading-none tracking-[-0.02em] text-[#202124] [font-family:'Satoshi',-apple-system,'SF_Pro_Display',sans-serif] max-[1024px]:text-[64px] max-[480px]:text-[2.5rem] max-[768px]:text-[3.5rem]"
          >
            IdeaBoard
          </h1>
          <p
            class="mt-4 text-[15px] font-normal uppercase tracking-[0.1em] text-[#202124] [font-family:'Comic_Sans_MS','Chalkboard_SE','Bradley_Hand','Caveat',cursive,sans-serif] max-[480px]:text-[12px]"
          >
            EXPLORE, EXPAND, AND REFINE YOUR IDEAS
          </p>
          <div
            class="mt-6 flex gap-4 max-[768px]:w-full max-[768px]:max-w-[300px] max-[768px]:flex-col"
          >
            <button
              class="h-[43px] cursor-pointer rounded-full border-0 bg-[#9278e6] px-10 text-[15px] font-medium text-white shadow-none [transition:transform_0.2s_cubic-bezier(0.2,0.8,0.2,1),background_0.2s] hover:scale-[1.04] hover:bg-[#8065d6] max-[768px]:w-full"
              @click="startJourney"
            >
              Get started
            </button>
          </div>
        </div>

        <!-- 漂浮装饰元素 -->
        <div class="pointer-events-none absolute inset-0 z-[5] h-full w-full">
          <div
            class="absolute left-[26.4%] top-[19.1%] w-[15.3%] min-w-[160px] rotate-[1.5deg] overflow-hidden rounded-2xl bg-[#e6ded5] p-8 text-[12px] leading-[1.5] text-[#202124] shadow-[0_10px_30px_rgba(0,0,0,0.05)] [font-family:-apple-system,BlinkMacSystemFont,'Segoe_UI',Roboto,sans-serif] max-[1024px]:hidden"
          >
            IdeaBoard is an AI-powered concepting board that helps you explore, expand, and refine
            your ideas.
          </div>
          <div
            class="relative left-[22.8%] top-[38.8%] z-[5] flex aspect-square w-[6.9%] min-w-[60px] items-center justify-center overflow-visible bg-transparent max-[768px]:hidden"
          >
            <div
              class="absolute left-[2%] top-[13%] aspect-square w-3/4 rotate-[-7.5deg] rounded-[12px] bg-[linear-gradient(135deg,#a855f7,#3b82f6)] shadow-[0_4px_12px_rgba(0,0,0,0.1)]"
            ></div>
            <div
              class="absolute left-0 top-0 aspect-square w-3/4 rotate-[15.6deg] rounded-[12px] border-2 border-[rgba(0,0,0,0.2)] bg-[rgba(255,255,255,0.5)] shadow-[0_4px_12px_rgba(0,0,0,0.1)] backdrop-blur-[4px]"
            ></div>
          </div>
          <div
            class="absolute left-[27.4%] top-[60%] aspect-square w-[8.6%] min-w-16 overflow-hidden rounded-full border-4 border-white bg-[linear-gradient(135deg,#34d399_0%,#3a92e3_73%,#3b82f6_100%)] shadow-[0_24px_48px_rgba(0,0,0,0.06),0_4px_12px_rgba(0,0,0,0.04)] max-[480px]:hidden"
          ></div>
          <div
            class="absolute left-[67.9%] top-[18.4%] aspect-[92/143] w-[6.4%] min-w-12 rotate-[-21.5deg] overflow-hidden rounded-2xl border-4 border-white bg-[linear-gradient(180deg,#fbbf24,#f97316)] shadow-[0_24px_48px_rgba(0,0,0,0.06),0_4px_12px_rgba(0,0,0,0.04)] max-[768px]:hidden"
          ></div>
          <div
            class="absolute left-[63.2%] top-[60.9%] aspect-square w-[10.4%] min-w-16 rotate-[8deg] overflow-hidden rounded-[20px] border-4 border-white bg-[conic-gradient(from_135deg,#d946ef,#3b82f6,#ec4899)] shadow-[0_24px_48px_rgba(0,0,0,0.06),0_4px_12px_rgba(0,0,0,0.04)] max-[480px]:hidden"
          ></div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
const router = useRouter()

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
