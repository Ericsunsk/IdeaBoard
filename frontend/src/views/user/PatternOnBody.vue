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
          <div v-for="ht in galleryTasks" :key="ht.task_id" class="gallery-item" :class="{ active: activeTaskId === ht.task_id }" @click="viewHistoryTask(ht)">
            <img :src="getTaskImageUrl(ht)" alt="" loading="lazy" @error="handleImgError" />
            <div class="gallery-item-overlay"><span class="gallery-item-prompt">{{ stripPrefix(ht.prompt) }}</span></div>
            <button class="gallery-delete-btn" @click.stop="showDeleteConfirm($event, ht.task_id)" title="删除">
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
            </button>
          </div>
        </div>
        <div class="history-empty" v-else>
          <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="#333" stroke-width="1.5"><rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="8.5" cy="8.5" r="1.5"/><path d="m21 15-5-5L5 21"/></svg>
          <p>还没有作品</p>
          <p style="font-size:11px;color:#444;">生成图片后将在这里展示</p>
        </div>
      </aside>

      <!-- ====== 中间：生成区 ====== -->
      <main class="canvas-area">
        <div v-if="!userStore.isLoggedIn" class="guest-prompt">
          <div class="smiley-icon">
            <svg width="120" height="120" viewBox="0 0 120 120" fill="none"><circle cx="60" cy="60" r="56" stroke="#a3e635" stroke-width="4" fill="none"/><circle cx="44" cy="48" r="5" fill="#a3e635"/><circle cx="76" cy="48" r="5" fill="#a3e635"/><path d="M38 72 C38 72, 44 90, 60 90 C76 90, 82 72, 82 72" stroke="#a3e635" stroke-width="4" stroke-linecap="round" fill="none"/></svg>
          </div>
          <p class="guest-title">温馨提示：</p>
          <p class="guest-desc">您当前是游客身份，暂无工具相关权限。</p>
          <p class="guest-action">请先 <router-link to="/user/register" class="link-highlight">注册</router-link> 或 <router-link to="/user/login" class="link-highlight">登录</router-link></p>
        </div>
        <div v-else class="gen-display" ref="chatMessagesRef">
          <div v-if="messages.length === 0" class="chat-empty">
            <div class="empty-icon-wrap">
              <svg width="64" height="64" viewBox="0 0 24 24" fill="none" stroke="#a3e635" stroke-width="1.2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 2L2 7l10 5 10-5-10-5z"/><path d="M2 17l10 5 10-5"/><path d="M2 12l10 5 10-5"/></svg>
            </div>
            <p class="chat-empty-text">图案上身</p>
            <p class="chat-empty-sub">上传衣服图片和图案，选择全身或局部模式后生成</p>
          </div>
          <div v-for="msg in messages" :key="msg.id" :id="msg.taskId ? 'task-' + msg.taskId : undefined" class="chat-bubble" :class="msg.role">
            <div v-if="msg.role === 'user'" class="bubble-row user">
              <div class="bubble-body user">
                <p class="bubble-prompt">{{ msg.prompt }}</p>
                <div class="bubble-ref-images" v-if="msg.refImageUrls && msg.refImageUrls.length">
                  <img v-for="(url, i) in msg.refImageUrls" :key="i" :src="url" alt="参考图" class="bubble-ref-img" />
                </div>
              </div>
              <span class="bubble-time" v-if="msg.time">{{ msg.time }}</span>
            </div>
            <div v-else class="bubble-row ai">
              <div v-if="msg.status === 'pending'" class="skeleton-wrap"><div class="skeleton-shimmer"></div><div class="skeleton-overlay"><div class="skeleton-spinner"></div><p class="skeleton-text">AI 正在生成中...</p><p class="skeleton-sub">请耐心等待，通常需要 10-30 秒</p></div></div>
              <div v-else-if="msg.status === 'failed'" class="bubble-body failed"><div class="bubble-failed-row"><div class="bubble-failed-icon">✕</div><div class="bubble-failed-info"><div class="bubble-failed-title">生成失败</div><div class="bubble-failed-reason">{{ msg.errorMsg }}</div></div></div></div>
              <template v-else-if="msg.status === 'completed'">
                <div class="bubble-body ai-success">
                  <div v-if="msg.imageUrl" class="bubble-image-wrap" @click="lightboxUrl = msg.imageUrl!">
                    <img :src="msg.imageUrl" :alt="msg.prompt" class="bubble-image" @error="handleImgError" />
                    <div class="image-zoom-hint"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/><line x1="11" y1="8" x2="11" y2="14"/><line x1="8" y1="11" x2="14" y2="11"/></svg></div>
                  </div>
                  <div class="bubble-footer">
                    <span class="bubble-meta" v-if="msg.elapsed">耗时 {{ msg.elapsed }}s</span>
                    <div class="result-actions">
                      <button class="result-action-btn" @click="downloadImage(msg.imageUrl)"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>下载</button>
                      <button class="result-action-btn primary" @click="handleGenerate"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/></svg>再次生成</button>
                      <button class="result-action-btn danger" @click="showDeleteConfirm($event, msg.taskId)"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>删除</button>
                    </div>
                  </div>
                </div>
              </template>
            </div>
          </div>
        </div>
      </main>

      <!-- ====== 右侧：创作面板 ====== -->
      <aside class="control-panel">
        <div class="panel-top">
          <div class="panel-tabs"><button class="panel-tab active">图案上身</button></div>
        </div>
        <div class="panel-scroll">

          <!-- ① 上传衣服图片 -->
          <div class="section">
            <div class="section-header-row"><span class="section-num">①</span><span class="section-label">上传衣服图片</span></div>
            <ImageUploader v-model="garmentImages" :gallery-groups="allGalleryGroups" :max="1" :max-size-m-b="20" />
          </div>

          <!-- ② 上传图案图片 -->
          <div class="section">
            <div class="section-header-row"><span class="section-num">②</span><span class="section-label">上传图案图片</span></div>
            <ImageUploader v-model="patternImages" :gallery-groups="allGalleryGroups" :max="1" :max-size-m-b="20" />
          </div>

          <!-- ③ 上身模式 -->
          <div class="section" v-if="garmentImageUrl && patternImageUrl">
            <div class="section-header-row"><span class="section-num">③</span><span class="section-label">上身模式</span></div>
            <div class="mode-selector">
              <button class="mode-btn" :class="{ active: patternMode === 'full' }" @click="patternMode = 'full'">
                <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="3" width="18" height="18" rx="2"/><path d="M3 9h18"/><path d="M9 21V9"/></svg>
                全身
              </button>
              <button class="mode-btn" :class="{ active: patternMode === 'partial' }" @click="patternMode = 'partial'">
                <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 19l7-7 3 3-7 7-3-3z"/><path d="M18 13l-1.5-7.5L2 2l3.5 14.5L13 18l5-5z"/></svg>
                局部
              </button>
            </div>
          </div>

          <!-- 全身模式：品类选择 -->
          <div class="section" v-if="patternMode === 'full' && garmentImageUrl && patternImageUrl">
            <div class="section-header-row"><span class="section-num">④</span><span class="section-label">选择品类（可选）</span></div>
            <div class="category-selector" @click="showCategoryPicker = true">
              <span v-if="selectedCategory" class="category-selected">{{ selectedCategory }}</span>
              <span v-else class="category-placeholder">点击选择品类</span>
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="6 9 12 15 18 9"/></svg>
            </div>
          </div>

          <!-- 局部模式：涂抹区域 -->
          <div class="section" v-if="patternMode === 'partial' && garmentImageUrl && patternImageUrl">
            <div class="section-header-row"><span class="section-num">④</span><span class="section-label">涂抹图案放置区域</span></div>
            <div class="paint-preview" @click="openPaintModal">
              <img :src="garmentImageUrl" class="paint-preview-img" />
              <canvas ref="previewCanvasRef" class="paint-preview-overlay"></canvas>
              <div class="paint-preview-mask">
                <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 19l7-7 3 3-7 7-3-3z"/><path d="M18 13l-1.5-7.5L2 2l3.5 14.5L13 18l5-5z"/></svg>
                <span>{{ hasPainted ? '重新涂抹' : '开始涂抹' }}</span>
              </div>
            </div>
            <div v-if="hasPainted" class="paint-done-tip">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="#a3e635" stroke-width="2.5"><polyline points="20 6 9 17 4 12"/></svg>
              <span>涂抹已完成</span>
            </div>
          </div>

          <!-- ⑤ 图案参数 -->
          <div class="section" v-if="patternImageUrl && garmentImageUrl">
            <div class="section-header-row">
              <span class="section-num">⑤</span><span class="section-label">图案参数</span>
            </div>

            <div class="fabric-preview-wrap">
              <div class="fabric-preview-box">
                <img :src="patternImageUrl" class="fabric-preview-img" :style="fabricPreviewStyle" />
              </div>
              <span class="fabric-preview-hint">实时预览</span>
            </div>

            <div class="param-group">
              <div class="param-label">替换方式</div>
              <div class="tile-mode-selector">
                <button class="tile-mode-btn" :class="{ active: tileMode === 'seamless' }" @click="tileMode = 'seamless'">
                  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/></svg>
                  四方连续
                </button>
                <button class="tile-mode-btn" :class="{ active: tileMode === 'single' }" @click="tileMode = 'single'">
                  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="3" width="18" height="18" rx="2"/></svg>
                  非四方连续
                </button>
              </div>
            </div>

            <div class="param-group">
              <div class="param-label-row"><span class="param-label">旋转角度</span><span class="param-value">{{ fabricRotation }}°</span></div>
              <input type="range" class="param-slider" v-model.number="fabricRotation" min="0" max="360" step="1" />
              <div class="param-ticks"><span @click="fabricRotation=0">0°</span><span @click="fabricRotation=90">90°</span><span @click="fabricRotation=180">180°</span><span @click="fabricRotation=270">270°</span><span @click="fabricRotation=360">360°</span></div>
            </div>
            <div class="param-group">
              <div class="param-label-row"><span class="param-label">图案大小</span><span class="param-value">{{ fabricScale }}%</span></div>
              <input type="range" class="param-slider" v-model.number="fabricScale" min="10" max="300" step="5" />
              <div class="param-ticks"><span @click="fabricScale=50">50%</span><span @click="fabricScale=100">100%</span><span @click="fabricScale=150">150%</span><span @click="fabricScale=200">200%</span></div>
            </div>
            <div class="param-group">
              <div class="param-label-row"><span class="param-label">透明度</span><span class="param-value">{{ fabricOpacity }}%</span></div>
              <input type="range" class="param-slider" v-model.number="fabricOpacity" min="0" max="100" step="5" />
              <div class="param-ticks"><span @click="fabricOpacity=0">0%</span><span @click="fabricOpacity=25">25%</span><span @click="fabricOpacity=50">50%</span><span @click="fabricOpacity=75">75%</span><span @click="fabricOpacity=100">100%</span></div>
            </div>
            <button class="reset-params-btn" @click="resetParams">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="1 4 1 10 7 10"/><path d="M3.51 15a9 9 0 1 0 2.13-9.36L1 10"/></svg>
              重置参数
            </button>
          </div>

          <!-- 状态提示 -->
          <div class="section status-section">
            <div v-if="canGenerate" class="status-ready">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#a3e635" stroke-width="2"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>
              <span>已就绪，可以开始生成</span>
            </div>
            <div v-else class="status-waiting">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#71717a" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>
              <span v-if="!garmentImageUrl || !patternImageUrl">请上传衣服图片和图案图片</span>
              <span v-else-if="patternMode === 'partial' && !hasPainted">请涂抹图案放置区域</span>
              <span v-else>请完成参数设置</span>
            </div>
          </div>
        </div>

        <div class="panel-bottom">
          <div class="button-group">
            <button class="btn-generate" :disabled="!canGenerate || generating || uploading" @click="handleGenerate">
              {{ uploading ? '上传涂抹图...' : generating ? '生成中...' : '生成图片' }}
            </button>
            <button class="btn-clear" :disabled="generating || uploading" @click="handleClear">清空</button>
          </div>
        </div>
      </aside>
    </div>

    <!-- 删除确认 -->
    <div v-if="deleteConfirm.show" class="delete-popover" :style="{ left: deleteConfirm.x+'px', top: deleteConfirm.y+'px' }">
      <p>确认删除？</p>
      <div class="delete-popover-btns"><button @click="deleteConfirm.show=false">取消</button><button class="confirm" @click="confirmDelete">删除</button></div>
    </div>

    <!-- 品类选择弹窗 -->
    <div v-if="showCategoryPicker" class="category-modal-overlay" @click.self="showCategoryPicker=false">
      <div class="category-modal">
        <div class="category-modal-header">
          <span class="category-modal-title">选择品类</span>
          <button class="category-modal-close" @click="showCategoryPicker=false"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg></button>
        </div>
        <div class="category-modal-body">
          <div v-for="group in categoryGroups" :key="group.label" class="category-group">
            <div class="category-group-label">{{ group.label }}</div>
            <div class="category-group-items">
              <button v-for="item in group.items" :key="item" class="category-item" :class="{ active: selectedCategory === item }" @click="pickCategory(item)">{{ item }}</button>
            </div>
          </div>
        </div>
        <div class="category-modal-footer"><button class="category-clear-btn" @click="selectedCategory=''; showCategoryPicker=false">清除选择</button></div>
      </div>
    </div>

    <!-- ====== 涂抹工作台弹窗 ====== -->
    <div v-if="showPaintModal" class="paint-modal-overlay">
      <div class="paint-modal">
        <div class="paint-modal-header">
          <span class="paint-modal-title">涂抹工作台</span>
          <span class="paint-modal-hint">在图片上涂抹图案放置的区域</span>
          <div class="header-actions">
            <button class="tool-btn-text cancel" @click="cancelPaintModal">取消</button>
            <button class="tool-btn-text confirm" @click="confirmPaintModal">完成涂抹</button>
          </div>
        </div>
        <div class="paint-modal-body" ref="viewportRef"
          :class="{ 'cursor-grab': paintTool==='hand' && !panning, 'cursor-grabbing': panning }"
          @mousedown="onViewportMouseDown" @mousemove="onViewportMouseMove" @mouseup="onViewportMouseUp"
          @mouseleave="onViewportMouseLeave" @wheel.prevent="onViewportWheel"
          @touchstart.prevent="onViewportTouchStart" @touchmove.prevent="onViewportTouchMove" @touchend.prevent="onViewportTouchEnd">
          <div class="paint-canvas-wrap" ref="canvasWrapRef" :style="canvasWrapStyle">
            <img ref="bgImageRef" :src="garmentImageUrl" class="paint-bg-image" @load="initCanvas" crossorigin="anonymous" />
            <canvas ref="paintCanvasRef" class="paint-canvas"></canvas>
            <div v-if="!hasPainted && !paintingNow" class="paint-hint">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 19l7-7 3 3-7 7-3-3z"/><path d="M18 13l-1.5-7.5L2 2l3.5 14.5L13 18l5-5z"/></svg>
              <span>在图片上涂抹图案放置的区域</span>
            </div>
          </div>
          <div v-if="cursorVisible && (paintTool==='brush'||paintTool==='eraser')" class="brush-cursor" :style="brushCursorStyle"></div>
        </div>
        <div class="paint-modal-footer">
          <div class="paint-toolbar">
            <div class="toolbar-group">
              <button class="tool-btn" :class="{active:paintTool==='brush'}" @click="paintTool='brush'" title="画笔 (B)"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 19l7-7 3 3-7 7-3-3z"/><path d="M18 13l-1.5-7.5L2 2l3.5 14.5L13 18l5-5z"/></svg></button>
              <button class="tool-btn" :class="{active:paintTool==='eraser'}" @click="paintTool='eraser'" title="橡皮擦 (E)"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="m7 21-4.3-4.3c-1-1-1-2.5 0-3.4l9.6-9.6c1-1 2.5-1 3.4 0l5.6 5.6c1 1 1 2.5 0 3.4L13 21"/><path d="M22 21H7"/><path d="m5 11 9 9"/></svg></button>
              <button class="tool-btn" :class="{active:paintTool==='hand'}" @click="paintTool='hand'" title="抓手 (H)"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 11V6a2 2 0 0 0-4 0v1"/><path d="M14 10V4a2 2 0 0 0-4 0v2"/><path d="M10 10.5V6a2 2 0 0 0-4 0v8"/><path d="M18 8a2 2 0 0 1 4 0v6a8 8 0 0 1-8 8h-2c-2.8 0-4.5-.86-5.99-2.34l-3.6-3.6a2 2 0 0 1 2.83-2.82L7 15"/></svg></button>
              <div class="toolbar-divider"></div>
              <div class="brush-size-wrap"><span class="brush-size-label">{{ brushSize }}px</span><input type="range" class="brush-size-slider" v-model.number="brushSize" min="5" max="80" step="1" /></div>
            </div>
            <div class="toolbar-group">
              <button class="tool-btn" @click="undoPaint" :disabled="paintHistory.length===0" title="撤销"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="1 4 1 10 7 10"/><path d="M3.51 15a9 9 0 1 0 2.13-9.36L1 10"/></svg></button>
              <button class="tool-btn" @click="redoPaint" :disabled="redoStack.length===0" title="重做"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/></svg></button>
              <button class="tool-btn" @click="clearPaint" title="重置"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg></button>
            </div>
            <div class="toolbar-group">
              <button class="tool-btn" @click="zoomOut" :disabled="zoom<=0.25"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/><line x1="8" y1="11" x2="14" y2="11"/></svg></button>
              <span class="zoom-label">{{ Math.round(zoom*100) }}%</span>
              <button class="tool-btn" @click="zoomIn" :disabled="zoom>=5"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/><line x1="11" y1="8" x2="11" y2="14"/><line x1="8" y1="11" x2="14" y2="11"/></svg></button>
              <button class="tool-btn" @click="zoomReset"><span style="font-size:11px;font-weight:700;">1:1</span></button>
              <button class="tool-btn" @click="zoomFit"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M8 3H5a2 2 0 0 0-2 2v3"/><path d="M21 8V5a2 2 0 0 0-2-2h-3"/><path d="M3 16v3a2 2 0 0 0 2 2h3"/><path d="M16 21h3a2 2 0 0 0 2-2v-3"/></svg></button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 图片放大 -->
    <div v-if="lightboxUrl" class="lightbox-overlay" @click.self="lightboxUrl=''">
      <div class="lightbox-container">
        <img :src="lightboxUrl" class="lightbox-image" />
        <button class="lightbox-close" @click="lightboxUrl=''"><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg></button>
        <div class="lightbox-actions"><button class="lightbox-action-btn" @click="downloadImage(lightboxUrl)"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>下载原图</button></div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, nextTick, onUnmounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useMessage } from 'naive-ui'
import { useUserStore } from '../../stores/user'
import { taskApi, uploadApi } from '../../api'
import ImageUploader from '../../components/ImageUploader.vue'
import { buildGalleryGroups } from '../../utils/galleryGroups'
import { downloadImageByProxy, getTaskImageUrl as extractTaskImageUrl, stripFeaturePrefix } from '../../utils/task-history'
import type { Task } from '../../api'

const FEATURE_SLUG = 'pattern-on-body'
const FEATURE_TITLE = '图案上身'
const FEATURE_PREFIX = `[${FEATURE_SLUG}] `

const router = useRouter()
const message = useMessage()
const userStore = useUserStore()

function stripPrefix(str?: string) {
  return stripFeaturePrefix(str)
}

// ============ 画廊 ============
const historyTasks = ref<Task[]>([])
const activeTaskId = ref<string | null>(null)
const galleryTasks = computed(() => historyTasks.value.filter(t => { if (t.status !== 'completed') return false; if (!t.prompt || !t.prompt.startsWith(FEATURE_PREFIX)) return false; try { return !!JSON.parse(t.result || '{}').image_url } catch { return false } }))
function getTaskImageUrl(task: Task): string {
  return extractTaskImageUrl(task)
}

async function loadHistory() { try { const res: any = await taskApi.list({ page: 1, page_size: 200 }); if (res.code === 0) { historyTasks.value = res.data.list || []; if (messages.value.length === 0) rebuildMessagesFromHistory() } } catch {} }

function rebuildMessagesFromHistory() {
  const tasks = historyTasks.value.filter(t => t.prompt && t.prompt.startsWith(FEATURE_PREFIX)).sort((a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime()).slice(-20)
  for (const task of tasks) {
    const imgUrl = getTaskImageUrl(task); const timeStr = task.created_at ? new Date(task.created_at).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' }) : ''
    messages.value.push({ id: 'hist_user_' + task.task_id, role: 'user', prompt: stripPrefix(task.prompt) || FEATURE_TITLE, time: timeStr })
    if (task.status === 'completed' && imgUrl) { let elapsed: string | undefined; if (task.completed_at && task.created_at) { const t = new Date(task.updated_at).getTime() - new Date(task.created_at).getTime(); if (t > 0) elapsed = (t / 1000).toFixed(1) }; messages.value.push({ id: 'hist_ai_' + task.task_id, role: 'assistant', prompt: task.prompt || '', status: 'completed', imageUrl: imgUrl, taskId: task.task_id, elapsed }) }
    else if (task.status === 'failed') { messages.value.push({ id: 'hist_ai_' + task.task_id, role: 'assistant', prompt: task.prompt || '', status: 'failed', errorMsg: task.error_msg || '生成失败', taskId: task.task_id }) }
  }
  if (messages.value.length > 0) scrollToBottom()
}

function viewHistoryTask(task: Task) {
  activeTaskId.value = task.task_id; const imgUrl = getTaskImageUrl(task)
  if (imgUrl) { const existing = messages.value.find(m => m.taskId === task.task_id); if (!existing) { messages.value.push({ id: 'hist_user_' + task.task_id, role: 'user', prompt: stripPrefix(task.prompt) || FEATURE_TITLE }); messages.value.push({ id: 'hist_ai_' + task.task_id, role: 'assistant', prompt: task.prompt || '', status: 'completed', imageUrl: imgUrl, taskId: task.task_id }); scrollToBottom() } else { const el = document.getElementById('task-' + task.task_id); if (el) { el.scrollIntoView({ behavior: 'smooth', block: 'center' }); el.classList.add('highlight'); setTimeout(() => el.classList.remove('highlight'), 2000) } } }
}

// ============ 删除 ============
const deleteConfirm = ref<{ show: boolean; x: number; y: number; taskId: string }>({ show: false, x: 0, y: 0, taskId: '' })
function showDeleteConfirm(e: MouseEvent, taskId?: string) { if (!taskId) return; deleteConfirm.value = { show: true, x: e.clientX, y: e.clientY, taskId }; const close = (ev: MouseEvent) => { const el = document.querySelector('.delete-popover'); if (el && !el.contains(ev.target as Node)) { deleteConfirm.value.show = false; document.removeEventListener('mousedown', close) } }; setTimeout(() => document.addEventListener('mousedown', close), 0) }
async function confirmDelete() { const taskId = deleteConfirm.value.taskId; deleteConfirm.value.show = false; if (!taskId) return; try { const res: any = await taskApi.delete(taskId); if (res.code === 0) { historyTasks.value = historyTasks.value.filter(t => t.task_id !== taskId); messages.value = messages.value.filter(m => m.id !== 'hist_user_' + taskId && m.id !== 'hist_ai_' + taskId && m.taskId !== taskId); if (activeTaskId.value === taskId) activeTaskId.value = null } } catch {} }

// ============ 双图上传 ============
const garmentImageUrl = ref('')
const patternImageUrl = ref('')
const garmentImages = computed({ get: () => garmentImageUrl.value ? [garmentImageUrl.value] : [], set: (val: string[]) => { garmentImageUrl.value = val.length > 0 ? val[0] : '' } })
const patternImages = computed({ get: () => patternImageUrl.value ? [patternImageUrl.value] : [], set: (val: string[]) => { patternImageUrl.value = val.length > 0 ? val[0] : '' } })
const allGalleryGroups = computed(() => buildGalleryGroups(historyTasks.value))

// ============ 模式 ============
const patternMode = ref<'full' | 'partial'>('full')

// ============ 品类选择（全身模式） ============
const showCategoryPicker = ref(false)
const selectedCategory = ref('')
const categoryGroups = [
  { label: '上装', items: ['T恤', '衬衫', '针织衫', '毛衣', '卫衣', '马甲', '背心', 'Polo衫', '打底衫', '雪纺衫', '蕾丝衫', '短上衣'] },
  { label: '外套', items: ['西装外套', '风衣', '大衣', '羽绒服', '棉服', '皮草', '皮衣', '牛仔外套', '夹克', '斗篷', '披肩', '短外套'] },
  { label: '裙装', items: ['连衣裙', '半身裙', 'A字裙', '百褶裙', '包臀裙', '鱼尾裙', '蓬蓬裙', '蛋糕裙', '旗袍', '吊带裙', '衬衫裙', '针织裙'] },
  { label: '裤装', items: ['牛仔裤', '休闲裤', '阔腿裤', '西裤', '铅笔裤', '喇叭裤', '工装裤', '运动裤', '打底裤', '短裤', '连体裤', '背带裤'] },
  { label: '套装 / 其他', items: ['套装', '礼服', '婚纱', '旗袍套装', '运动套装', '家居服', '睡衣', '泳装', '汉服', '民族服饰', '制服', 'Lolita'] },
]
function pickCategory(item: string) { selectedCategory.value = item; showCategoryPicker.value = false }

// ============ 图案参数 ============
const fabricRotation = ref(0)
const fabricOpacity = ref(100)
const fabricScale = ref(100)
const tileMode = ref<'seamless' | 'single'>('seamless')
const fabricPreviewStyle = computed(() => ({ transform: `rotate(${fabricRotation.value}deg) scale(${fabricScale.value / 100})`, opacity: fabricOpacity.value / 100 }))
function resetParams() { fabricRotation.value = 0; fabricOpacity.value = 100; fabricScale.value = 100; tileMode.value = 'seamless' }

// ============ 涂抹画布（局部模式） ============
const canvasWrapRef = ref<HTMLElement | null>(null)
const bgImageRef = ref<HTMLImageElement | null>(null)
const paintCanvasRef = ref<HTMLCanvasElement | null>(null)
const previewCanvasRef = ref<HTMLCanvasElement | null>(null)
const viewportRef = ref<HTMLElement | null>(null)
const showPaintModal = ref(false)
const paintTool = ref<'brush' | 'eraser' | 'hand'>('brush')
const brushSize = ref(30)
const hasPainted = ref(false)
const paintingNow = ref(false)
const paintHistory = ref<ImageData[]>([])
const redoStack = ref<ImageData[]>([])
let savedPaintData: ImageData | null = null
const zoom = ref(1); const panX = ref(0); const panY = ref(0); const panning = ref(false)
let panStartX = 0; let panStartY = 0; let panStartPX = 0; let panStartPY = 0; let fitZoom = 1; let canvasW = 0; let canvasH = 0
const cursorX = ref(0); const cursorY = ref(0); const cursorVisible = ref(false)
const canvasWrapStyle = computed(() => ({ transform: `translate(${panX.value}px, ${panY.value}px) scale(${zoom.value})`, transformOrigin: '0 0' }))
const brushCursorStyle = computed(() => { const size = brushSize.value * zoom.value; return { width: size + 'px', height: size + 'px', left: (cursorX.value - size / 2) + 'px', top: (cursorY.value - size / 2) + 'px', borderColor: paintTool.value === 'eraser' ? '#fff' : 'rgba(255,80,80,0.8)' } })

function openPaintModal() { showPaintModal.value = true; nextTick(() => { if (savedPaintData && paintCanvasRef.value) { const ctx = paintCanvasRef.value.getContext('2d'); if (ctx) ctx.putImageData(savedPaintData, 0, 0) }; document.addEventListener('keydown', onKeyDown) }) }
function confirmPaintModal() { const canvas = paintCanvasRef.value; if (canvas) { const ctx = canvas.getContext('2d'); if (ctx) { savedPaintData = ctx.getImageData(0, 0, canvas.width, canvas.height); hasPainted.value = isCanvasNonEmpty(canvas) } }; closePaintModal(); syncPreviewCanvas() }
function cancelPaintModal() { const canvas = paintCanvasRef.value; if (canvas) { const ctx = canvas.getContext('2d'); if (ctx) { if (savedPaintData) ctx.putImageData(savedPaintData, 0, 0); else ctx.clearRect(0, 0, canvas.width, canvas.height) } }; paintHistory.value = []; redoStack.value = []; closePaintModal() }
function closePaintModal() { showPaintModal.value = false; document.removeEventListener('keydown', onKeyDown) }
function syncPreviewCanvas() { const preview = previewCanvasRef.value; if (!preview || !savedPaintData) return; preview.width = savedPaintData.width; preview.height = savedPaintData.height; const ctx = preview.getContext('2d'); if (ctx) ctx.putImageData(savedPaintData, 0, 0) }
function initCanvas() { const canvas = paintCanvasRef.value; const img = bgImageRef.value; if (!canvas || !img) return; canvasW = img.naturalWidth; canvasH = img.naturalHeight; canvas.width = canvasW; canvas.height = canvasH; paintHistory.value = []; redoStack.value = []; if (savedPaintData) { const ctx = canvas.getContext('2d'); if (ctx) ctx.putImageData(savedPaintData, 0, 0) }; nextTick(() => { zoomFit() }) }

watch(garmentImageUrl, (newVal, oldVal) => { hasPainted.value = false; paintHistory.value = []; redoStack.value = []; savedPaintData = null; if (newVal && newVal !== oldVal && patternMode.value === 'partial') nextTick(() => { openPaintModal() }) })

function zoomIn() { setZoom(zoom.value * 1.25) }
function zoomOut() { setZoom(zoom.value / 1.25) }
function zoomReset() { zoom.value = 1; centerCanvas() }
function zoomFit() { const vp = viewportRef.value; if (!vp || !canvasW || !canvasH) return; const pad = 40; fitZoom = Math.min((vp.clientWidth - pad) / canvasW, (vp.clientHeight - pad) / canvasH, 1); zoom.value = fitZoom; centerCanvas() }
function setZoom(z: number) { zoom.value = Math.max(0.1, Math.min(5, z)); clampPan() }
function centerCanvas() { const vp = viewportRef.value; if (!vp) return; panX.value = (vp.clientWidth - canvasW * zoom.value) / 2; panY.value = (vp.clientHeight - canvasH * zoom.value) / 2 }
function clampPan() { const vp = viewportRef.value; if (!vp) return; const w = canvasW * zoom.value; const h = canvasH * zoom.value; const vpW = vp.clientWidth; const vpH = vp.clientHeight; if (w <= vpW) panX.value = (vpW - w) / 2; else panX.value = Math.min(0, Math.max(vpW - w, panX.value)); if (h <= vpH) panY.value = (vpH - h) / 2; else panY.value = Math.min(0, Math.max(vpH - h, panY.value)) }
function viewportToCanvas(vx: number, vy: number): { x: number; y: number } | null { if (!canvasW || !canvasH) return null; const x = (vx - panX.value) / zoom.value; const y = (vy - panY.value) / zoom.value; if (x < 0 || y < 0 || x > canvasW || y > canvasH) return null; return { x, y } }
function getViewportPos(e: MouseEvent): { vx: number; vy: number } { const vp = viewportRef.value!; const rect = vp.getBoundingClientRect(); return { vx: e.clientX - rect.left, vy: e.clientY - rect.top } }

let lastPos: { x: number; y: number } | null = null
function onViewportMouseDown(e: MouseEvent) { const { vx, vy } = getViewportPos(e); if (paintTool.value === 'hand' || e.button === 1) { panning.value = true; panStartX = e.clientX; panStartY = e.clientY; panStartPX = panX.value; panStartPY = panY.value; return }; const pos = viewportToCanvas(vx, vy); if (!pos) return; paintingNow.value = true; saveCanvasState(); drawDot(pos.x, pos.y); lastPos = pos; hasPainted.value = true }
function onViewportMouseMove(e: MouseEvent) { const { vx, vy } = getViewportPos(e); cursorX.value = vx; cursorY.value = vy; cursorVisible.value = true; if (panning.value) { panX.value = panStartPX + (e.clientX - panStartX); panY.value = panStartPY + (e.clientY - panStartY); clampPan(); return }; if (!paintingNow.value) return; const pos = viewportToCanvas(vx, vy); if (!pos || !lastPos) return; drawLine(lastPos.x, lastPos.y, pos.x, pos.y); lastPos = pos }
function onViewportMouseUp() { paintingNow.value = false; panning.value = false; lastPos = null }
function onViewportMouseLeave() { paintingNow.value = false; panning.value = false; lastPos = null; cursorVisible.value = false }
function onViewportWheel(e: WheelEvent) { const { vx, vy } = getViewportPos(e); const oldZ = zoom.value; const newZ = Math.max(0.1, Math.min(5, oldZ * (e.deltaY < 0 ? 1.15 : 1 / 1.15))); zoom.value = newZ; panX.value = vx - (vx - panX.value) * (newZ / oldZ); panY.value = vy - (vy - panY.value) * (newZ / oldZ); clampPan() }
let lastTouchPos: { x: number; y: number } | null = null
function onViewportTouchStart(e: TouchEvent) { if (e.touches.length !== 1) return; const vp = viewportRef.value!; const rect = vp.getBoundingClientRect(); const vx = e.touches[0].clientX - rect.left; const vy = e.touches[0].clientY - rect.top; if (paintTool.value === 'hand') { panning.value = true; panStartX = e.touches[0].clientX; panStartY = e.touches[0].clientY; panStartPX = panX.value; panStartPY = panY.value; return }; const pos = viewportToCanvas(vx, vy); if (!pos) return; paintingNow.value = true; saveCanvasState(); drawDot(pos.x, pos.y); lastTouchPos = pos; hasPainted.value = true }
function onViewportTouchMove(e: TouchEvent) { if (e.touches.length !== 1) return; if (panning.value) { panX.value = panStartPX + (e.touches[0].clientX - panStartX); panY.value = panStartPY + (e.touches[0].clientY - panStartY); clampPan(); return }; if (!paintingNow.value) return; const vp = viewportRef.value!; const rect = vp.getBoundingClientRect(); const vx = e.touches[0].clientX - rect.left; const vy = e.touches[0].clientY - rect.top; const pos = viewportToCanvas(vx, vy); if (!pos || !lastTouchPos) return; drawLine(lastTouchPos.x, lastTouchPos.y, pos.x, pos.y); lastTouchPos = pos }
function onViewportTouchEnd() { paintingNow.value = false; panning.value = false; lastTouchPos = null }

function saveCanvasState() { const canvas = paintCanvasRef.value; if (!canvas) return; const ctx = canvas.getContext('2d'); if (!ctx) return; paintHistory.value.push(ctx.getImageData(0, 0, canvas.width, canvas.height)); if (paintHistory.value.length > 50) paintHistory.value.shift(); redoStack.value = [] }
function drawDot(x: number, y: number) { const canvas = paintCanvasRef.value; if (!canvas) return; const ctx = canvas.getContext('2d'); if (!ctx) return; ctx.beginPath(); ctx.arc(x, y, brushSize.value / 2, 0, Math.PI * 2); if (paintTool.value === 'eraser') { ctx.globalCompositeOperation = 'destination-out'; ctx.fill(); ctx.globalCompositeOperation = 'source-over' } else { ctx.fillStyle = 'rgba(255, 80, 80, 0.55)'; ctx.fill() } }
function drawLine(x1: number, y1: number, x2: number, y2: number) { const canvas = paintCanvasRef.value; if (!canvas) return; const ctx = canvas.getContext('2d'); if (!ctx) return; ctx.lineWidth = brushSize.value; ctx.lineCap = 'round'; ctx.lineJoin = 'round'; if (paintTool.value === 'eraser') { ctx.globalCompositeOperation = 'destination-out'; ctx.strokeStyle = 'rgba(0,0,0,1)'; ctx.beginPath(); ctx.moveTo(x1, y1); ctx.lineTo(x2, y2); ctx.stroke(); ctx.globalCompositeOperation = 'source-over' } else { ctx.strokeStyle = 'rgba(255, 80, 80, 0.55)'; ctx.beginPath(); ctx.moveTo(x1, y1); ctx.lineTo(x2, y2); ctx.stroke() } }
function undoPaint() { const canvas = paintCanvasRef.value; if (!canvas || paintHistory.value.length === 0) return; const ctx = canvas.getContext('2d'); if (!ctx) return; redoStack.value.push(ctx.getImageData(0, 0, canvas.width, canvas.height)); const prev = paintHistory.value.pop()!; ctx.putImageData(prev, 0, 0); if (paintHistory.value.length === 0) hasPainted.value = isCanvasNonEmpty(canvas) }
function redoPaint() { const canvas = paintCanvasRef.value; if (!canvas || redoStack.value.length === 0) return; const ctx = canvas.getContext('2d'); if (!ctx) return; paintHistory.value.push(ctx.getImageData(0, 0, canvas.width, canvas.height)); const next = redoStack.value.pop()!; ctx.putImageData(next, 0, 0); hasPainted.value = isCanvasNonEmpty(canvas) }
function clearPaint() { const canvas = paintCanvasRef.value; if (!canvas) return; const ctx = canvas.getContext('2d'); if (!ctx) return; saveCanvasState(); ctx.clearRect(0, 0, canvas.width, canvas.height); hasPainted.value = false }
function isCanvasNonEmpty(canvas: HTMLCanvasElement): boolean { const ctx = canvas.getContext('2d'); if (!ctx) return false; const data = ctx.getImageData(0, 0, canvas.width, canvas.height).data; for (let i = 3; i < data.length; i += 4) { if (data[i] > 0) return true }; return false }
function onKeyDown(e: KeyboardEvent) { if (!showPaintModal.value) return; const ctrl = e.ctrlKey || e.metaKey; if (ctrl && e.shiftKey && e.key.toLowerCase() === 'z') { e.preventDefault(); redoPaint(); return }; if (ctrl && e.key.toLowerCase() === 'z') { e.preventDefault(); undoPaint(); return }; if (e.key === 'b' || e.key === 'B') { paintTool.value = 'brush'; return }; if (e.key === 'e' || e.key === 'E') { paintTool.value = 'eraser'; return }; if (e.key === 'h' || e.key === 'H') { paintTool.value = 'hand'; return }; if (e.key === '=' || e.key === '+') { e.preventDefault(); zoomIn(); return }; if (e.key === '-') { e.preventDefault(); zoomOut(); return }; if (e.key === '0') { e.preventDefault(); zoomFit(); return }; if (e.key === '1') { e.preventDefault(); zoomReset(); return }; if (e.key === '[') { brushSize.value = Math.max(5, brushSize.value - 5); return }; if (e.key === ']') { brushSize.value = Math.min(80, brushSize.value + 5); return } }

async function exportPaintedImage(): Promise<File> {
  if (!savedPaintData) throw new Error('无涂抹数据')
  const img = await new Promise<HTMLImageElement>((resolve, reject) => { const el = new Image(); el.crossOrigin = 'anonymous'; el.onload = () => resolve(el); el.onerror = () => reject(new Error('原图加载失败')); el.src = garmentImageUrl.value })
  const paintLayer = document.createElement('canvas'); paintLayer.width = savedPaintData.width; paintLayer.height = savedPaintData.height; paintLayer.getContext('2d')!.putImageData(savedPaintData, 0, 0)
  const offscreen = document.createElement('canvas'); offscreen.width = img.naturalWidth; offscreen.height = img.naturalHeight; const ctx = offscreen.getContext('2d')!
  ctx.drawImage(img, 0, 0, img.naturalWidth, img.naturalHeight); ctx.globalAlpha = 0.35; ctx.drawImage(paintLayer, 0, 0, paintLayer.width, paintLayer.height, 0, 0, img.naturalWidth, img.naturalHeight); ctx.globalAlpha = 1.0
  return new Promise((resolve) => { offscreen.toBlob((blob) => { resolve(new File([blob!], 'painted_' + Date.now() + '.png', { type: 'image/png' })) }, 'image/png') })
}

// ============ 消息气泡 ============
interface ChatMessage { id: string; role: 'user' | 'assistant'; prompt?: string; refImageUrls?: string[]; status?: 'pending' | 'completed' | 'failed'; imageUrl?: string; errorMsg?: string; taskId?: string; elapsed?: string; time?: string }
const messages = ref<ChatMessage[]>([])
const chatMessagesRef = ref<HTMLElement | null>(null)
function scrollToBottom() { nextTick(() => { if (chatMessagesRef.value) chatMessagesRef.value.scrollTop = chatMessagesRef.value.scrollHeight }) }

// ============ 生成 ============
const generating = ref(false)
const uploading = ref(false)
const currentTaskId = ref<string | null>(null)
const lightboxUrl = ref('')

const canGenerate = computed(() => {
  if (!userStore.isLoggedIn) return false
  if (generating.value || uploading.value) return false
  if (!garmentImageUrl.value || !patternImageUrl.value) return false
  if (patternMode.value === 'partial' && !hasPainted.value) return false
  return true
})

async function handleGenerate() {
  if (!userStore.isLoggedIn) { message.warning('请先登录'); router.push('/user/login'); return }
  if (!garmentImageUrl.value || !patternImageUrl.value) { message.warning('请上传衣服图片和图案图片'); return }
  if (patternMode.value === 'partial' && (!hasPainted.value || !savedPaintData)) { message.warning('请先涂抹图案放置区域'); return }

  activeTaskId.value = null
  const now = new Date()
  const timeStr = `${now.getHours().toString().padStart(2, '0')}:${now.getMinutes().toString().padStart(2, '0')}`
  const tileModeLabel = tileMode.value === 'seamless' ? '四方连续' : '非四方连续'
  const modeLabel = patternMode.value === 'full' ? '全身' : '局部'
  const catLabel = patternMode.value === 'full' && selectedCategory.value ? `，品类：${selectedCategory.value}` : ''
  const displayPrompt = `${FEATURE_TITLE}（${modeLabel}，${tileModeLabel}，旋转${fabricRotation.value}°，大小${fabricScale.value}%，透明度${fabricOpacity.value}%${catLabel}）`

  // 局部模式需要上传涂抹图
  let paintedUrl = ''
  if (patternMode.value === 'partial') {
    uploading.value = true
    try {
      const file = await exportPaintedImage()
      const uploadRes: any = await uploadApi.upload(file)
      if (uploadRes.code !== 0 || !uploadRes.data?.url) { message.error(uploadRes.msg || '涂抹图上传失败'); uploading.value = false; return }
      paintedUrl = uploadRes.data.url
    } catch (e: any) { message.error('涂抹图上传失败: ' + (e.message || '网络错误')); uploading.value = false; return }
    uploading.value = false
  }

  const imageUrls = patternMode.value === 'partial' ? [paintedUrl, patternImageUrl.value] : [garmentImageUrl.value, patternImageUrl.value]

  const userMsg: ChatMessage = { id: 'msg_' + Date.now(), role: 'user', prompt: displayPrompt, refImageUrls: [garmentImageUrl.value, patternImageUrl.value], time: timeStr }
  messages.value.push(userMsg)
  const assistantMsg: ChatMessage = { id: 'loading_' + Date.now(), role: 'assistant', prompt: displayPrompt, status: 'pending' }
  messages.value.push(assistantMsg)
  const assistantRef = messages.value[messages.value.length - 1]
  scrollToBottom()

  generating.value = true; currentTaskId.value = null

  try {
    const params: Record<string, any> = {
      image_urls: imageUrls,
      feature: FEATURE_SLUG,
      pattern_mode: patternMode.value,
      fabric_rotation: fabricRotation.value,
      fabric_opacity: fabricOpacity.value,
      fabric_scale: fabricScale.value,
      tile_mode: tileMode.value,
    }
    if (patternMode.value === 'full' && selectedCategory.value) params.category = selectedCategory.value
    if (patternMode.value === 'partial') params.original_image_url = garmentImageUrl.value

    const res: any = await taskApi.submit({
      prompt: FEATURE_PREFIX + `${modeLabel}，${tileModeLabel}，旋转${fabricRotation.value}°，大小${fabricScale.value}%，透明度${fabricOpacity.value}%${catLabel}`,
      params,
    })
    if (res.code === 0) { currentTaskId.value = res.data.task_id; assistantRef.taskId = res.data.task_id; pollTaskStatus(res.data.task_id, assistantRef) }
    else { assistantRef.status = 'failed'; assistantRef.errorMsg = res.msg || '提交失败'; generating.value = false }
  } catch (e: any) { assistantRef.status = 'failed'; assistantRef.errorMsg = e.msg || '网络错误，请重试'; generating.value = false }
}

let pollTimer: number | null = null
async function pollTaskStatus(taskId: string, assistantMsg: ChatMessage) {
  if (pollTimer) clearTimeout(pollTimer)
  try { const res: any = await taskApi.get(taskId); if (res.code === 0) { const task = res.data.task; if (task.status === 'completed') { try { const result = JSON.parse(task.result || '{}'); assistantMsg.imageUrl = result.image_url || ''; if (task.completed_at) { const t = new Date(task.updated_at).getTime() - new Date(task.created_at).getTime(); assistantMsg.elapsed = (t / 1000).toFixed(1) } } catch { assistantMsg.imageUrl = '' }; assistantMsg.status = 'completed'; generating.value = false; scrollToBottom(); loadHistory(); return } else if (task.status === 'failed') { assistantMsg.status = 'failed'; assistantMsg.errorMsg = task.error_msg || '生成失败'; generating.value = false; scrollToBottom(); loadHistory(); return } } } catch {}
  pollTimer = window.setTimeout(() => pollTaskStatus(taskId, assistantMsg), 3000)
}

function handleClear() { garmentImageUrl.value = ''; patternImageUrl.value = ''; selectedCategory.value = ''; resetParams(); messages.value = []; activeTaskId.value = null; hasPainted.value = false; paintHistory.value = []; redoStack.value = []; savedPaintData = null }
function handleImgError(e: Event) { (e.target as HTMLImageElement).style.background = '#27272a' }
function downloadImage(url?: string) {
  downloadImageByProxy(url)
}

onMounted(() => { if (userStore.isLoggedIn) loadHistory() })
onUnmounted(() => { if (pollTimer) clearTimeout(pollTimer); document.removeEventListener('keydown', onKeyDown) })
</script>

<style scoped>
.home-layout { width: 100%; height: 100%; display: flex; flex-direction: column; background: #111113; overflow: hidden; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif; }
.home-body { flex: 1; display: grid; grid-template-columns: 220px 1fr 380px; min-height: 0; overflow: hidden; }
.history-panel { border-right: 1px solid #222225; background: #111113; display: flex; flex-direction: column; overflow: hidden; min-height: 0; }
.history-header { padding: 14px 16px 10px; font-size: 13px; font-weight: 600; color: #ccc; border-bottom: 1px solid #222225; flex-shrink: 0; display: flex; align-items: center; justify-content: space-between; }
.gallery-count { font-size: 11px; color: #a3e635; background: rgba(163,230,53,0.1); padding: 1px 8px; border-radius: 10px; font-weight: 500; }
.gallery-grid { flex: 1; overflow-y: auto; padding: 8px; column-count: 2; column-gap: 6px; }
.gallery-item { break-inside: avoid; margin-bottom: 6px; border-radius: 8px; overflow: hidden; position: relative; cursor: pointer; background: #1a1a1d; border: 2px solid transparent; transition: border-color 0.2s; }
.gallery-item.active { border-color: #a3e635; }
.gallery-item img { display: block; width: 100%; }
.gallery-item-overlay { position: absolute; bottom: 0; left: 0; right: 0; padding: 4px 6px; background: linear-gradient(transparent, rgba(0,0,0,0.7)); }
.gallery-item-prompt { font-size: 10px; color: #aaa; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; display: block; }
.gallery-delete-btn { position: absolute; top: 4px; right: 4px; width: 20px; height: 20px; border-radius: 50%; background: rgba(0,0,0,0.6); border: none; color: #999; cursor: pointer; display: flex; align-items: center; justify-content: center; opacity: 0; transition: opacity 0.15s; }
.gallery-item:hover .gallery-delete-btn { opacity: 1; }
.history-empty { flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; color: #555; font-size: 12px; gap: 6px; }
.canvas-area { display: flex; flex-direction: column; overflow: hidden; background: #111113; }
.guest-prompt { flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 8px; color: #888; }
.guest-title { font-size: 15px; font-weight: 600; color: #ccc; }
.guest-desc { font-size: 13px; color: #888; }
.guest-action { font-size: 13px; color: #888; }
.link-highlight { color: #a3e635; text-decoration: none; font-weight: 600; }
.gen-display { flex: 1; overflow-y: auto; padding: 24px 32px; display: flex; flex-direction: column; gap: 16px; }
.chat-empty { flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 8px; }
.empty-icon-wrap { margin-bottom: 8px; }
.chat-empty-text { font-size: 16px; color: #888; font-weight: 500; }
.chat-empty-sub { font-size: 12px; color: #555; }
.chat-bubble { display: flex; flex-direction: column; }
.chat-bubble.user { align-items: flex-end; }
.chat-bubble.assistant { align-items: flex-start; }
.bubble-row { max-width: 80%; }
.bubble-row.user { display: flex; flex-direction: column; align-items: flex-end; gap: 4px; }
.bubble-row.ai { display: flex; flex-direction: column; align-items: flex-start; gap: 4px; }
.bubble-body { border-radius: 12px; padding: 12px 16px; }
.bubble-body.user { background: rgba(163,230,53,0.1); border: 1px solid rgba(163,230,53,0.2); }
.bubble-body.failed { background: rgba(248,113,113,0.08); border: 1px solid rgba(248,113,113,0.25); }
.bubble-body.ai-success { background: #1a1a1e; border: 1px solid #27272a; }
.bubble-prompt { font-size: 13px; color: #ccc; line-height: 1.6; margin: 0; word-break: break-word; }
.bubble-time { font-size: 10px; color: #444; padding: 0 4px; }
.bubble-ref-images { display: flex; gap: 8px; margin-top: 8px; }
.bubble-ref-img { width: 80px; height: 80px; object-fit: cover; border-radius: 8px; border: 1px solid #333; }
.bubble-failed-row { display: flex; align-items: center; gap: 12px; }
.bubble-failed-icon { width: 32px; height: 32px; border-radius: 50%; background: rgba(248,113,113,0.15); color: #f87171; display: flex; align-items: center; justify-content: center; font-size: 16px; flex-shrink: 0; }
.bubble-failed-info { flex: 1; }
.bubble-failed-title { font-size: 13px; color: #f87171; font-weight: 500; }
.bubble-failed-reason { font-size: 12px; color: #888; margin-top: 3px; word-break: break-word; }
.bubble-image-wrap { border-radius: 10px; overflow: hidden; max-width: 400px; cursor: pointer; position: relative; }
.bubble-image-wrap:hover .image-zoom-hint { opacity: 1; }
.image-zoom-hint { position: absolute; top: 8px; right: 8px; width: 32px; height: 32px; border-radius: 8px; background: rgba(0,0,0,0.55); display: flex; align-items: center; justify-content: center; color: #fff; opacity: 0; transition: opacity 0.2s; pointer-events: none; }
.bubble-image { width: 100%; display: block; transition: transform 0.2s; }
.bubble-image-wrap:hover .bubble-image { transform: scale(1.02); }
.bubble-footer { display: flex; align-items: center; justify-content: space-between; margin-top: 10px; padding-top: 8px; border-top: 1px solid #27272a; }
.bubble-meta { font-size: 11px; color: #555; }
.result-actions { display: flex; align-items: center; gap: 8px; }
.result-action-btn { display: inline-flex; align-items: center; gap: 6px; padding: 5px 10px; border-radius: 6px; border: 1px solid #333; background: #27272a; color: #aaa; font-size: 12px; cursor: pointer; transition: all 0.2s; }
.result-action-btn:hover { border-color: #444; color: #ddd; }
.result-action-btn.primary { background: rgba(163,230,53,0.1); border-color: rgba(163,230,53,0.3); color: #a3e635; }
.result-action-btn.primary:hover { background: rgba(163,230,53,0.18); border-color: #a3e635; }
.result-action-btn.danger { color: #888; }
.result-action-btn.danger:hover { background: rgba(220,38,38,0.12); border-color: rgba(220,38,38,0.4); color: #ef4444; }
.chat-bubble.highlight .bubble-body, .chat-bubble.highlight .bubble-image-wrap { outline: 2px solid #a3e635; outline-offset: 2px; transition: outline 0.3s; }
.delete-popover { position: fixed; z-index: 1000; background: #27272a; border: 1px solid #3f3f46; border-radius: 10px; padding: 12px 16px; box-shadow: 0 8px 24px rgba(0,0,0,0.4); transform: translate(-50%, -100%); margin-top: -8px; }
.delete-popover p { margin: 0 0 8px; font-size: 13px; color: #e4e4e7; }
.delete-popover-btns { display: flex; gap: 8px; justify-content: flex-end; }
.delete-popover-btns button { padding: 4px 14px; border-radius: 6px; border: 1px solid #3f3f46; background: #18181b; color: #a1a1aa; font-size: 12px; cursor: pointer; }
.delete-popover-btns button.confirm { background: rgba(220,38,38,0.2); border-color: rgba(220,38,38,0.4); color: #ef4444; }
.skeleton-wrap { width: 240px; aspect-ratio: 3 / 4; border-radius: 12px; background: #1a1a1e; border: 1px solid #27272a; position: relative; overflow: hidden; }
.skeleton-shimmer { position: absolute; inset: 0; background: linear-gradient(110deg, transparent 30%, rgba(163,230,53,0.04) 50%, transparent 70%); background-size: 300% 100%; animation: shimmer 2s ease-in-out infinite; }
@keyframes shimmer { 0% { background-position: 200% 0; } 100% { background-position: -100% 0; } }
.skeleton-overlay { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 12px; }
.skeleton-spinner { width: 32px; height: 32px; border: 3px solid #27272a; border-top-color: #a3e635; border-radius: 50%; animation: spin 1s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
.skeleton-text { font-size: 13px; color: #888; font-weight: 500; }
.skeleton-sub { font-size: 11px; color: #555; }
.control-panel { display: flex; flex-direction: column; border-left: 1px solid #222225; background: #18181b; overflow: hidden; }
.panel-top { padding: 12px 16px 0; flex-shrink: 0; }
.panel-tabs { display: flex; gap: 0; background: #27272a; border-radius: 8px; padding: 3px; }
.panel-tab { flex: 1; padding: 7px 0; border: none; border-radius: 6px; background: transparent; color: #888; font-size: 13px; font-weight: 500; cursor: pointer; transition: all 0.2s; }
.panel-tab.active { background: #3f3f46; color: #e4e4e7; }
.panel-scroll { flex: 1; overflow-y: auto; padding: 16px; }
.section { margin-bottom: 20px; }
.section-header-row { display: flex; align-items: center; gap: 8px; margin-bottom: 10px; }
.section-num { width: 22px; height: 22px; border-radius: 50%; background: rgba(163,230,53,0.12); color: #a3e635; font-size: 12px; font-weight: 700; display: flex; align-items: center; justify-content: center; flex-shrink: 0; }
.section-label { font-size: 13px; font-weight: 600; color: #d4d4d8; }

/* 模式选择器 */
.mode-selector { display: flex; gap: 8px; }
.mode-btn { flex: 1; display: flex; align-items: center; justify-content: center; gap: 8px; padding: 12px; border-radius: 10px; border: 1px solid #333; background: #1a1a1e; color: #a1a1aa; font-size: 13px; font-weight: 500; cursor: pointer; transition: all 0.2s; }
.mode-btn:hover { border-color: #555; color: #ddd; }
.mode-btn.active { background: rgba(163,230,53,0.1); border-color: rgba(163,230,53,0.4); color: #a3e635; }

/* 品类选择器 */
.category-selector { display: flex; align-items: center; justify-content: space-between; padding: 10px 14px; border-radius: 10px; border: 1px solid #333; background: #111113; cursor: pointer; transition: all 0.2s; }
.category-selector:hover { border-color: #555; }
.category-placeholder { font-size: 13px; color: #555; }
.category-selected { font-size: 13px; color: #a3e635; font-weight: 500; }
.category-selector svg { color: #555; flex-shrink: 0; }
.category-modal-overlay { position: fixed; inset: 0; z-index: 1000; background: rgba(0,0,0,0.65); display: flex; align-items: center; justify-content: center; }
.category-modal { width: 580px; max-width: 92vw; max-height: 80vh; background: #1a1a1e; border: 1px solid #333; border-radius: 14px; display: flex; flex-direction: column; box-shadow: 0 16px 48px rgba(0,0,0,0.5); }
.category-modal-header { display: flex; align-items: center; justify-content: space-between; padding: 16px 20px; border-bottom: 1px solid #27272a; flex-shrink: 0; }
.category-modal-title { font-size: 15px; font-weight: 600; color: #e4e4e7; }
.category-modal-close { width: 30px; height: 30px; border-radius: 8px; border: none; background: transparent; color: #888; cursor: pointer; display: flex; align-items: center; justify-content: center; }
.category-modal-close:hover { background: #27272a; color: #ccc; }
.category-modal-body { flex: 1; overflow-y: auto; padding: 16px 20px; display: flex; flex-direction: column; gap: 18px; }
.category-group-label { font-size: 12px; font-weight: 600; color: #a3e635; margin-bottom: 8px; }
.category-group-items { display: flex; flex-wrap: wrap; gap: 8px; }
.category-item { padding: 6px 14px; border-radius: 8px; border: 1px solid #333; background: #27272a; color: #a1a1aa; font-size: 13px; cursor: pointer; transition: all 0.15s; white-space: nowrap; }
.category-item:hover { border-color: #555; color: #ddd; background: #3f3f46; }
.category-item.active { background: rgba(163,230,53,0.12); border-color: rgba(163,230,53,0.4); color: #a3e635; font-weight: 500; }
.category-modal-footer { padding: 12px 20px; border-top: 1px solid #27272a; display: flex; justify-content: flex-end; flex-shrink: 0; }
.category-clear-btn { padding: 6px 16px; border-radius: 8px; border: 1px solid #333; background: #27272a; color: #888; font-size: 12px; cursor: pointer; }
.category-clear-btn:hover { border-color: #555; color: #ccc; }

/* 面料预览 */
.fabric-preview-wrap { margin-bottom: 16px; text-align: center; }
.fabric-preview-box { width: 120px; height: 120px; margin: 0 auto 6px; border-radius: 10px; border: 1px solid #333; background: repeating-conic-gradient(#1a1a1e 0% 25%, #111113 0% 50%) 0 0 / 12px 12px; overflow: hidden; display: flex; align-items: center; justify-content: center; }
.fabric-preview-img { max-width: 100%; max-height: 100%; transition: transform 0.2s, opacity 0.2s; pointer-events: none; }
.fabric-preview-hint { font-size: 10px; color: #555; }
.param-group { margin-bottom: 16px; }
.param-label { font-size: 12px; font-weight: 600; color: #a1a1aa; margin-bottom: 8px; }
.param-label-row { display: flex; align-items: center; justify-content: space-between; margin-bottom: 8px; }
.param-value { font-size: 12px; color: #a3e635; font-weight: 600; font-variant-numeric: tabular-nums; }
.param-slider { width: 100%; height: 4px; -webkit-appearance: none; appearance: none; background: #333; border-radius: 2px; outline: none; }
.param-slider::-webkit-slider-thumb { -webkit-appearance: none; width: 16px; height: 16px; border-radius: 50%; background: #a3e635; cursor: pointer; border: 2px solid #111; box-shadow: 0 0 4px rgba(163,230,53,0.4); }
.param-ticks { display: flex; justify-content: space-between; margin-top: 4px; }
.param-ticks span { font-size: 10px; color: #555; cursor: pointer; padding: 2px 4px; border-radius: 4px; transition: all 0.15s; }
.param-ticks span:hover { color: #a3e635; background: rgba(163,230,53,0.08); }
.tile-mode-selector { display: flex; gap: 8px; }
.tile-mode-btn { flex: 1; display: flex; align-items: center; justify-content: center; gap: 6px; padding: 10px 12px; border-radius: 10px; border: 1px solid #333; background: #1a1a1e; color: #a1a1aa; font-size: 12px; font-weight: 500; cursor: pointer; transition: all 0.2s; }
.tile-mode-btn:hover { border-color: #555; color: #ddd; }
.tile-mode-btn.active { background: rgba(163,230,53,0.1); border-color: rgba(163,230,53,0.4); color: #a3e635; }
.reset-params-btn { display: flex; align-items: center; justify-content: center; gap: 6px; width: 100%; padding: 8px 14px; border-radius: 8px; border: 1px solid #333; background: #27272a; color: #888; font-size: 12px; cursor: pointer; transition: all 0.15s; }
.reset-params-btn:hover { border-color: #555; color: #ccc; }
.status-section { padding: 12px 14px; border-radius: 10px; border: 1px solid #27272a; background: #1a1a1e; }
.status-ready, .status-waiting { display: flex; align-items: center; gap: 10px; font-size: 12px; }
.status-ready { color: #a3e635; }
.status-waiting { color: #71717a; }

/* 涂抹预览 */
.paint-preview { position: relative; border-radius: 10px; overflow: hidden; cursor: pointer; border: 1px solid #27272a; transition: border-color 0.2s; }
.paint-preview:hover { border-color: #a3e635; }
.paint-preview-img { display: block; width: 100%; height: auto; pointer-events: none; }
.paint-preview-overlay { position: absolute; top: 0; left: 0; width: 100%; height: 100%; pointer-events: none; }
.paint-preview-mask { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 6px; background: rgba(0,0,0,0.5); color: #a3e635; font-size: 13px; font-weight: 500; opacity: 0; transition: opacity 0.2s; }
.paint-preview:hover .paint-preview-mask { opacity: 1; }
.paint-done-tip { display: flex; align-items: center; gap: 6px; margin-top: 8px; font-size: 12px; color: #a3e635; }

/* 涂抹弹窗 */
.paint-modal-overlay { position: fixed; inset: 0; z-index: 1100; background: rgba(0,0,0,0.85); display: flex; align-items: center; justify-content: center; }
.paint-modal { width: 94vw; max-width: 1100px; height: 90vh; background: #18181b; border-radius: 14px; border: 1px solid #27272a; display: flex; flex-direction: column; overflow: hidden; box-shadow: 0 20px 60px rgba(0,0,0,0.5); }
.paint-modal-header { display: flex; align-items: center; gap: 12px; padding: 10px 20px; border-bottom: 1px solid #27272a; flex-shrink: 0; }
.paint-modal-title { font-size: 14px; font-weight: 600; color: #e4e4e7; }
.paint-modal-hint { font-size: 12px; color: #71717a; flex: 1; }
.header-actions { display: flex; gap: 8px; }
.paint-modal-body { flex: 1; overflow: hidden; position: relative; background: #0a0a0c; cursor: crosshair; }
.paint-modal-body.cursor-grab { cursor: grab; }
.paint-modal-body.cursor-grabbing { cursor: grabbing; }
.paint-modal-footer { padding: 10px 16px; border-top: 1px solid #27272a; flex-shrink: 0; }
.paint-canvas-wrap { position: absolute; top: 0; left: 0; border: 1px solid #333; background: repeating-conic-gradient(#1a1a1e 0% 25%, #111113 0% 50%) 0 0 / 16px 16px; }
.paint-bg-image { display: block; width: 100%; height: auto; pointer-events: none; user-select: none; }
.paint-canvas { position: absolute; top: 0; left: 0; width: 100%; height: 100%; touch-action: none; }
.paint-hint { position: absolute; bottom: 12px; left: 50%; transform: translateX(-50%); display: flex; align-items: center; gap: 6px; padding: 6px 14px; background: rgba(0,0,0,0.75); border-radius: 8px; color: #a3e635; font-size: 12px; pointer-events: none; white-space: nowrap; }
.brush-cursor { position: absolute; pointer-events: none; border: 2px solid rgba(255,80,80,0.8); border-radius: 50%; box-sizing: border-box; z-index: 10; mix-blend-mode: difference; }
.paint-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.toolbar-group { display: flex; align-items: center; gap: 5px; }
.toolbar-divider { width: 1px; height: 22px; background: #333; margin: 0 4px; }
.tool-btn { width: 34px; height: 34px; border-radius: 8px; border: 1px solid #333; background: #27272a; color: #aaa; cursor: pointer; display: flex; align-items: center; justify-content: center; transition: all 0.15s; flex-shrink: 0; }
.tool-btn:hover { border-color: #555; color: #ddd; }
.tool-btn.active { background: rgba(163,230,53,0.12); border-color: #a3e635; color: #a3e635; }
.tool-btn:disabled { opacity: 0.35; cursor: not-allowed; }
.tool-btn-text { padding: 6px 18px; border-radius: 8px; border: 1px solid #333; font-size: 13px; font-weight: 500; cursor: pointer; transition: all 0.15s; white-space: nowrap; }
.tool-btn-text.cancel { background: #27272a; color: #aaa; }
.tool-btn-text.cancel:hover { background: #3f3f46; color: #ddd; }
.tool-btn-text.confirm { background: linear-gradient(135deg, #a3e635 0%, #65a30d 100%); border-color: #65a30d; color: #111; font-weight: 600; }
.tool-btn-text.confirm:hover { opacity: 0.9; }
.zoom-label { font-size: 11px; color: #888; min-width: 38px; text-align: center; font-variant-numeric: tabular-nums; }
.brush-size-wrap { display: flex; align-items: center; gap: 6px; margin-left: 2px; }
.brush-size-label { font-size: 11px; color: #888; min-width: 34px; text-align: center; }
.brush-size-slider { width: 90px; height: 4px; -webkit-appearance: none; appearance: none; background: #333; border-radius: 2px; outline: none; }
.brush-size-slider::-webkit-slider-thumb { -webkit-appearance: none; width: 14px; height: 14px; border-radius: 50%; background: #a3e635; cursor: pointer; }

.panel-bottom { padding: 16px; border-top: 1px solid #27272a; flex-shrink: 0; }
.button-group { display: flex; gap: 10px; }
.btn-generate { flex: 1; padding: 12px 24px; border-radius: 10px; border: none; background: linear-gradient(135deg, #a3e635 0%, #65a30d 100%); color: #111; font-size: 14px; font-weight: 600; cursor: pointer; transition: all 0.2s; }
.btn-generate:hover:not(:disabled) { opacity: 0.9; transform: translateY(-1px); }
.btn-generate:disabled { opacity: 0.4; cursor: not-allowed; }
.btn-clear { padding: 12px 24px; border-radius: 10px; border: 1px solid #333; background: #27272a; color: #aaa; font-size: 14px; cursor: pointer; }
.btn-clear:hover:not(:disabled) { background: #3f3f46; }
.lightbox-overlay { position: fixed; inset: 0; z-index: 999; background: rgba(0,0,0,0.85); display: flex; align-items: center; justify-content: center; }
.lightbox-container { position: relative; max-width: 90vw; max-height: 90vh; }
.lightbox-image { max-width: 100%; max-height: 85vh; border-radius: 8px; }
.lightbox-close { position: absolute; top: -10px; right: -10px; width: 32px; height: 32px; border-radius: 50%; background: #333; border: none; color: #ccc; cursor: pointer; display: flex; align-items: center; justify-content: center; }
.lightbox-actions { display: flex; gap: 8px; justify-content: center; margin-top: 12px; }
.lightbox-action-btn { padding: 8px 16px; border-radius: 8px; border: 1px solid #444; background: #27272a; color: #ccc; font-size: 13px; cursor: pointer; display: flex; align-items: center; gap: 6px; }
</style>
