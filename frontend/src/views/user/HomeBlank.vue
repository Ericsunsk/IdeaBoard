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
                            {{ primaryActionText }}
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

        <!-- 登录弹窗 -->
        <LoginModal
            :show="showLoginModal"
            @update:show="onLoginVisibleChange"
            @to-register="switchToRegister"
        />

        <!-- 注册弹窗 -->
        <RegisterModal
            :show="showRegisterModal"
            @update:show="onRegisterVisibleChange"
            @to-login="switchToLogin"
        />
    </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useUserStore } from "../../stores/user";
import LoginModal from "../../components/LoginModal.vue";
import RegisterModal from "../../components/RegisterModal.vue";

const router = useRouter();
const route = useRoute();
const userStore = useUserStore();
const showLoginModal = ref(false);
const showRegisterModal = ref(false);

const primaryActionText = computed(() =>
    userStore.isLoggedIn ? "Get started" : "Sign in"
);

function startJourney() {
    if (userStore.isLoggedIn) {
        router.push("/user/picture");
        return;
    }
    openLoginModal();
}

function getAuthQueryMode(): "login" | "register" | null {
    const auth = Array.isArray(route.query.auth) ? route.query.auth[0] : route.query.auth;
    if (auth === "login" || auth === "register") {
        return auth;
    }
    return null;
}

function replaceAuthQuery(mode: "login" | "register" | null) {
    const nextQuery = { ...route.query };
    if (mode) {
        nextQuery.auth = mode;
    } else {
        delete nextQuery.auth;
    }
    router.replace({ path: "/user/home", query: nextQuery });
}

function syncAuthModalFromRoute() {
    const mode = getAuthQueryMode();
    if (mode === "login") {
        showLoginModal.value = true;
        showRegisterModal.value = false;
        return;
    }
    if (mode === "register") {
        showLoginModal.value = false;
        showRegisterModal.value = true;
        return;
    }
    showLoginModal.value = false;
    showRegisterModal.value = false;
}

function openLoginModal() {
    showLoginModal.value = true;
    showRegisterModal.value = false;
    if (getAuthQueryMode() !== "login") {
        replaceAuthQuery("login");
    }
}

function openRegisterModal() {
    showLoginModal.value = false;
    showRegisterModal.value = true;
    if (getAuthQueryMode() !== "register") {
        replaceAuthQuery("register");
    }
}

function clearAuthModal() {
    showLoginModal.value = false;
    showRegisterModal.value = false;
    if (getAuthQueryMode()) {
        replaceAuthQuery(null);
    }
}

function onLoginVisibleChange(visible: boolean) {
    if (visible) {
        openLoginModal();
        return;
    }
    clearAuthModal();
}

function onRegisterVisibleChange(visible: boolean) {
    if (visible) {
        openRegisterModal();
        return;
    }
    clearAuthModal();
}

function switchToRegister() {
    openRegisterModal();
}

function switchToLogin() {
    openLoginModal();
}

watch(
    () => route.query.auth,
    () => {
        syncAuthModalFromRoute();
    }
);

onMounted(() => {
    syncAuthModalFromRoute();
});

function scrollToFeatures() {
    const container = document.querySelector(".home-blank") as HTMLElement | null;
    if (container) {
        container.scrollTo({ top: container.clientHeight, behavior: "smooth" });
        return;
    }
    window.scrollTo({ top: window.innerHeight, behavior: "smooth" });
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
    background-size: 24px 24px;
    background-position: center center;
    background-attachment: fixed;
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
    opacity: 0.4;
    mix-blend-mode: normal;
}

.blob-1 {
    top: 10%;
    left: 20%;
    width: 70vw;
    height: 70vw;
    background: #e0e7ff;
}

.blob-2 {
    bottom: 10%;
    right: 10%;
    width: 80vw;
    height: 80vw;
    background: #fae8ff;
}

.blob-3 {
    top: 40%;
    left: 40%;
    width: 60vw;
    height: 60vw;
    background: #f3e8ff;
}

.blob-4 {
    top: -10%;
    right: -10%;
    width: 60vw;
    height: 60vw;
    background: #ede9fe;
}

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
    gap: 12px; /* 减小全局间距: 24px -> 12px */
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
    margin-top: 0; /* 移除 margin，仅靠父级的 gap: 12px 维持间距 */
    font-weight: normal;
}

.hero-actions {
    display: flex;
    gap: 16px;
    margin-top: 16px; /* 单独给按钮区域加间距 */
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
    background: #e6ded5;
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
    background: transparent !important;
    border-radius: 0 !important;
    display: flex;
    align-items: center;
    justify-content: center;
    position: relative;
    z-index: 5;
    box-shadow: none !important;
    overflow: visible !important;
}

.layers-icon .layer {
    position: absolute;
    width: 75px;
    height: 75px;
    border-radius: 12px;
}

.layers-icon .bottom-layer {
    background: linear-gradient(135deg, #a855f7, #3b82f6);
    top: 12.5px;
    left: 1.8px;
    transform: rotate(-7.5deg);
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
}

.layers-icon .top-layer {
    border: 2px solid rgba(0, 0, 0, 0.2);
    top: 0.17px;
    left: 0px;
    background: rgba(255, 255, 255, 0.5);
    backdrop-filter: blur(4px);
    transform: rotate(15.6deg);
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
}

.circle-gradient {
    left: 392px;
    top: 556px;
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
    /* 倾斜方向翻转: 从 -21.5deg 改为 21.5deg */
    transform: rotate(21.5deg) scaleX(-1);
}

.purple-sq {
    left: 921px;
    top: 538px;
    width: 131px;
    height: 131px;
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
}

.hint-arrow {
    width: 24px;
    height: 24px;
    color: #1a1a1a;
}
</style>
