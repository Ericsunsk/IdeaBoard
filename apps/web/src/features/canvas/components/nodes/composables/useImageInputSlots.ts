import { computed, ref, toValue, type MaybeRefOrGetter, type Ref } from 'vue'
import { useSortableMediaInputs } from './useSortableMediaInputs'

interface ImageInputSlotItem {
  id: string
}

interface UseImageInputSlotsOptions<T extends ImageInputSlotItem> {
  containerRef: Ref<HTMLElement | null>
  inputImages: MaybeRefOrGetter<readonly T[]>
  maxImages: number
  draggableSelector: string
  filterSelector?: string
  onUpdate: (items: T[]) => void
  onDrop: (event: DragEvent) => void
  onRemove: (item: T) => void
}

export function useImageInputSlots<T extends ImageInputSlotItem>({
  containerRef,
  inputImages,
  maxImages,
  draggableSelector,
  filterSelector,
  onUpdate,
  onDrop,
  onRemove,
}: UseImageInputSlotsOptions<T>) {
  const addSlotDragOver = ref(false)
  const visibleInputImages = computed(() => toValue(inputImages).slice(0, maxImages))
  const canAcceptMoreImages = computed(() => visibleInputImages.value.length < maxImages)
  const { sortableItems: sortableInputImages } = useSortableMediaInputs({
    containerRef,
    sourceItems: visibleInputImages,
    draggableSelector,
    filterSelector,
    onUpdate,
  })

  function removeInputImage(imageId: string) {
    const item = visibleInputImages.value.find((inputImage) => inputImage.id === imageId)
    if (!item) return
    onRemove(item)
  }

  function onAddSlotDragOver(event: DragEvent) {
    if (!canAcceptMoreImages.value) return
    addSlotDragOver.value = true
    if (event.dataTransfer) event.dataTransfer.dropEffect = 'copy'
  }

  function onAddSlotDragLeave() {
    addSlotDragOver.value = false
  }

  function onAddSlotDrop(event: DragEvent) {
    addSlotDragOver.value = false
    if (!canAcceptMoreImages.value) return
    onDrop(event)
  }

  return {
    addSlotDragOver,
    canAcceptMoreImages,
    onAddSlotDragLeave,
    onAddSlotDragOver,
    onAddSlotDrop,
    removeInputImage,
    sortableInputImages,
    visibleInputImages,
  }
}
