import { computed, ref } from 'vue'
import {
  clampMaskFeatherStrength,
  MASK_EDITOR_DEFAULT_FEATHER_STRENGTH,
} from '../config/maskEditorConstants'
import {
  type StrengthRangeRefTarget,
  useStrengthRangeEditSession,
} from '../../composables/useStrengthRangeEditSession'

const DEFAULT_LONG_PRESS_MS = 260
const FEATHER_STRENGTH_KEY = 'feather'

type UseMaskFeatherStrengthControlOptions = {
  onApply: (strength: number) => void
  longPressMs?: number
}

export function useMaskFeatherStrengthControl(options: UseMaskFeatherStrengthControlOptions) {
  const strength = ref(MASK_EDITOR_DEFAULT_FEATHER_STRENGTH)
  const rangeEdit = useStrengthRangeEditSession<typeof FEATHER_STRENGTH_KEY>({
    readValue: () => strength.value,
    normalizeValue: (_key, value) => clampMaskFeatherStrength(value),
    writeValue: (_key, value) => {
      strength.value = value
    },
    onCommit: () => {
      options.onApply(strength.value)
    },
    longPressMs: options.longPressMs ?? DEFAULT_LONG_PRESS_MS,
  })
  const visible = computed(() => rangeEdit.activeKey.value === FEATHER_STRENGTH_KEY)

  function setRangeRef(el: StrengthRangeRefTarget) {
    rangeEdit.setRangeRef(FEATHER_STRENGTH_KEY, el)
    rangeEdit.setTriggerRef(FEATHER_STRENGTH_KEY, el)
  }

  function onButtonPointerDown(event: PointerEvent) {
    rangeEdit.onRangePointerDown(FEATHER_STRENGTH_KEY, event)
  }

  function onButtonClick(event?: MouseEvent) {
    const input = event?.currentTarget instanceof HTMLInputElement ? event.currentTarget : null
    if (rangeEdit.consumeSuppressedClick(FEATHER_STRENGTH_KEY)) {
      input?.blur()
      return
    }
    input?.blur()
    options.onApply(strength.value)
  }

  function onStrengthInput(event: Event) {
    rangeEdit.onInput(FEATHER_STRENGTH_KEY, event)
  }

  function onStrengthChange(event: Event) {
    rangeEdit.onChange(FEATHER_STRENGTH_KEY, event)
  }

  function onStrengthPointerUp(event: PointerEvent) {
    rangeEdit.onPointerUp(FEATHER_STRENGTH_KEY, event)
  }

  return {
    strength,
    visible,
    setRangeRef,
    onButtonPointerDown,
    onButtonClick,
    onStrengthInput,
    onStrengthChange,
    onStrengthPointerUp,
    dispose: rangeEdit.dispose,
  }
}
