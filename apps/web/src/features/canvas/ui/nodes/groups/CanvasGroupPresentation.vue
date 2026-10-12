<template>
  <CanvasGroupFramesOverlay
    :frames="presentation.groupFrames.value"
    @resize-start="presentation.onGroupResizeStart"
  />
  <Teleport v-if="actionTarget" :to="actionTarget">
    <CanvasSelectionGroupActionButton
      v-if="presentation.isSelectionGroupActionVisible.value"
      :action="presentation.selectionGroupAction.value"
      :position="presentation.selectionGroupActionButtonPosition.value"
      @activate="presentation.onSelectionGroupAction"
    />
  </Teleport>
</template>

<script setup lang="ts">
import { CanvasGroupFramesOverlay, CanvasSelectionGroupActionButton } from '../../overlays'
import type { CanvasGroupPresentationInteraction } from './useCanvasGroupPresentationInteraction'

defineProps<{
  presentation: CanvasGroupPresentationInteraction
  actionTarget: HTMLElement | null
}>()
</script>
