<template>
  <div class="home-layout">
    <div class="home-body">

      <!-- ====== 左侧：作品画廊 ====== -->
      <aside class="history-panel">
        <div class="history-header">
          <span>我的画廊</span>
          <span class="gallery-count" v-if="galleryTasks.length">{{ galleryTasks.length }}</span>
        </div>
        <div class="gallery-grid" v-if="galleryTasks.length > 0">
          <div
            v-for="ht in galleryTasks"
            :key="ht.task_id"
            class="gallery-item"
            :class="{ active: activeTaskId === ht.task_id }"
            @click="viewHistoryTask(ht)"
          >
            <img :src="getTaskImageUrl(ht)" alt="" loading="lazy" @error="handleImgError" />
            <div class="gallery-item-overlay">
              <span class="gallery-item-prompt">{{ ht.prompt }}</span>
            </div>
            <button class="gallery-delete-btn" @click.stop="showDeleteConfirm($event, ht.task_id)" title="删除">
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
            </button>
          </div>
        </div>
        <div class="history-empty" v-else>
          <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="#333" stroke-width="1.5"><rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="8.5" cy="8.5" r="1.5"/><path d="m21 15-5-5L5 21"/></svg>
          <p>还没有作品</p>
          <p style="font-size: 11px; color: #444;">生成图片后将在这里展示</p>
        </div>
      </aside>

      <!-- ====== 中间：生成区 ====== -->
      <main class="canvas-area">
        <!-- 未登录 -->
        <div v-if="!userStore.isLoggedIn" class="guest-prompt">
          <div class="smiley-icon">
            <svg width="120" height="120" viewBox="0 0 120 120" fill="none">
              <circle cx="60" cy="60" r="56" stroke="#a3e635" stroke-width="4" fill="none"/>
              <circle cx="44" cy="48" r="5" fill="#a3e635"/>
              <circle cx="76" cy="48" r="5" fill="#a3e635"/>
              <path d="M38 72 C38 72, 44 90, 60 90 C76 90, 82 72, 82 72" stroke="#a3e635" stroke-width="4" stroke-linecap="round" fill="none"/>
            </svg>
          </div>
          <p class="guest-title">温馨提示：</p>
          <p class="guest-desc">您当前是游客身份，暂无工具相关权限。</p>
          <p class="guest-action">
            请先 <router-link to="/user/register" class="link-highlight">注册</router-link>
            或 <router-link to="/user/login" class="link-highlight">登录</router-link>
          </p>
        </div>

        <!-- 已登录：聊天记录气泡 -->
        <div v-else class="gen-display" ref="chatMessagesRef">
          <!-- 空状态 -->
          <div v-if="messages.length === 0" class="chat-empty">
            <div class="empty-icon-wrap">
              <svg width="64" height="64" viewBox="0 0 24 24" fill="none" stroke="#a3e635" stroke-width="1.2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M12 2L2 7l10 5 10-5-10-5z"/><path d="M2 17l10 5 10-5"/><path d="M2 12l10 5 10-5"/>
              </svg>
            </div>
            <p class="chat-empty-text">描述你的创意，AI 为你绘制</p>
            <p class="chat-empty-sub">在右侧选择风格和参数，输入画面描述开始创作</p>
          </div>

          <!-- 聊天气泡 -->
          <div v-for="msg in messages" :key="msg.id" :id="msg.taskId ? 'task-' + msg.taskId : undefined" class="chat-bubble" :class="msg.role">
            <!-- 用户消息 -->
            <div v-if="msg.role === 'user'" class="bubble-row user">
              <div class="bubble-body user">
                <p class="bubble-prompt">{{ msg.prompt || '(历史任务)' }}</p>
                <div class="bubble-tags" v-if="msg.cardNames.length">
                  <span v-for="name in msg.cardNames" :key="name" class="tag">{{ name }}</span>
                </div>
              </div>
              <span class="bubble-time" v-if="msg.time">{{ msg.time }}</span>
            </div>

            <!-- AI 消息 -->
            <div v-else class="bubble-row ai">
              <!-- pending: 3:4 骨架占位 -->
              <div v-if="msg.status === 'pending'" class="skeleton-wrap">
                <div class="skeleton-shimmer"></div>
                <div class="skeleton-overlay">
                  <div class="skeleton-spinner"></div>
                  <p class="skeleton-text">AI 正在生成中...</p>
                  <p class="skeleton-sub">请耐心等待，通常需要 10-30 秒</p>
                </div>
              </div>

              <!-- failed -->
              <div v-else-if="msg.status === 'failed'" class="bubble-body failed">
                <div class="bubble-failed-row">
                  <div class="bubble-failed-icon">✕</div>
                  <div class="bubble-failed-info">
                    <div class="bubble-failed-title">生成失败</div>
                    <div class="bubble-failed-reason">{{ msg.errorMsg }}</div>
                  </div>
                </div>
              </div>

              <!-- completed -->
              <template v-else-if="msg.status === 'completed'">
                <div class="bubble-body ai-success">
                  <div v-if="msg.imageUrl" class="bubble-image-wrap" @click="openLightbox(msg.imageUrl)">
                    <img :src="msg.imageUrl" :alt="msg.prompt" class="bubble-image" @error="handleImgError" />
                    <div class="image-zoom-hint">
                      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/><line x1="11" y1="8" x2="11" y2="14"/><line x1="8" y1="11" x2="14" y2="11"/></svg>
                    </div>
                  </div>
                  <div class="bubble-footer">
                    <span class="bubble-meta" v-if="msg.elapsed">耗时 {{ msg.elapsed }}s</span>
                    <div class="result-actions">
                      <button class="result-action-btn" @click="downloadImage(msg.imageUrl, msg.prompt)">
                        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
                        下载
                      </button>
                      <button class="result-action-btn" @click="copyPrompt(msg.prompt)">
                        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
                        复制提示词
                      </button>
                      <button class="result-action-btn primary" @click="regenerate(msg.prompt)">
                        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/></svg>
                        再次生成
                      </button>
                      <button class="result-action-btn danger" @click="showDeleteConfirm($event, msg.taskId)">
                        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
                        删除
                      </button>
                    </div>
                  </div>
                </div>
              </template>
            </div>
          </div>
        </div>
      </main>

      <!-- ====== 右侧：工作区 ====== -->
      <aside class="control-panel">
        <div class="tutorial-float">操<br/>作<br/>教<br/>程</div>

        <div class="panel-top">
          <div class="panel-tabs">
            <button class="panel-tab" :class="{ active: modeTab === 'simple' }" @click="modeTab = 'simple'">简洁模式</button>
            <button class="panel-tab" :class="{ active: modeTab === 'pro' }" @click="modeTab = 'pro'">专业模式</button>
          </div>
          <a class="quick-edit-link" href="#" @click.prevent>一句话改图</a>
        </div>

        <!-- ====== 简洁模式 ====== -->
        <div v-if="modeTab === 'simple'" class="panel-scroll">
          <!-- ① 增效模型 -->
          <div class="section-block">
            <div class="section-header">
              <span class="section-num">①</span>
              <span class="section-title">增效模型</span>
              <n-tooltip trigger="hover" :style="{ maxWidth: '240px' }">
                <template #trigger>
                  <span class="section-info">ⓘ</span>
                </template>
                调用为您预设好的增效模型，对所需的图片效果做不一样风格的精准调用
              </n-tooltip>
            </div>
            <div class="brand-cards-grid">
              <div v-if="loadingCards" class="loading-state"><n-spin size="small" /></div>
              <template v-else-if="styleCards.length > 0">
                <div v-for="card in displayCards" :key="card.id" class="brand-card" :class="{ selected: selectedCardIds.includes(card.id) }" @click="toggleCard(card.id, card.is_exclusive)">
                  <div class="brand-card-cover">
                    <img :src="card.cover_image" :alt="card.name" loading="lazy" @error="handleImgError" />
                    <div v-if="selectedCardIds.includes(card.id)" class="card-check">
                      <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="white" stroke-width="3"><polyline points="20 6 9 17 4 12"/></svg>
                    </div>
                  </div>
                  <div class="brand-card-name">{{ card.name }}</div>
                </div>
              </template>
              <div v-else class="empty-state"><p>暂无可用模型</p></div>
            </div>
            <button class="view-all-btn" @click="showAllCards = true">查看全部模型</button>
          </div>

          <!-- ② 画面描述 -->
          <div class="section-block">
            <div class="section-header">
              <span class="section-num">②</span>
              <span class="section-title">画面描述</span>
              <span class="section-info">ⓘ</span>
            </div>
            <n-input v-model:value="userPrompt" type="textarea" placeholder="描述（人物主体+色彩+面料材质+服发发式+动作+背景）" :rows="3" :maxlength="500" clearable @clear="userPrompt = ''" />
          </div>

          <!-- ③ 图片尺寸 -->
          <div class="section-block">
            <div class="section-header">
              <span class="section-num">③</span>
              <span class="section-title">图片尺寸</span>
            </div>
            <div class="ratio-grid">
              <button v-for="r in ratios" :key="r.value" class="ratio-btn" :class="{ active: selectedRatio === r.value && selectedRatio !== 'custom' }" @click="selectedRatio = r.value">
                <div class="ratio-icon" :style="r.iconStyle"></div>
                <span>{{ r.label }}</span>
              </button>
            </div>
            <div class="custom-size-row">
              <span class="custom-size-label" :class="{ active: selectedRatio === 'custom' }" @click="selectedRatio = 'custom'">自定义</span>
              <input type="number" class="custom-size-input" v-model.number="customWidth" placeholder="宽" min="512" max="4096" step="64" @focus="selectedRatio = 'custom'" />
              <span class="size-separator">×</span>
              <input type="number" class="custom-size-input" v-model.number="customHeight" placeholder="高" min="512" max="4096" step="64" @focus="selectedRatio = 'custom'" />
              <span class="size-unit">px</span>
            </div>
          </div>

          <!-- ④ 出图张数 -->
          <div class="section-block">
            <div class="section-header">
              <span class="section-num">④</span>
              <span class="section-title">出图张数</span>
            </div>
            <div class="count-grid">
              <button v-for="n in [1, 2, 4]" :key="n" class="count-btn" :class="{ active: imageCount === n }" @click="imageCount = n">{{ n }} 张</button>
            </div>
          </div>

          <!-- ⑤ 图片清晰度 -->
          <div class="section-block">
            <div class="section-header">
              <span class="section-num">⑤</span>
              <span class="section-title">图片清晰度</span>
            </div>
            <div class="quality-grid">
              <button v-for="q in qualityOptions" :key="q.value" class="quality-btn" :class="{ active: selectedQuality === q.value }" @click="selectedQuality = q.value">
                <span class="quality-label">{{ q.label }}</span>
                <span class="quality-desc">{{ q.desc }}</span>
              </button>
            </div>
          </div>

        </div>

        <!-- ====== 专业模式 ====== -->
        <div v-else-if="modeTab === 'pro'" class="panel-scroll">

          <!-- ① 上传图片 -->
          <div class="section-block">
            <div class="section-header">
              <span class="section-num">①</span>
              <span class="section-title">上传图片</span>
              <n-tooltip trigger="hover" :style="{ maxWidth: '240px' }">
                <template #trigger>
                  <span class="section-info">ⓘ</span>
                </template>
                上传参考图片，AI 将基于图片进行创作。最多 6 张，每张不超过 30MB
              </n-tooltip>
            </div>
            <ImageUploader
              v-model="proRefImages"
              :gallery-groups="allGalleryGroups"
              :max="6"
              :max-size-m-b="30"
            />
          </div>

          <!-- ② 增效模型 -->
          <div class="section-block">
            <div class="section-header">
              <span class="section-num">②</span>
              <span class="section-title">增效模型</span>
              <n-tooltip trigger="hover" :style="{ maxWidth: '240px' }">
                <template #trigger>
                  <span class="section-info">ⓘ</span>
                </template>
                调用为您预设好的增效模型，对所需的图片效果做不一样风格的精准调用
              </n-tooltip>
            </div>
            <div class="brand-cards-grid">
              <div v-if="loadingCards" class="loading-state"><n-spin size="small" /></div>
              <template v-else-if="styleCards.length > 0">
                <div v-for="card in displayCards" :key="card.id" class="brand-card" :class="{ selected: proSelectedCardIds.includes(card.id) }" @click="proToggleCard(card.id, card.is_exclusive)">
                  <div class="brand-card-cover">
                    <img :src="card.cover_image" :alt="card.name" loading="lazy" @error="handleImgError" />
                    <div v-if="proSelectedCardIds.includes(card.id)" class="card-check">
                      <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="white" stroke-width="3"><polyline points="20 6 9 17 4 12"/></svg>
                    </div>
                  </div>
                  <div class="brand-card-name">{{ card.name }}</div>
                </div>
              </template>
              <div v-else class="empty-state"><p>暂无可用模型</p></div>
            </div>
            <button class="view-all-btn" @click="proShowAllCards = true">查看全部模型</button>
          </div>

          <!-- ③ 画面描述 -->
          <div class="section-block">
            <div class="section-header">
              <span class="section-num">③</span>
              <span class="section-title">画面描述</span>
              <span class="section-info">ⓘ</span>
            </div>
            <n-input v-model:value="proUserPrompt" type="textarea" placeholder="描述（人物主体+色彩+面料材质+服发发式+动作+背景）" :rows="3" :maxlength="500" clearable @clear="proUserPrompt = ''" />
          </div>

          <!-- ④ 图片尺寸 -->
          <div class="section-block">
            <div class="section-header">
              <span class="section-num">④</span>
              <span class="section-title">图片尺寸</span>
            </div>
            <div class="ratio-grid">
              <button v-for="r in ratios" :key="r.value" class="ratio-btn" :class="{ active: proSelectedRatio === r.value && proSelectedRatio !== 'custom' }" @click="proSelectedRatio = r.value">
                <div class="ratio-icon" :style="r.iconStyle"></div>
                <span>{{ r.label }}</span>
              </button>
            </div>
            <div class="custom-size-row">
              <span class="custom-size-label" :class="{ active: proSelectedRatio === 'custom' }" @click="proSelectedRatio = 'custom'">自定义</span>
              <input type="number" class="custom-size-input" v-model.number="proCustomWidth" placeholder="宽" min="512" max="4096" step="64" @focus="proSelectedRatio = 'custom'" />
              <span class="size-separator">×</span>
              <input type="number" class="custom-size-input" v-model.number="proCustomHeight" placeholder="高" min="512" max="4096" step="64" @focus="proSelectedRatio = 'custom'" />
              <span class="size-unit">px</span>
            </div>
          </div>

          <!-- ⑤ 变化幅度 -->
          <div class="section-block" v-if="proRefImages.length > 0">
            <div class="section-header">
              <span class="section-num">⑤</span>
              <span class="section-title">变化幅度</span>
              <n-tooltip trigger="hover" :style="{ maxWidth: '260px' }">
                <template #trigger>
                  <span class="section-info">ⓘ</span>
                </template>
                微调优化：在参考图基础上进行微小调整，保留更多原图特征。创意延伸：在参考图基础上大幅度创新，生成更有创意的变体。
              </n-tooltip>
            </div>
            <div class="variation-grid">
              <button class="variation-btn" :class="{ active: proVariationStrength === 'low' }" @click="proVariationStrength = 'low'">微调优化</button>
              <button class="variation-btn" :class="{ active: proVariationStrength === 'high' }" @click="proVariationStrength = 'high'">创意延伸</button>
            </div>
          </div>

          <!-- ⑥ 出图张数 -->
          <div class="section-block">
            <div class="section-header">
              <span class="section-num">{{ proRefImages.length > 0 ? '⑥' : '⑤' }}</span>
              <span class="section-title">出图张数</span>
            </div>
            <div class="count-grid">
              <button v-for="n in [1, 2, 4]" :key="n" class="count-btn" :class="{ active: proImageCount === n }" @click="proImageCount = n">{{ n }} 张</button>
            </div>
          </div>

          <!-- ⑦ 图片清晰度 -->
          <div class="section-block">
            <div class="section-header">
              <span class="section-num">{{ proRefImages.length > 0 ? '⑦' : '⑥' }}</span>
              <span class="section-title">图片清晰度</span>
            </div>
            <div class="quality-grid">
              <button v-for="q in qualityOptions" :key="q.value" class="quality-btn" :class="{ active: proSelectedQuality === q.value }" @click="proSelectedQuality = q.value">
                <span class="quality-label">{{ q.label }}</span>
                <span class="quality-desc">{{ q.desc }}</span>
              </button>
            </div>
          </div>

        </div>

        <!-- 按钮组 - 固定底部 -->
        <div class="panel-bottom">
          <div class="button-group">
            <template v-if="modeTab === 'pro'">
              <button class="btn-generate" :disabled="!proCanGenerate || generating" @click="proHandleGenerate">
                {{ generating ? '生成中...' : '生成图片' }}
              </button>
              <button class="btn-clear" :disabled="generating" @click="proHandleClear">清空</button>
            </template>
            <template v-else>
              <button class="btn-generate" :disabled="!canGenerate || generating" @click="handleGenerate">
                {{ generating ? '生成中...' : '生成图片' }}
              </button>
              <button class="btn-clear" :disabled="generating" @click="handleClear">清空</button>
            </template>
          </div>
        </div>
      </aside>
    </div>

    <!-- ========== 图片放大预览 ========== -->
    <div v-if="lightboxUrl" class="lightbox-overlay" @click.self="lightboxUrl = ''">
      <div class="lightbox-container">
        <img :src="lightboxUrl" class="lightbox-image" />
        <button class="lightbox-close" @click="lightboxUrl = ''">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
        </button>
        <div class="lightbox-actions">
          <button class="lightbox-action-btn" @click="downloadImage(lightboxUrl, 'image')">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
            下载原图
          </button>
          <button class="lightbox-action-btn seg-btn" @click="openSegWorkspace(lightboxUrl)">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/></svg>
            智能分割
          </button>
        </div>
      </div>
    </div>

    <!-- ========== 分割工作台弹窗 ========== -->
    <n-modal v-model:show="segWorkspaceVisible" preset="card" title="智能分割工作台" :style="{ maxWidth: '900px', maxHeight: '90vh' }" :scrollable="true">
      <div class="seg-workspace">
        <div class="seg-type-bar">
          <button v-for="st in segTypes" :key="st.value" class="seg-type-btn" :class="{ active: segActiveType === st.value }" @click="selectSegType(st.value)" :disabled="segLoading">
            <span class="seg-type-icon">{{ st.icon }}</span>
            <span>{{ st.label }}</span>
          </button>
        </div>
        <div class="seg-content">
          <div class="seg-panel">
            <div class="seg-panel-label">原图</div>
            <div class="seg-panel-img">
              <img :src="segSourceUrl" alt="原图" />
            </div>
          </div>
          <div class="seg-arrow">
            <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="#666" stroke-width="2"><line x1="5" y1="12" x2="19" y2="12"/><polyline points="12 5 19 12 12 19"/></svg>
          </div>
          <div class="seg-panel">
            <div class="seg-panel-label">分割结果</div>
            <div class="seg-panel-img result">
              <n-spin v-if="segLoading" size="medium" />
              <img v-else-if="segResultUrl" :src="segResultUrl" alt="结果" />
              <div v-else class="seg-placeholder">{{ segActiveType ? '点击下方按钮开始分割' : '请先选择分割类型' }}</div>
            </div>
          </div>
        </div>
      </div>
      <template #footer>
        <div class="modal-footer">
          <span class="seg-status-text" v-if="segError">{{ segError }}</span>
          <n-button v-if="!segResultUrl" type="primary" :disabled="!segActiveType || segLoading" :loading="segLoading" @click="runSegment(segActiveType)">开始分割</n-button>
          <template v-else>
            <n-button @click="segResultUrl = ''; segError = ''">重新分割</n-button>
            <n-button :loading="segSaving" :disabled="segSaved" @click="saveSegResult">
              {{ segSaved ? '✓ 已保存' : '保存到画廊' }}
            </n-button>
            <n-button @click="downloadImage(segResultUrl, 'segment')">下载分割结果</n-button>
          </template>
        </div>
      </template>
    </n-modal>

    <!-- ========== 全部卡片弹窗 ========== -->
    <n-modal v-model:show="showAllCards" preset="card" title="全部风格模型" :style="{ maxWidth: '720px', maxHeight: '80vh' }" :scrollable="true">
      <div class="all-cards-grid">
        <div v-for="card in styleCards" :key="card.id" class="brand-card" :class="{ selected: selectedCardIds.includes(card.id) }" @click="toggleCard(card.id, card.is_exclusive)">
          <div class="brand-card-cover">
            <img :src="card.cover_image" :alt="card.name" @error="handleImgError" />
            <div v-if="selectedCardIds.includes(card.id)" class="card-check">
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="white" stroke-width="3"><polyline points="20 6 9 17 4 12"/></svg>
            </div>
          </div>
          <div class="brand-card-name">{{ card.name }}</div>
        </div>
      </div>
      <template #footer>
        <div class="modal-footer">
          <span class="selected-count">已选 {{ selectedCardIds.length }} 个</span>
          <n-button type="primary" @click="showAllCards = false">确定</n-button>
        </div>
      </template>
    </n-modal>

    <!-- ========== 专业模式 - 全部卡片弹窗 ========== -->
    <n-modal v-model:show="proShowAllCards" preset="card" title="全部风格模型" :style="{ maxWidth: '720px', maxHeight: '80vh' }" :scrollable="true">
      <div class="all-cards-grid">
        <div v-for="card in styleCards" :key="card.id" class="brand-card" :class="{ selected: proSelectedCardIds.includes(card.id) }" @click="proToggleCard(card.id, card.is_exclusive)">
          <div class="brand-card-cover">
            <img :src="card.cover_image" :alt="card.name" @error="handleImgError" />
            <div v-if="proSelectedCardIds.includes(card.id)" class="card-check">
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="white" stroke-width="3"><polyline points="20 6 9 17 4 12"/></svg>
            </div>
          </div>
          <div class="brand-card-name">{{ card.name }}</div>
        </div>
      </div>
      <template #footer>
        <div class="modal-footer">
          <span class="selected-count">已选 {{ proSelectedCardIds.length }} 个</span>
          <n-button type="primary" @click="proShowAllCards = false">确定</n-button>
        </div>
      </template>
    </n-modal>

    <!-- 删除确认气泡 -->
    <Teleport to="body">
      <div v-if="deleteConfirm.show" class="delete-popover" :style="{ left: deleteConfirm.x + 'px', top: deleteConfirm.y + 'px' }">
        <div class="delete-popover-text">确认删除？</div>
        <div class="delete-popover-actions">
          <button class="dp-btn dp-cancel" @click="deleteConfirm.show = false">取消</button>
          <button class="dp-btn dp-confirm" @click="confirmDelete">删除</button>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import { NButton, NInput, NModal, NSpin, NTooltip, useMessage, useDialog } from 'naive-ui'
import { styleCardApi, taskApi, segApi } from '../../api/index'
import type { StyleCard, Task } from '../../api/index'
import { useUserStore } from '../../stores/user'
import ImageUploader from '../../components/ImageUploader.vue'
import { buildGalleryGroups } from '../../utils/galleryGroups'
import { downloadImageByProxy, getTaskImageUrl as extractTaskImageUrl } from '../../utils/task-history'

const router = useRouter()
const userStore = useUserStore()
const message = useMessage()
const dialog = useDialog()

// ============ 模式 Tab ============
const modeTab = ref<'simple' | 'pro'>('simple')

// ============ 风格卡片 ============
const styleCards = ref<StyleCard[]>([])
const selectedCardIds = ref<number[]>([])
const loadingCards = ref(false)
const showAllCards = ref(false)
const MAX_DISPLAY = 12
const displayCards = computed(() => styleCards.value.slice(0, MAX_DISPLAY))

// ============ 画廊（仅显示成功出图的任务，排除其他功能页任务） ============
const galleryTasks = computed(() =>
  historyTasks.value.filter(t => {
    if (t.status !== 'completed' || !t.result) return false
    if (t.prompt && /^\[[\w\u4e00-\u9fff -]+\] /.test(t.prompt)) return false
    try { return !!JSON.parse(t.result).image_url } catch { return false }
  })
)

function getTaskImageUrl(task: Task): string {
  return extractTaskImageUrl(task)
}

// ============ 图片尺寸 ============
const ratios = [
  { label: '3:2', value: '3:2', iconStyle: { width: '24px', height: '16px' } },
  { label: '16:9', value: '16:9', iconStyle: { width: '28px', height: '16px' } },
  { label: '1:1', value: '1:1', iconStyle: { width: '18px', height: '18px' } },
  { label: '2:3', value: '2:3', iconStyle: { width: '16px', height: '24px' } },
  { label: '3:4', value: '3:4', iconStyle: { width: '16px', height: '22px' } },
  { label: '9:16', value: '9:16', iconStyle: { width: '14px', height: '26px' } },
]
const selectedRatio = ref('3:4')
const customWidth = ref<number | null>(1024)
const customHeight = ref<number | null>(1365)

// ============ 出图张数 ============
const imageCount = ref(1)

// ============ 图片清晰度 ============
const qualityOptions = [
  { label: '1K', value: '1k', desc: '1024px' },
  { label: '2K', value: '2k', desc: '2048px' },
  { label: '4K', value: '4k', desc: '4096px' },
]
const selectedQuality = ref('1k')

// ============ 提示词 ============
const userPrompt = ref('')

// ============ 专业模式独立状态 ============
const proSelectedCardIds = ref<number[]>([])
const proUserPrompt = ref('')
const proSelectedRatio = ref('3:4')
const proCustomWidth = ref<number | null>(1024)
const proCustomHeight = ref<number | null>(1365)
const proImageCount = ref(1)
const proSelectedQuality = ref('1k')
const proShowAllCards = ref(false)
const proVariationStrength = ref<'low' | 'high'>('low')

// ============ 专业模式：上传图片 ============
const proRefImages = ref<string[]>([])

const allGalleryGroups = computed(() => buildGalleryGroups(historyTasks.value))

// ============ 历史任务 ============
const historyTasks = ref<Task[]>([])
const activeTaskId = ref<string | null>(null)
let historyInitialized = false

async function loadHistory() {
  if (!userStore.isLoggedIn) return
  try {
    const res: any = await taskApi.list({ page: 1, page_size: 50 })
    if (res.code === 0) {
      historyTasks.value = res.data.list || []
      if (!historyInitialized) {
        initChatFromHistory()
        historyInitialized = true
      }
    }
  } catch (e) {
    console.error('Failed to load history', e)
  }
}

function initChatFromHistory() {
  const sorted = [...historyTasks.value]
    .filter(t => !(t.prompt && /^\[[\w\u4e00-\u9fff -]+\] /.test(t.prompt)))
    .reverse()
  const historyMsgs: ChatMessage[] = []
  for (const task of sorted) {
    historyMsgs.push({
      id: 'hist_user_' + task.task_id,
      role: 'user',
      prompt: task.prompt,
      cardNames: [],
      time: formatTime(task.created_at),
    })
    const aiMsg: ChatMessage = {
      id: 'hist_ai_' + task.task_id,
      role: 'assistant',
      prompt: task.prompt,
      cardNames: [],
      taskId: task.task_id,
      status: (task.status === 'completed' || task.status === 'failed') ? task.status : 'pending',
    }
    if (task.status === 'completed' && task.result) {
      try {
        const r = JSON.parse(task.result)
        aiMsg.imageUrl = r.image_url || ''
      } catch {}
      if (task.completed_at && task.created_at) {
        const t = new Date(task.completed_at).getTime() - new Date(task.created_at).getTime()
        aiMsg.elapsed = (t / 1000).toFixed(1)
      }
    } else if (task.status === 'failed') {
      aiMsg.errorMsg = task.error_msg || ''
      if (!aiMsg.errorMsg && task.result) {
        try { aiMsg.errorMsg = JSON.parse(task.result).error || '生成失败' } catch { aiMsg.errorMsg = '生成失败' }
      }
      if (!aiMsg.errorMsg) aiMsg.errorMsg = '生成失败'
    }
    historyMsgs.push(aiMsg)
  }
  messages.value = historyMsgs

  // 对仍在进行中的任务自动恢复轮询（从最新的开始找）
  nextTick(() => {
    for (let i = messages.value.length - 1; i >= 0; i--) {
      const msg = messages.value[i]
      if (msg.role === 'assistant' && msg.status === 'pending' && msg.taskId) {
        generating.value = true
        pollTaskStatus(msg.taskId, msg)
        break
      }
    }
    scrollToBottom()
    setTimeout(() => scrollToBottom(), 500)
    setTimeout(() => scrollToBottom(), 1500)
  })
}

const deleteConfirm = ref<{ show: boolean; x: number; y: number; taskId: string }>({ show: false, x: 0, y: 0, taskId: '' })

function showDeleteConfirm(e: MouseEvent, taskId?: string) {
  if (!taskId) return
  deleteConfirm.value = { show: true, x: e.clientX, y: e.clientY, taskId }
  const close = (ev: MouseEvent) => {
    const el = document.querySelector('.delete-popover')
    if (el && !el.contains(ev.target as Node)) {
      deleteConfirm.value.show = false
      document.removeEventListener('mousedown', close)
    }
  }
  setTimeout(() => document.addEventListener('mousedown', close), 0)
}

async function confirmDelete() {
  const taskId = deleteConfirm.value.taskId
  deleteConfirm.value.show = false
  if (!taskId) return
  try {
    const res: any = await taskApi.delete(taskId)
    if (res.code === 0) {
      historyTasks.value = historyTasks.value.filter(t => t.task_id !== taskId)
      messages.value = messages.value.filter(m =>
        m.id !== 'hist_user_' + taskId && m.id !== 'hist_ai_' + taskId &&
        m.taskId !== taskId && m.id !== 'user_' + taskId && m.id !== 'ai_' + taskId
      )
      if (activeTaskId.value === taskId) activeTaskId.value = null
    }
  } catch (e) {
    console.error('Delete task failed', e)
  }
}

function viewHistoryTask(task: Task) {
  const el = document.getElementById('task-' + task.task_id)
  if (el) {
    el.scrollIntoView({ behavior: 'smooth', block: 'center' })
    el.classList.add('highlight')
    setTimeout(() => el.classList.remove('highlight'), 2000)
  }
}

function statusLabel(status: string) {
  const map: Record<string, string> = { pending: '等待中', running: '生成中', completed: '完成', failed: '失败' }
  return map[status] || status
}

function formatTime(t: string) {
  if (!t) return ''
  const d = new Date(t)
  const now = new Date()
  const diff = now.getTime() - d.getTime()
  if (diff < 60000) return '刚刚'
  if (diff < 3600000) return Math.floor(diff / 60000) + ' 分钟前'
  if (diff < 86400000) return Math.floor(diff / 3600000) + ' 小时前'
  if (diff < 259200000) return Math.floor(diff / 86400000) + ' 天前'
  return d.toLocaleDateString()
}

// ============ 图片放大 & 下载 ============
const lightboxUrl = ref('')

function openLightbox(url?: string) {
  if (url) lightboxUrl.value = url
}

function downloadImage(url?: string, _name?: string) {
  downloadImageByProxy(url)
  message.success('下载已开始')
}

// ============ 复制提示词 & 再次生成 ============
function copyPrompt(prompt: string) {
  navigator.clipboard.writeText(prompt).then(() => {
    message.success('提示词已复制')
  }).catch(() => {
    message.error('复制失败')
  })
}

function regenerate(prompt: string) {
  userPrompt.value = prompt
  activeTaskId.value = null
  handleGenerate()
}

// ============ 初始化 ============
onMounted(async () => {
  loadingCards.value = true
  try {
    const res: any = await styleCardApi.getEnabled()
    if (res.code === 0) {
      styleCards.value = res.data.list
    }
  } catch (e) {
    console.error('Failed to load style cards', e)
  } finally {
    loadingCards.value = false
  }
  loadHistory()
})

function toggleCard(id: number, isExclusive: boolean) {
  const idx = selectedCardIds.value.indexOf(id)
  if (idx >= 0) {
    selectedCardIds.value.splice(idx, 1)
  } else {
    if (isExclusive) {
      selectedCardIds.value = [id]
    } else {
      selectedCardIds.value.push(id)
    }
  }
}

// ============ 专业模式独立方法 ============
function proToggleCard(id: number, isExclusive: boolean) {
  const idx = proSelectedCardIds.value.indexOf(id)
  if (idx >= 0) {
    proSelectedCardIds.value.splice(idx, 1)
  } else {
    if (isExclusive) {
      proSelectedCardIds.value = [id]
    } else {
      proSelectedCardIds.value.push(id)
    }
  }
}

const proCanGenerate = computed(() => {
  if (!userStore.isLoggedIn) return false
  if (generating.value) return false
  if (!proUserPrompt.value.trim()) return false
  return true
})

async function proHandleGenerate() {
  if (!userStore.isLoggedIn) {
    message.warning('请先登录')
    router.push('/user/login')
    return
  }
  if (!proUserPrompt.value.trim()) {
    message.warning('请输入画面描述')
    return
  }

  activeTaskId.value = null

  const cardNames = styleCards.value
    .filter(c => proSelectedCardIds.value.includes(c.id))
    .map(c => c.name)

  const msgId = 'msg_' + Date.now()
  const userMsg: ChatMessage = { id: msgId, role: 'user', prompt: proUserPrompt.value.trim(), cardNames }
  messages.value.push(userMsg)

  const assistantMsgRaw: ChatMessage = {
    id: 'loading_' + Date.now(), role: 'assistant', prompt: proUserPrompt.value.trim(), cardNames: [], status: 'pending',
  }
  messages.value.push(assistantMsgRaw)
  const assistantMsg = messages.value[messages.value.length - 1]
  scrollToBottom()

  generating.value = true
  currentTaskId.value = null

  try {
    const submitParams: Record<string, any> = {
      ratio: proSelectedRatio.value === 'custom' ? undefined : proSelectedRatio.value,
      custom_width: proSelectedRatio.value === 'custom' ? (proCustomWidth.value || 1024) : undefined,
      custom_height: proSelectedRatio.value === 'custom' ? (proCustomHeight.value || 1365) : undefined,
      count: proImageCount.value,
      quality: proSelectedQuality.value,
    }
    if (proRefImages.value.length > 0) {
      submitParams.image_urls = proRefImages.value
      submitParams.variation_strength = proVariationStrength.value
    }
    const res: any = await taskApi.submit({
      prompt: proUserPrompt.value.trim(),
      card_ids: proSelectedCardIds.value.length > 0 ? proSelectedCardIds.value : undefined,
      params: submitParams,
    })
    if (res.code === 0) {
      currentTaskId.value = res.data.task_id
      assistantMsg.taskId = res.data.task_id
      pollTaskStatus(res.data.task_id, assistantMsg)
    } else {
      assistantMsg.status = 'failed'
      assistantMsg.errorMsg = res.msg || '提交失败'
      generating.value = false
    }
  } catch (e: any) {
    assistantMsg.status = 'failed'
    assistantMsg.errorMsg = e.msg || '网络错误，请重试'
    generating.value = false
  }
}

function proHandleClear() {
  proSelectedCardIds.value = []
  proUserPrompt.value = ''
  proSelectedRatio.value = '3:4'
  proImageCount.value = 1
  proSelectedQuality.value = '1k'
  proCustomWidth.value = 1024
  proCustomHeight.value = 1365
  proRefImages.value = []
  proVariationStrength.value = 'low'
  currentTaskId.value = null
  messages.value = []
  activeTaskId.value = null
  historyInitialized = false
  loadHistory()
}

// ============ 分割工作台 ============
const segWorkspaceVisible = ref(false)
const segSourceUrl = ref('')
const segResultUrl = ref('')
const segActiveType = ref('')
const segLoading = ref(false)
const segError = ref('')
const segResultCache = ref<Record<string, string>>({})
const segSaving = ref(false)
const segSaved = ref(false)

const segTypes = [
  { value: 'body', label: '人体分割', icon: '🧍' },
  { value: 'head', label: '头像分割', icon: '👤' },
  { value: 'clothing', label: '服饰分割', icon: '👗' },
  { value: 'commodity', label: '商品分割', icon: '📦' },
  { value: 'common', label: '通用分割', icon: '✂️' },
]

function openSegWorkspace(url: string) {
  segSourceUrl.value = url
  segResultUrl.value = ''
  segActiveType.value = ''
  segError.value = ''
  segLoading.value = false
  segResultCache.value = {}
  lightboxUrl.value = ''
  segWorkspaceVisible.value = true
}

function selectSegType(type_: string) {
  if (type_ === segActiveType.value) return
  segActiveType.value = type_
  segError.value = ''
  segSaved.value = false
  segResultUrl.value = segResultCache.value[type_] || ''
}

async function runSegment(type_: string) {
  if (!type_) return
  segResultUrl.value = ''
  segError.value = ''
  segLoading.value = true
  try {
    const res: any = await segApi.segment({ image_url: segSourceUrl.value, seg_type: type_ })
    if (res.code === 0) {
      segResultUrl.value = res.data.result_url
      segResultCache.value[type_] = res.data.result_url
      segSaved.value = false
    } else {
      segError.value = res.msg || '分割失败'
    }
  } catch (e: any) {
    segError.value = e.msg || '网络错误'
  } finally {
    segLoading.value = false
  }
}

async function saveSegResult() {
  if (!segResultUrl.value || !segActiveType.value) return
  segSaving.value = true
  try {
    const res: any = await segApi.save({ result_url: segResultUrl.value, seg_type: segActiveType.value })
    if (res.code === 0) {
      segSaved.value = true
      loadHistory()
    } else {
      segError.value = res.msg || '保存失败'
    }
  } catch (e: any) {
    segError.value = e.msg || '保存失败'
  } finally {
    segSaving.value = false
  }
}

function handleImgError(e: Event) {
  const img = e.target as HTMLImageElement
  img.style.background = '#27272a'
  img.src = 'data:image/svg+xml;base64,PHN2ZyB3aWR0aD0iMjQiIGhlaWdodD0iMjQiIHZpZXdCb3g9IjAgMCAyNCAyNCIgZmlsbD0ibm9uZSIgeG1sbnM9Imh0dHA6Ly93d3cudzMub3JnLzIwMDAvc3ZnIj48cmVjdCB3aWR0aD0iMjQiIGhlaWdodD0iMjQiIHJ4PSIyIiBmaWxsPSIjMzM2NjZmMSIgZmlsbC1vcGFjaXR5PSIwLjEiLz48cGF0aCBkPSJNMyA5aDE4bC0xOCA4eiIgZmlsbD0ibm9uZSIgc3Ryb2tlPSIjNTU1NTY2IiBzdHJva2Utd2lkdGg9IjIiLz48L3N2Zz4='
}

// ============ 消息气泡 ============
interface ChatMessage {
  id: string
  role: 'user' | 'assistant'
  prompt: string
  cardNames: string[]
  imageUrl?: string
  elapsed?: string
  errorMsg?: string
  taskId?: string
  status?: 'pending' | 'completed' | 'failed'
  time?: string
}

const messages = ref<ChatMessage[]>([])
const chatMessagesRef = ref<HTMLElement | null>(null)

function scrollToBottom() {
  nextTick(() => {
    if (chatMessagesRef.value) {
      chatMessagesRef.value.scrollTop = chatMessagesRef.value.scrollHeight
    }
  })
}

// ============ 生成 ============
const generating = ref(false)
const currentTaskId = ref<string | null>(null)

const canGenerate = computed(() => {
  if (!userStore.isLoggedIn) return false
  if (generating.value) return false
  if (!userPrompt.value.trim()) return false
  return true
})

async function handleGenerate() {
  if (!userStore.isLoggedIn) {
    message.warning('请先登录')
    router.push('/user/login')
    return
  }
  if (!userPrompt.value.trim()) {
    message.warning('请输入画面描述')
    return
  }

  // 清除历史查看
  activeTaskId.value = null

  const cardNames = styleCards.value
    .filter(c => selectedCardIds.value.includes(c.id))
    .map(c => c.name)

  const msgId = 'msg_' + Date.now()
  const userMsg: ChatMessage = { id: msgId, role: 'user', prompt: userPrompt.value.trim(), cardNames }
  messages.value.push(userMsg)

  const assistantMsgRaw: ChatMessage = {
    id: 'loading_' + Date.now(), role: 'assistant', prompt: userPrompt.value.trim(), cardNames: [], status: 'pending',
  }
  messages.value.push(assistantMsgRaw)
  // Get the reactive proxy version so Vue detects mutations
  const assistantMsg = messages.value[messages.value.length - 1]
  scrollToBottom()

  generating.value = true
  currentTaskId.value = null

  try {
    const res: any = await taskApi.submit({
      prompt: userPrompt.value.trim(),
      card_ids: selectedCardIds.value.length > 0 ? selectedCardIds.value : undefined,
      params: {
        ratio: selectedRatio.value === 'custom' ? undefined : selectedRatio.value,
        custom_width: selectedRatio.value === 'custom' ? (customWidth.value || 1024) : undefined,
        custom_height: selectedRatio.value === 'custom' ? (customHeight.value || 1365) : undefined,
        count: imageCount.value,
        quality: selectedQuality.value,
      }
    })
    if (res.code === 0) {
      currentTaskId.value = res.data.task_id
      assistantMsg.taskId = res.data.task_id
      pollTaskStatus(res.data.task_id, assistantMsg)
    } else {
      assistantMsg.status = 'failed'
      assistantMsg.errorMsg = res.msg || '提交失败'
      generating.value = false
    }
  } catch (e: any) {
    assistantMsg.status = 'failed'
    assistantMsg.errorMsg = e.msg || '网络错误，请重试'
    generating.value = false
  }
}

let pollTimer: number | null = null

async function pollTaskStatus(taskId: string, assistantMsg: ChatMessage) {
  if (pollTimer) clearTimeout(pollTimer)
  try {
    const res: any = await taskApi.get(taskId)
    console.log('[Poll] response:', res.code, res.data?.task?.status, res.data?.task?.result?.substring(0, 80))
    if (res.code === 0) {
      const task = res.data.task
      if (task.status === 'completed') {
        try {
          const result = JSON.parse(task.result || '{}')
          console.log('[Poll] completed, image_url:', result.image_url)
          assistantMsg.imageUrl = result.image_url || ''
          if (!assistantMsg.imageUrl) assistantMsg.errorMsg = '生成完成，但未获取到图片链接'
          if (task.completed_at) {
            const t = new Date(task.updated_at).getTime() - new Date(task.created_at).getTime()
            assistantMsg.elapsed = (t / 1000).toFixed(1)
          }
        } catch (e) { console.error('[Poll] parse error:', e); assistantMsg.imageUrl = '' }
        assistantMsg.status = 'completed'
        generating.value = false
        scrollToBottom()
        loadHistory()
        return
      } else if (task.status === 'failed') {
        assistantMsg.status = 'failed'
        assistantMsg.errorMsg = task.error_msg || '生成失败'
        generating.value = false
        scrollToBottom()
        loadHistory()
        return
      }
      pollTimer = window.setTimeout(() => pollTaskStatus(taskId, assistantMsg), 2000)
    } else {
      console.warn('[Poll] non-zero code:', res.code, res.msg)
      pollTimer = window.setTimeout(() => pollTaskStatus(taskId, assistantMsg), 3000)
    }
  } catch (e) {
    console.error('[Poll] error:', e)
    pollTimer = window.setTimeout(() => pollTaskStatus(taskId, assistantMsg), 3000)
  }
}

function handleClear() {
  selectedCardIds.value = []
  userPrompt.value = ''
  selectedRatio.value = '3:4'
  imageCount.value = 1
  selectedQuality.value = '1k'
  customWidth.value = 1024
  customHeight.value = 1365
  currentTaskId.value = null
  messages.value = []
  activeTaskId.value = null
  historyInitialized = false
  loadHistory()
}
</script>

<style scoped>
/* ============ 整体布局 ============ */
.home-layout {
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
  background: #111113;
  overflow: hidden;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif;
}
.home-body {
  flex: 1;
  display: grid;
  grid-template-columns: 220px 1fr 380px;
  min-height: 0;
  overflow: hidden;
}

/* ============ 左侧：作品画廊 ============ */
.history-panel {
  border-right: 1px solid #222225;
  background: #111113;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  min-height: 0;
}
.history-header {
  padding: 14px 16px 10px;
  font-size: 13px;
  font-weight: 600;
  color: #ccc;
  border-bottom: 1px solid #222225;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.gallery-count {
  font-size: 11px;
  color: #a3e635;
  background: rgba(163,230,53,0.1);
  padding: 1px 8px;
  border-radius: 10px;
  font-weight: 500;
}
.gallery-grid {
  flex: 1;
  overflow-y: auto;
  padding: 8px;
  column-count: 2;
  column-gap: 6px;
}
.gallery-grid::-webkit-scrollbar { width: 3px; }
.gallery-grid::-webkit-scrollbar-thumb { background: #333; border-radius: 2px; }
.gallery-item {
  position: relative;
  border-radius: 8px;
  overflow: hidden;
  cursor: pointer;
  background: #1a1a1e;
  border: 2px solid transparent;
  transition: all 0.2s;
  break-inside: avoid;
  margin-bottom: 6px;
}
.gallery-item:hover { border-color: #3a3a3e; transform: scale(1.02); }
.gallery-item.active { border-color: #a3e635; }
.gallery-item img {
  width: 100%;
  height: auto;
  min-height: 80px;
  display: block;
  object-fit: cover;
  background: #27272a;
}
.gallery-item-overlay {
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
  background: linear-gradient(transparent, rgba(0,0,0,0.8));
  padding: 20px 6px 6px;
  opacity: 0;
  transition: opacity 0.2s;
}
.gallery-item:hover .gallery-item-overlay { opacity: 1; }
.gallery-delete-btn {
  position: absolute;
  top: 4px;
  right: 4px;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: rgba(0,0,0,0.6);
  border: none;
  color: #aaa;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  opacity: 0;
  transition: opacity 0.2s, background 0.2s;
  z-index: 2;
}
.gallery-item:hover .gallery-delete-btn { opacity: 1; }
.gallery-delete-btn:hover { background: rgba(220,38,38,0.8); color: #fff; }
.gallery-item-prompt {
  font-size: 10px;
  color: #ddd;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  line-height: 1.4;
}
.history-empty {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  font-size: 12px;
  color: #555;
}

/* ============ 中间：生成区 ============ */
.canvas-area {
  display: flex;
  flex-direction: column;
  background: #141416;
  overflow: hidden;
  min-height: 0;
}
.guest-prompt {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
}
.smiley-icon { margin-bottom: 16px; }
.guest-title { font-size: 14px; color: #999; }
.guest-desc { font-size: 13px; color: #777; }
.guest-action { font-size: 13px; color: #888; margin-top: 4px; }
.link-highlight { color: #a3e635; text-decoration: underline; cursor: pointer; font-weight: 500; }
.link-highlight:hover { color: #bef264; }

/* 生成展示区 */
.gen-display {
  flex: 1;
  overflow-y: auto;
  padding: 20px 28px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.gen-display::-webkit-scrollbar { width: 4px; }
.gen-display::-webkit-scrollbar-thumb { background: #333; border-radius: 2px; }

.chat-empty {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
}
.chat-empty-text { font-size: 15px; color: #666; font-weight: 500; }
.chat-empty-sub { font-size: 13px; color: #444; }

/* 聊天气泡 */
.chat-bubble { display: flex; flex-direction: column; }
.chat-bubble.user { align-items: flex-end; }
.chat-bubble.assistant { align-items: flex-start; }

.bubble-row { max-width: 80%; }
.bubble-row.user { display: flex; flex-direction: column; align-items: flex-end; gap: 4px; }
.bubble-row.ai { display: flex; flex-direction: column; align-items: flex-start; gap: 4px; }

.bubble-body {
  border-radius: 12px;
  padding: 12px 16px;
}
.bubble-body.user {
  background: rgba(163,230,53,0.1);
  border: 1px solid rgba(163,230,53,0.2);
}
.bubble-body.failed {
  background: rgba(248,113,113,0.08);
  border: 1px solid rgba(248,113,113,0.25);
}
.bubble-body.ai-success {
  background: #1a1a1e;
  border: 1px solid #27272a;
}

.bubble-prompt {
  font-size: 13px;
  color: #ccc;
  line-height: 1.6;
  margin: 0;
  word-break: break-word;
}
.bubble-tags { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 6px; }
.bubble-time {
  font-size: 10px;
  color: #444;
  padding: 0 4px;
}

/* 3:4 骨架占位 */
.skeleton-wrap {
  width: 240px;
  aspect-ratio: 3 / 4;
  border-radius: 12px;
  background: #1a1a1e;
  border: 1px solid #27272a;
  position: relative;
  overflow: hidden;
}
.skeleton-shimmer {
  position: absolute;
  inset: 0;
  background: linear-gradient(110deg, transparent 30%, rgba(163,230,53,0.04) 50%, transparent 70%);
  background-size: 300% 100%;
  animation: shimmer 2s ease-in-out infinite;
}
@keyframes shimmer {
  0% { background-position: 200% 0; }
  100% { background-position: -100% 0; }
}
.skeleton-overlay {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
}
.skeleton-spinner {
  width: 32px;
  height: 32px;
  border: 3px solid #27272a;
  border-top-color: #a3e635;
  border-radius: 50%;
  animation: spin 1s linear infinite;
}
.skeleton-text { font-size: 13px; color: #888; font-weight: 500; }
.skeleton-sub { font-size: 11px; color: #555; }

/* 失败气泡 */
.bubble-failed-row {
  display: flex;
  align-items: center;
  gap: 12px;
}
.bubble-failed-icon {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  background: rgba(248,113,113,0.15);
  color: #f87171;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 16px;
  flex-shrink: 0;
}
.bubble-failed-title { font-size: 13px; color: #f87171; font-weight: 500; }
.bubble-failed-reason { font-size: 12px; color: #888; margin-top: 3px; word-break: break-word; }

/* 成功气泡 */
.bubble-image-wrap {
  border-radius: 10px;
  overflow: hidden;
  max-width: 400px;
  cursor: pointer;
  position: relative;
}
.bubble-image-wrap:hover .image-zoom-hint { opacity: 1; }
.image-zoom-hint {
  position: absolute;
  top: 8px;
  right: 8px;
  width: 32px;
  height: 32px;
  border-radius: 8px;
  background: rgba(0,0,0,0.55);
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: center;
  opacity: 0;
  transition: opacity 0.2s;
  pointer-events: none;
}
.bubble-image {
  width: 100%;
  display: block;
  transition: transform 0.2s;
}
.bubble-image-wrap:hover .bubble-image { transform: scale(1.02); }

/* Lightbox */
.lightbox-overlay {
  position: fixed;
  inset: 0;
  z-index: 9999;
  background: rgba(0,0,0,0.85);
  display: flex;
  align-items: center;
  justify-content: center;
  animation: lightbox-in 0.2s ease;
}
@keyframes lightbox-in {
  from { opacity: 0; }
  to { opacity: 1; }
}
.lightbox-container {
  position: relative;
  max-width: 90vw;
  max-height: 90vh;
}
.lightbox-image {
  max-width: 90vw;
  max-height: 90vh;
  border-radius: 8px;
  object-fit: contain;
  display: block;
}
.lightbox-close {
  position: absolute;
  top: -36px;
  right: 0;
  background: rgba(255,255,255,0.1);
  border: none;
  color: #fff;
  width: 32px;
  height: 32px;
  border-radius: 50%;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: background 0.2s;
}
.lightbox-close:hover { background: rgba(255,255,255,0.25); }
.lightbox-actions {
  position: absolute;
  bottom: -45px;
  left: 50%;
  transform: translateX(-50%);
  display: flex;
  gap: 8px;
}
.lightbox-action-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 7px 16px;
  border-radius: 8px;
  border: 1px solid rgba(255,255,255,0.15);
  background: rgba(255,255,255,0.08);
  color: #ddd;
  font-size: 12px;
  cursor: pointer;
  transition: all 0.2s;
  white-space: nowrap;
}
.lightbox-action-btn:hover { background: rgba(163,230,53,0.15); border-color: #a3e635; color: #a3e635; }
.lightbox-action-btn.seg-btn { border-color: rgba(96,165,250,0.3); }
.lightbox-action-btn.seg-btn:hover { background: rgba(96,165,250,0.15); border-color: #60a5fa; color: #60a5fa; }

/* 分割工作台 */
.seg-workspace { padding: 0; }
.seg-type-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 16px;
}
.seg-type-btn {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 7px 14px;
  border-radius: 8px;
  border: 1px solid #2a2a2e;
  background: transparent;
  color: #999;
  font-size: 13px;
  cursor: pointer;
  transition: all 0.2s;
}
.seg-type-btn:hover { border-color: #60a5fa; color: #ccc; }
.seg-type-btn.active {
  border-color: #60a5fa;
  background: rgba(96,165,250,0.1);
  color: #60a5fa;
}
.seg-type-icon { font-size: 15px; }
.seg-content {
  display: flex;
  align-items: center;
  gap: 12px;
}
.seg-panel { flex: 1; min-width: 0; }
.seg-panel-label {
  font-size: 12px;
  color: #888;
  margin-bottom: 6px;
  text-align: center;
}
.seg-panel-img {
  border-radius: 10px;
  overflow: hidden;
  background: #1a1a1e;
  aspect-ratio: 1;
  display: flex;
  align-items: center;
  justify-content: center;
}
.seg-panel-img img { width: 100%; height: 100%; object-fit: contain; display: block; }
.seg-panel-img.result { background: repeating-conic-gradient(#222 0% 25%, #1a1a1e 0% 50%) 50%/16px 16px; }
.seg-arrow { flex-shrink: 0; display: flex; align-items: center; }
.seg-placeholder { color: #555; font-size: 13px; }
.seg-status-text { color: #ef4444; font-size: 13px; margin-right: auto; }
.bubble-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding-top: 8px;
}
.bubble-meta {
  font-size: 11px;
  color: #555;
}

/* 高亮动画 */
.chat-bubble.highlight .bubble-body,
.chat-bubble.highlight .skeleton-wrap,
.chat-bubble.highlight .bubble-image-wrap {
  outline: 2px solid #a3e635;
  outline-offset: 2px;
  transition: outline 0.3s;
}
@keyframes highlight-fade {
  0% { outline-color: #a3e635; }
  100% { outline-color: transparent; }
}

/* 结果操作按钮 */
.result-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}
.result-action-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 5px 12px;
  border-radius: 8px;
  border: 1px solid #2a2a2e;
  background: #1a1a1e;
  color: #999;
  font-size: 11px;
  cursor: pointer;
  transition: all 0.2s;
}
.result-action-btn:hover { border-color: #444; color: #ddd; }
.result-action-btn.primary {
  background: rgba(163,230,53,0.1);
  border-color: rgba(163,230,53,0.3);
  color: #a3e635;
}
.result-action-btn.primary:hover {
  background: rgba(163,230,53,0.18);
  border-color: #a3e635;
}
.result-action-btn.danger {
  color: #888;
}
.result-action-btn.danger:hover {
  background: rgba(220,38,38,0.12);
  border-color: rgba(220,38,38,0.4);
  color: #ef4444;
}

.tag {
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 10px;
  background: rgba(163,230,53,0.1);
  color: #a3e635;
  border: 1px solid rgba(163,230,53,0.2);
}

@keyframes spin { to { transform: rotate(360deg); } }

.empty-icon-wrap {
  margin-bottom: 12px;
  opacity: 0.6;
}

/* ============ 右侧控制面板 ============ */
.control-panel {
  border-left: 1px solid #222225;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: #19191c;
  position: relative;
  min-height: 0;
}
.tutorial-float {
  position: absolute;
  left: -28px;
  top: 50%;
  transform: translateY(-50%);
  background: #2a2a2e;
  color: #999;
  font-size: 11px;
  padding: 10px 6px;
  border-radius: 6px 0 0 6px;
  cursor: pointer;
  line-height: 1.4;
  text-align: center;
  letter-spacing: 1px;
  z-index: 10;
  transition: background 0.2s;
}
.tutorial-float:hover { background: #333; color: #ccc; }

.panel-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 16px 10px;
  border-bottom: 1px solid #222225;
  flex-shrink: 0;
}
.panel-tabs { display: flex; gap: 0; }
.panel-tab {
  padding: 6px 14px;
  border: none;
  cursor: pointer;
  font-size: 13px;
  font-weight: 500;
  transition: all 0.2s;
  background: transparent;
  color: #666;
  border-bottom: 2px solid transparent;
}
.panel-tab:hover:not(.active) { color: #aaa; }
.panel-tab.active { color: #a3e635; border-bottom-color: #a3e635; }

.quick-edit-link {
  font-size: 12px;
  color: #a3e635;
  text-decoration: none;
  white-space: nowrap;
}
.quick-edit-link:hover { text-decoration: underline; }

.panel-scroll {
  flex: 1;
  overflow-y: auto;
  padding: 14px 16px 10px;
  display: flex;
  flex-direction: column;
  gap: 14px;
  min-height: 0;
}
.panel-scroll::-webkit-scrollbar { width: 4px; }
.panel-scroll::-webkit-scrollbar-thumb { background: #333; border-radius: 2px; }

.panel-bottom {
  flex-shrink: 0;
  padding: 12px 16px 16px;
  border-top: 1px solid #222225;
  background: #19191c;
}

.section-block { display: flex; flex-direction: column; gap: 10px; }
.section-header { display: flex; align-items: center; gap: 6px; font-size: 13px; }
.section-num { color: #a3e635; font-weight: 600; }
.section-title { color: #ddd; font-weight: 600; }
.section-info { color: #555; font-size: 14px; cursor: help; }
.section-link { margin-left: auto; font-size: 12px; color: #666; text-decoration: none; }
.section-link:hover { color: #aaa; }

/* 风格卡片网格 */
.brand-cards-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 8px; }
.loading-state, .empty-state {
  grid-column: 1 / -1;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 30px 0;
  color: #555;
}
.empty-state p { font-size: 12px; }
.brand-card {
  cursor: pointer;
  border-radius: 8px;
  overflow: hidden;
  border: 2px solid transparent;
  transition: all 0.2s;
  background: #222225;
}
.brand-card:hover { border-color: #3a3a3e; transform: translateY(-1px); }
.brand-card.selected { border-color: #a3e635; }
.brand-card-cover {
  position: relative;
  width: 100%;
  aspect-ratio: 1;
  background: #1a1a1e;
  overflow: hidden;
}
.brand-card-cover img { width: 100%; height: 100%; object-fit: cover; display: block; }
.card-check {
  position: absolute;
  top: 4px;
  right: 4px;
  width: 18px;
  height: 18px;
  border-radius: 50%;
  background: #a3e635;
  display: flex;
  align-items: center;
  justify-content: center;
}
.brand-card-name {
  padding: 4px;
  font-size: 10px;
  color: #999;
  text-align: center;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  line-height: 1.4;
}
.view-all-btn {
  width: 100%;
  padding: 8px 0;
  border: 1px solid #2a2a2e;
  border-radius: 8px;
  background: transparent;
  color: #999;
  font-size: 12px;
  cursor: pointer;
  transition: all 0.2s;
  text-align: center;
}
.view-all-btn:hover { border-color: #a3e635; color: #a3e635; }

/* ③ 图片尺寸 */
.ratio-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 8px;
}
.ratio-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 8px 0;
  border-radius: 8px;
  border: 1px solid #2a2a2e;
  background: transparent;
  cursor: pointer;
  transition: all 0.2s;
  color: #888;
  font-size: 12px;
}
.ratio-btn:hover { border-color: #a3e635; color: #aaa; }
.ratio-btn.active {
  border-color: #a3e635;
  background: rgba(163,230,53,0.08);
  color: #a3e635;
}
.ratio-icon {
  border: 1.5px solid currentColor;
  border-radius: 2px;
  flex-shrink: 0;
}

/* 自定义尺寸 */
.custom-size-row {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  margin-top: 6px;
}
.custom-size-label {
  font-size: 12px;
  color: #666;
  cursor: pointer;
  padding: 4px 10px;
  border-radius: 6px;
  border: 1px solid #2a2a2e;
  transition: all 0.2s;
  white-space: nowrap;
}
.custom-size-label:hover { border-color: #a3e635; color: #aaa; }
.custom-size-label.active {
  border-color: #a3e635;
  background: rgba(163,230,53,0.08);
  color: #a3e635;
}
.custom-size-input {
  width: 70px;
  padding: 5px 8px;
  border: 1px solid #2a2a2e;
  border-radius: 6px;
  background: #1e1e22;
  color: #ddd;
  font-size: 12px;
  text-align: center;
  outline: none;
  transition: border-color 0.2s;
  -moz-appearance: textfield;
}
.custom-size-input::-webkit-inner-spin-button,
.custom-size-input::-webkit-outer-spin-button { -webkit-appearance: none; margin: 0; }
.custom-size-input:focus { border-color: #a3e635; }
.custom-size-input::placeholder { color: #555; }
.size-separator { color: #555; font-size: 13px; }
.size-unit { color: #555; font-size: 11px; }

/* ④ 出图张数 */
.count-grid {
  display: flex;
  gap: 8px;
}
.count-btn {
  flex: 1;
  padding: 8px 0;
  border-radius: 8px;
  border: 1px solid #2a2a2e;
  background: transparent;
  cursor: pointer;
  font-size: 13px;
  color: #888;
  transition: all 0.2s;
  text-align: center;
}
.count-btn:hover { border-color: #a3e635; color: #aaa; }
.count-btn.active {
  border-color: #a3e635;
  background: rgba(163,230,53,0.08);
  color: #a3e635;
}

/* ⑤ 图片清晰度 */
.quality-grid {
  display: flex;
  gap: 8px;
}
.quality-btn {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  padding: 8px 0;
  border-radius: 8px;
  border: 1px solid #2a2a2e;
  background: transparent;
  cursor: pointer;
  transition: all 0.2s;
}
.quality-btn:hover { border-color: #a3e635; }
.quality-btn.active {
  border-color: #a3e635;
  background: rgba(163,230,53,0.08);
}
.quality-label {
  font-size: 13px;
  font-weight: 600;
  color: #888;
}
.quality-btn.active .quality-label { color: #a3e635; }
.quality-btn:hover .quality-label { color: #aaa; }
.quality-desc {
  font-size: 10px;
  color: #555;
}

/* ⑤ 变化幅度 */
.variation-grid {
  display: flex;
  gap: 8px;
}
.variation-btn {
  flex: 1;
  padding: 8px 0;
  border-radius: 8px;
  border: 1px solid #2a2a2e;
  background: transparent;
  color: #888;
  font-size: 13px;
  cursor: pointer;
  transition: all 0.2s;
}
.variation-btn:hover { border-color: #a3e635; color: #aaa; }
.variation-btn.active {
  border-color: #a3e635;
  background: rgba(163,230,53,0.08);
  color: #a3e635;
}

/* 按钮组 */
.button-group { display: flex; gap: 10px; }
.btn-generate {
  flex: 1;
  padding: 12px 0;
  border: none;
  border-radius: 8px;
  background: #a3e635;
  color: #111;
  font-size: 14px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.2s;
}
.btn-generate:hover:not(:disabled) { background: #bef264; }
.btn-generate:disabled { opacity: 0.4; cursor: not-allowed; }
.btn-clear {
  flex: 1;
  padding: 12px 0;
  border: 1px solid #2a2a2e;
  border-radius: 8px;
  background: #222225;
  color: #999;
  font-size: 14px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s;
}
.btn-clear:hover:not(:disabled) { border-color: #444; color: #ccc; }
.btn-clear:disabled { opacity: 0.4; cursor: not-allowed; }

/* 弹窗 */
.all-cards-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 10px; max-height: 60vh; overflow-y: auto; }
.modal-footer { display: flex; align-items: center; justify-content: space-between; }
.selected-count { font-size: 13px; color: #71717a; }

/* Naive UI 覆盖 */
:deep(.n-input) {
  --n-border: 1px solid #2a2a2e;
  --n-border-hover: 1px solid #a3e635;
  --n-border-focus: 1px solid #a3e635;
  --n-color: #1e1e22;
  --n-color-focus: #1e1e22;
  --n-text-color: #ddd;
  --n-placeholder-color: #555;
  --n-caret-color: #a3e635;
  border-radius: 8px;
}
:deep(.n-input .n-input__textarea-el) { font-size: 13px; line-height: 1.7; }
:deep(.n-button--primary-type) {
  --n-color: #a3e635;
  --n-color-hover: #bef264;
  --n-color-pressed: #84cc16;
  --n-color-focus: #a3e635;
  --n-text-color: #111;
  border-radius: 8px;
}
:deep(.n-card) { --n-color: #1a1a1e; --n-border-color: #27272a; border-radius: 12px; }
:deep(.n-card-header__main) { color: #e4e4e7; }
</style>

<style>
.delete-popover {
  position: fixed;
  transform: translate(-50%, -100%) translateY(-8px);
  background: #1e1e22;
  border: 1px solid #333;
  border-radius: 10px;
  padding: 10px 14px;
  z-index: 9999;
  box-shadow: 0 4px 20px rgba(0,0,0,0.5);
  animation: dp-fade-in 0.15s ease;
}
@keyframes dp-fade-in {
  from { opacity: 0; transform: translate(-50%, -100%) translateY(-2px); }
  to { opacity: 1; transform: translate(-50%, -100%) translateY(-8px); }
}
.delete-popover-text {
  font-size: 13px;
  color: #ccc;
  margin-bottom: 8px;
  white-space: nowrap;
}
.delete-popover-actions {
  display: flex;
  gap: 6px;
  justify-content: flex-end;
}
.dp-btn {
  padding: 3px 12px;
  border-radius: 6px;
  border: 1px solid #333;
  font-size: 12px;
  cursor: pointer;
  transition: all 0.15s;
}
.dp-cancel {
  background: #27272a;
  color: #aaa;
}
.dp-cancel:hover { background: #333; color: #ddd; }
.dp-confirm {
  background: rgba(220,38,38,0.15);
  border-color: rgba(220,38,38,0.4);
  color: #ef4444;
}
.dp-confirm:hover { background: rgba(220,38,38,0.3); }
</style>
