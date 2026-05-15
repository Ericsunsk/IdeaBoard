<template>
  <div class="home-blank">
    <!-- 苹果风 Mesh Gradient 动态背景 -->
    <div class="mesh-bg">
      <div class="mesh-blob blob-1"></div>
      <div class="mesh-blob blob-2"></div>
      <div class="mesh-blob blob-3"></div>
      <div class="mesh-blob blob-4"></div>
      <!-- 噪点纹理，增加高级感 -->
      <div class="mesh-noise"></div>
    </div>

    <!-- 顶部 Hero 区域 -->
    <div class="hero-section">
      <div class="hero-stage">
        <!-- 核心文字内容 -->
        <div class="hero-content">
          <h1 class="hero-title">IdeaBoard</h1>
          <p class="mixboard-subtitle">EXPLORE, EXPAND, AND REFINE YOUR IDEAS</p>
          <div class="hero-actions">
            <button class="btn-primary mixboard-btn" @click="startJourney">Get started</button>
          </div>
        </div>

        <!-- 漂浮装饰元素 -->
        <div class="floating-gallery">
          <div class="float-item text-note">
            IdeaBoard is an AI-powered concepting board that helps you explore, expand, and refine
            your ideas.
          </div>
          <div class="float-item pos-1 layers-icon">
            <div class="layer bottom-layer"></div>
            <div class="layer top-layer"></div>
          </div>
          <div class="float-item pos-2 circle-gradient"></div>
          <div class="float-item pos-3 orange-rect"></div>
          <div class="float-item pos-4 purple-sq"></div>
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
/* 苹果风设计规范 (Apple-like Design Language) */
.home-blank {
  width: 100%;
  height: 100%;
  overflow: hidden;
  position: relative;
  background-color: #ffffff;
  /* Google Mixboard style dotted background */
  background-image: radial-gradient(circle, #e2e4e8 1.5px, transparent 1.5px);
  background-size: 24px 24px; /* 密度再次提升 1/2: 32px -> 24px */
  background-position: center center; /* 锚定中心点 */
  background-attachment: fixed; /* 固定背景不随滚动/缩放漂移 */
  color: #1d1d1f;
  font-family:
    -apple-system, BlinkMacSystemFont, 'SF Pro Display', 'Segoe UI', Roboto, 'Helvetica Neue',
    sans-serif;
  scroll-behavior: smooth;
  --theme-primary: #3b82f6;
  --theme-secondary: #a855f7;
  --theme-tertiary: #ec4899;
}

/* =========================================
   Mesh Gradient Background (类似 macOS 壁纸)
   ========================================= */
.mesh-bg {
  position: fixed;
  inset: 0;
  width: 100vw;
  height: 100vh;
  z-index: 0;
  overflow: hidden;
  background: transparent;
  pointer-events: none;
}

.mesh-blob {
  position: absolute;
  border-radius: 50%;
  filter: blur(140px);
  opacity: 0.4; /* 调低不透明度以展现柔和光晕 */
  mix-blend-mode: normal; /* Mixboard 背景是粉彩色的纯正融合，不要正片叠底 */
  transition: background-color 1s ease;
  will-change: transform;
}

/* 使用动态主题色 (Mixboard colors) */
.blob-1 {
  top: 10%;
  left: 20%;
  width: 70vw;
  height: 70vw;
  background: #e0e7ff; /* light blue */
  animation-delay: 0s;
}

.blob-2 {
  bottom: 10%;
  right: 10%;
  width: 80vw;
  height: 80vw;
  background: #fae8ff; /* pinkish purple */
  animation-direction: alternate-reverse;
  animation-delay: -5s;
}

.blob-3 {
  top: 40%;
  left: 40%;
  width: 60vw;
  height: 60vw;
  background: #f3e8ff; /* light purple */
  animation-delay: -10s;
}

.blob-4 {
  top: -10%;
  right: -10%;
  width: 60vw;
  height: 60vw;
  background: #ede9fe; /* very light purple */
  animation-direction: alternate-reverse;
  animation-delay: -11s;
}

@keyframes float-apple {
  0% {
    transform: translate(0, 0) scale(1) rotate(0deg);
  }
  33% {
    transform: translate(8%, 12%) scale(1.15) rotate(15deg);
  }
  66% {
    transform: translate(-8%, 8%) scale(0.85) rotate(-15deg);
  }
  100% {
    transform: translate(12%, -8%) scale(1.2) rotate(25deg);
  }
}

/* 噪点覆盖层（关闭以获得类似Mixboard的纯净渐变背景） */
.mesh-noise {
  display: none;
}

/* =========================================
   Glassmorphism Base Class (毛玻璃效果)
   ========================================= */
.glass-panel {
  background: rgba(255, 255, 255, 0.6);
  backdrop-filter: blur(30px);
  -webkit-backdrop-filter: blur(30px);
  border: 1px solid rgba(0, 0, 0, 0.05);
  box-shadow: 0 8px 32px 0 rgba(0, 0, 0, 0.05);
}

.container {
  max-width: 1200px;
  margin: 0 auto;
  padding: 0 24px;
}

/* ===== Hero Section ===== */
.hero-section {
  height: 100%;
  min-height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  position: relative;
  padding: 0;
  z-index: 1;
  overflow: hidden;
}

/* 核心舞台容器：响应式设计基准 */
.hero-stage {
  width: 100%;
  max-width: 1440px;
  aspect-ratio: 1440 / 900;
  position: relative;
  flex-shrink: 0;
}

.hero-content {
  position: absolute;
  left: 22.2%;
  top: 38.9%;
  width: 55.6%;
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  gap: 24px;
  opacity: 0;
  animation: fadeUp 1s cubic-bezier(0.16, 1, 0.3, 1) forwards;
  z-index: 10;
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

.hero-title {
  font-family:
    'Satoshi',
    -apple-system,
    'SF Pro Display',
    sans-serif;
  font-size: 90px;
  line-height: 1;
  font-weight: 600;
  margin: 0;
  letter-spacing: -0.02em;
  color: #202124;
}

.mixboard-subtitle {
  font-family: 'Comic Sans MS', 'Chalkboard SE', 'Bradley Hand', 'Caveat', cursive, sans-serif;
  text-transform: uppercase;
  font-size: 15px;
  letter-spacing: 0.1em;
  color: #202124;
  margin-top: 16px;
  font-weight: normal;
}

.hero-actions {
  display: flex;
  gap: 16px;
  margin-top: 24px;
}

.mixboard-btn {
  background: #9278e6 !important;
  color: #ffffff !important;
  border-radius: 999px;
  padding: 0 40px !important;
  font-size: 15px !important;
  font-weight: 500 !important;
  height: 43px !important;
  box-shadow: none !important;
  border: none !important;
  cursor: pointer;
  transition:
    transform 0.2s cubic-bezier(0.2, 0.8, 0.2, 1),
    background 0.2s !important;
}

.mixboard-btn:hover {
  background: #8065d6 !important;
  transform: scale(1.04) !important;
}

/* Mixboard Floating Gallery */
.floating-gallery {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
  z-index: 5;
}

.float-item {
  position: absolute;
  background: #ffffff;
  border-radius: 20px;
  box-shadow:
    0 24px 48px rgba(0, 0, 0, 0.06),
    0 4px 12px rgba(0, 0, 0, 0.04);
  overflow: hidden;
}

.text-note {
  left: 26.4%;
  top: 19.1%;
  width: 15.3%;
  min-width: 160px;
  padding: 32px;
  background: #e6ded5; /* Match exactly the warm beige */
  color: #202124;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  font-size: 12px;
  line-height: 1.5;
  border-radius: 16px;
  transform: rotate(1.5deg);
  box-shadow: 0 10px 30px rgba(0, 0, 0, 0.05);
}

.layers-icon {
  left: 22.8%;
  top: 38.8%;
  width: 6.9%;
  aspect-ratio: 1;
  min-width: 60px;
  background: transparent !important; /* 强制透明，防止继承 float-item 的白色 */
  border-radius: 0 !important;
  display: flex;
  align-items: center;
  justify-content: center;
  position: relative;
  z-index: 5;
  box-shadow: none !important; /* 移除外层投影 */
  overflow: visible !important; /* 关键：防止旋转后的图层被截断 */
}

.layers-icon .layer {
  position: absolute;
  width: 75%;
  aspect-ratio: 1;
  border-radius: 12px;
}

.layers-icon .bottom-layer {
  background: linear-gradient(135deg, #a855f7, #3b82f6);
  top: 13%;
  left: 2%;
  transform: rotate(-7.5deg);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
}

.layers-icon .top-layer {
  border: 2px solid rgba(0, 0, 0, 0.2);
  top: 0;
  left: 0;
  background: rgba(255, 255, 255, 0.5);
  backdrop-filter: blur(4px);
  transform: rotate(15.6deg);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
}

.circle-gradient {
  left: 27.4%;
  top: 60%;
  width: 8.6%;
  aspect-ratio: 1;
  min-width: 64px;
  border-radius: 50%;
  background: linear-gradient(135deg, #34d399 0%, #3a92e3 73%, #3b82f6 100%);
  border: 4px solid #ffffff;
}

.orange-rect {
  left: 67.9%;
  top: 18.4%;
  width: 6.4%;
  aspect-ratio: 92 / 143;
  min-width: 48px;
  border-radius: 16px;
  background: linear-gradient(180deg, #fbbf24, #f97316);
  border: 4px solid #ffffff;
  transform: rotate(-21.5deg);
}

.purple-sq {
  left: 63.2%;
  top: 60.9%;
  width: 10.4%;
  aspect-ratio: 1;
  min-width: 64px;
  border-radius: 20px;
  background: conic-gradient(from 135deg, #d946ef, #3b82f6, #ec4899);
  border: 4px solid #ffffff;
  transform: rotate(8deg);
}

/* 响应式设计 */
@media (max-width: 1024px) {
  .hero-title {
    font-size: 64px;
  }

  .text-note {
    display: none;
  }
}

@media (max-width: 768px) {
  .hero-title {
    font-size: 3.5rem;
  }

  .hero-content {
    left: 10%;
    width: 80%;
  }

  .hero-actions {
    flex-direction: column;
    width: 100%;
    max-width: 300px;
  }

  .mixboard-btn {
    width: 100%;
  }

  .float-item:not(.circle-gradient):not(.purple-sq) {
    display: none;
  }
}

@media (max-width: 480px) {
  .hero-title {
    font-size: 2.5rem;
  }

  .mixboard-subtitle {
    font-size: 12px;
  }

  .float-item {
    display: none;
  }
}
</style>
