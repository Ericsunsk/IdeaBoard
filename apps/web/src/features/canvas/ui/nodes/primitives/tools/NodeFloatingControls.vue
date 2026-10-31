<template>
  <div class="node-floating-controls canvas-node-screen-ui">
    <slot />
  </div>
</template>

<style scoped>
.node-floating-controls {
  position: absolute;
  top: 100%;
  left: 50%;
  width: min(var(--canvas-node-floating-ui-width), calc(100vw - 32px));
  min-height: 0;
  translate: -50% 0;
  z-index: var(--z-index-popover);
  pointer-events: none;
}

.node-floating-controls :slotted(*) {
  pointer-events: auto;
}
</style>
