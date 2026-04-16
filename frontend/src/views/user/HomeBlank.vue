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
                    <p class="mixboard-subtitle">
                        EXPLORE, EXPAND, AND REFINE YOUR IDEAS
                    </p>
                    <div class="hero-actions">
                        <button
                            class="btn-primary mixboard-btn"
                            @click="startJourney"
                        >
                            Get started
                        </button>
                    </div>
                </div>

                <!-- 漂浮装饰元素 -->
                <div class="floating-gallery">
                    <div class="float-item text-note">
                        Feng-AI is an AI-powered concepting board that helps you
                        explore, expand, and refine your ideas.
                    </div>
                    <div class="float-item pos-1 layers-icon">
                        <div class="layer bottom-layer"></div>
                        <div class="layer top-layer"></div>
                    </div>
                    <div class="float-item pos-2 circle-gradient"></div>
                    <div class="float-item pos-3 orange-rect"></div>
                    <div class="float-item pos-4 purple-sq"></div>
                </div>

                <!-- 手写箭头引导 -->
                <div class="hand-drawn-hint" @click="scrollToFeatures">
                    <span class="hint-text">WATCH THIS FEATURES</span>
                    <svg
                        class="hint-arrow"
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="2"
                        stroke-linecap="round"
                        stroke-linejoin="round"
                    >
                        <path d="M12 5v14M19 12l-7 7-7-7" />
                    </svg>
                </div>
            </div>
        </div>
    </div>
</template>

<script setup lang="ts">
import { ref, computed } from "vue";
import { useRouter } from "vue-router";

const router = useRouter();
const featuresRef = ref<HTMLElement | null>(null);

// 动态主题系统
const themes = [
    {
        name: "Siri Glow",
        primary: "#3b82f6",
        secondary: "#a855f7",
        tertiary: "#ec4899",
    }, // Apple Siri 风格蓝紫粉
    {
        name: "Aurora",
        primary: "#10b981",
        secondary: "#3b82f6",
        tertiary: "#a3e635",
    }, // 极光绿蓝
    {
        name: "Sunset",
        primary: "#f97316",
        secondary: "#eab308",
        tertiary: "#ef4444",
    }, // 日落橘黄红
    {
        name: "Cosmic",
        primary: "#8b5cf6",
        secondary: "#6366f1",
        tertiary: "#d946ef",
    }, // 宇宙深紫
    {
        name: "Original",
        primary: "#a3e635",
        secondary: "#34d399",
        tertiary: "#10b981",
    }, // 原始亮绿
];

const currentTheme = ref(themes[0]);

const themeStyles = computed(() => ({
    "--theme-primary": currentTheme.value.primary,
    "--theme-secondary": currentTheme.value.secondary,
    "--theme-tertiary": currentTheme.value.tertiary,
}));

const featureGroups = [
    {
        title: "服装实验室",
        items: [
            {
                label: "线稿成款",
                desc: "将手绘线稿转化为真实服装，细节尽显",
                path: "/user/sketch-to-design",
                icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/><polyline points="10 9 9 9 8 9"/></svg>',
            },
            {
                label: "线稿生成",
                desc: "根据提示词快速生成多种风格的服装线稿",
                path: "/user/line-draft",
                icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M12 2L2 7l10 5 10-5-10-5z"/><path d="M2 17l10 5 10-5"/><path d="M2 12l10 5 10-5"/></svg>',
            },
            {
                label: "面料上身",
                desc: "将指定面料材质无缝应用到模特图片上",
                path: "/user/fabric-on-body",
                icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><rect x="3" y="3" width="18" height="18" rx="2" ry="2"/><line x1="3" y1="9" x2="21" y2="9"/><line x1="9" y1="21" x2="9" y2="9"/></svg>',
            },
        ],
    },
    {
        title: "摄影商拍",
        items: [
            {
                label: "人台变模特",
                desc: "将人台图无缝转化为高度逼真的真人模特展示",
                path: "/user/mannequin-to-model",
                icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M22 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/></svg>',
            },
            {
                label: "换背景",
                desc: "一键替换商品图背景，自然融合光影",
                path: "/user/bg-swap",
                icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><rect x="3" y="3" width="18" height="18" rx="2" ry="2"/><circle cx="8.5" cy="8.5" r="1.5"/><polyline points="21 15 16 10 5 21"/></svg>',
            },
            {
                label: "款式上身",
                desc: "虚拟试衣，在不同体型上展现真实的穿搭效果",
                path: "/user/garment-on-model",
                icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M20.59 13.41l-7.17 7.17a2 2 0 0 1-2.83 0L2 12V2h10l8.59 8.59a2 2 0 0 1 0 2.82z"/><line x1="7" y1="7" x2="7.01" y2="7"/></svg>',
            },
            {
                label: "模特多角度",
                desc: "由单图生成模特不同角度的高清展示图",
                path: "/user/multi-angle",
                icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><polygon points="23 7 16 12 23 17 23 7"/><rect x="1" y="5" width="15" height="14" rx="2" ry="2"/></svg>',
            },
        ],
    },
    {
        title: "图案工作室",
        items: [
            {
                label: "画风复刻",
                desc: "提取参考图的独特画风，完美应用到新作品中",
                path: "/user/style-clone",
                icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M12 20h9"/><path d="M16.5 3.5a2.121 2.121 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z"/></svg>',
            },
        ],
    },
    {
        title: "AI 工具箱",
        items: [
            {
                label: "抠图褪底",
                desc: "发丝级精准抠图，一键提取画面主体",
                path: "/user/remove-bg",
                icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><rect x="3" y="3" width="18" height="18" rx="2" ry="2"/><path d="M8 12h8"/><path d="M12 8v8"/></svg>',
            },
            {
                label: "转矢量图",
                desc: "将位图转换为可无限缩放的高质量矢量图形",
                path: "/user/to-vector",
                icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><polygon points="12 2 2 22 12 17 22 22 12 2"/></svg>',
            },
        ],
    },
];

function navigateTo(path: string) {
    router.push(path);
}

function startJourney() {
    router.push("/user/picture");
}

function scrollToFeatures() {
    if (featuresRef.value) {
        featuresRef.value.scrollIntoView({ behavior: "smooth" });
    }
}
</script>

<style scoped>
/* 苹果风设计规范 (Apple-like Design Language) */
.home-blank {
    width: 100%;
    height: 100%;
    overflow-y: auto;
    overflow-x: hidden;
    position: relative;
    background-color: #ffffff;
    /* Google Mixboard style dotted background */
    background-image: radial-gradient(circle, #e2e4e8 1.5px, transparent 1.5px);
    background-size: 24px 24px; /* 密度再次提升 1/2: 32px -> 24px */
    background-position: center center; /* 锚定中心点 */
    background-attachment: fixed; /* 固定背景不随滚动/缩放漂移 */
    color: #1d1d1f;
    font-family:
        -apple-system, BlinkMacSystemFont, "SF Pro Display", "Segoe UI", Roboto,
        "Helvetica Neue", sans-serif;
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
    min-height: 100vh;
    display: flex;
    align-items: center;
    justify-content: center;
    position: relative;
    padding: 0;
    z-index: 1;
    overflow: hidden;
}

/* 核心舞台容器：锁定 1440x900 的设计基准 */
.hero-stage {
    width: 1440px;
    height: 900px;
    position: relative;
    flex-shrink: 0;
}

.hero-content {
    position: absolute;
    left: 320px;
    top: 350px;
    width: 800px;
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
        "Satoshi",
        -apple-system,
        "SF Pro Display",
        sans-serif;
    font-size: 90px;
    line-height: 1;
    font-weight: 600;
    margin: 0;
    letter-spacing: -0.02em;
    color: #202124;
}

.mixboard-subtitle {
    font-family:
        "Comic Sans MS", "Chalkboard SE", "Bradley Hand", "Caveat", cursive,
        sans-serif;
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
    left: 380px;
    top: 172px;
    width: 220px;
    padding: 32px;
    background: #e6ded5; /* Match exactly the warm beige */
    color: #202124;
    font-family:
        -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
    font-size: 12px;
    line-height: 1.5;
    border-radius: 16px;
    transform: rotate(1.5deg);
    box-shadow: 0 10px 30px rgba(0, 0, 0, 0.05);
}

.layers-icon {
    left: 328px;
    top: 349px;
    width: 100px;
    height: 100px;
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
    width: 75px;
    height: 75px;
    border-radius: 12px;
}

.layers-icon .bottom-layer {
    background: linear-gradient(135deg, #a855f7, #3b82f6);
    top: 13px;
    left: 2px;
    transform: rotate(-7.5deg);
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
}

.layers-icon .top-layer {
    border: 2px solid rgba(0, 0, 0, 0.2);
    top: 0px;
    left: 0px;
    background: rgba(255, 255, 255, 0.5);
    backdrop-filter: blur(4px);
    transform: rotate(15.6deg);
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
}

.circle-gradient {
    left: 394px;
    top: 540px;
    width: 124px;
    height: 124px;
    border-radius: 50%;
    background: linear-gradient(135deg, #34d399 0%, #3a92e3 73%, #3b82f6 100%);
    border: 4px solid #ffffff;
}

.orange-rect {
    left: 978px;
    top: 166px;
    width: 92px;
    height: 143px;
    border-radius: 16px;
    background: linear-gradient(180deg, #fbbf24, #f97316);
    border: 4px solid #ffffff;
    transform: rotate(-21.5deg);
}

.purple-sq {
    left: 910px;
    top: 548px;
    width: 150px;
    height: 150px;
    border-radius: 20px;
    background: conic-gradient(from 135deg, #d946ef, #3b82f6, #ec4899);
    border: 4px solid #ffffff;
    transform: rotate(8deg);
}

/* Hand drawn hint */
.hand-drawn-hint {
    position: absolute;
    left: 1007px;
    top: 432px;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 8px;
    cursor: pointer;
    z-index: 20;
    transform: rotate(-10.6deg);
}

.hint-text {
    font-family: "Comic Sans MS", "Chalkboard SE", "Bradley Hand", sans-serif;
    text-transform: uppercase;
    font-size: 14px;
    letter-spacing: 1px;
    color: #1a1a1a;
    transform: rotate(0deg);
}

.hint-arrow {
    width: 24px;
    height: 24px;
    color: #1a1a1a;
    transform: rotate(0deg);
}

/* 响应式设计 */
@media (max-width: 768px) {
    .hero-title {
        font-size: 3.5rem;
    }

    .bento-card.card-large {
        grid-column: span 1;
    }

    .hero-actions {
        flex-direction: column;
        width: 100%;
        max-width: 300px;
    }

    .btn-primary,
    .btn-secondary {
        width: 100%;
    }
}

@media (max-width: 480px) {
    .hero-title {
        font-size: 2.5rem;
    }
    .hero-subtitle {
        font-size: 1.0625rem;
    }
}
</style>
>
