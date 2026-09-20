import {
  computed,
  inject,
  useId,
  type ComputedRef,
  type InjectionKey,
  type Ref,
} from 'vue'

export interface FormContext<TValues extends Record<string, unknown> = Record<string, unknown>> {
  values: TValues
  errors: Ref<Partial<Record<keyof TValues, string>>>
  touched: Ref<Partial<Record<keyof TValues, boolean>>>
  isSubmitting: Ref<boolean>
  isDirty: ComputedRef<boolean>
  isValid: ComputedRef<boolean>
  setFieldValue: <K extends keyof TValues>(field: K, value: TValues[K]) => void
  setFieldError: <K extends keyof TValues>(field: K, message: string) => void
  clearErrors: (field?: keyof TValues) => void
  validateField: <K extends keyof TValues>(field: K) => Promise<boolean>
  validate: () => Promise<boolean>
  handleSubmit: (
    onValid: (values: TValues) => Promise<void> | void,
    onInvalid?: (errors: Partial<Record<keyof TValues, string>>) => void,
  ) => (e?: Event) => Promise<void>
  resetForm: (nextValues?: Partial<TValues>) => void
}

export interface FormItemContext {
  id: string
}

export interface FormFieldContext {
  name: string
}

export const FORM_CONTEXT_KEY: InjectionKey<FormContext<Record<string, unknown>>> =
  Symbol('FORM_CONTEXT_KEY')
export const FORM_ITEM_CONTEXT_KEY: InjectionKey<FormItemContext> = Symbol('FORM_ITEM_CONTEXT_KEY')
export const FORM_FIELD_CONTEXT_KEY: InjectionKey<FormFieldContext> =
  Symbol('FORM_FIELD_CONTEXT_KEY')

export function useFormField() {
  const formContext = inject(FORM_CONTEXT_KEY, null)
  const fieldContext = inject(FORM_FIELD_CONTEXT_KEY, null)
  const itemContext = inject(FORM_ITEM_CONTEXT_KEY, null)

  const generatedId = useId()
  const id = itemContext?.id ?? `form-item-${generatedId}`
  const formItemId = id
  const formDescriptionId = `${id}-description`
  const formMessageId = `${id}-message`

  const name = fieldContext?.name

  const error = computed(() => {
    if (!name || !formContext) return undefined
    return formContext.errors.value[name]
  })

  const isTouched = computed(() => {
    if (!name || !formContext) return false
    return Boolean(formContext.touched.value[name])
  })

  const invalid = computed(() => Boolean(error.value))

  const ariaDescribedby = computed(() => {
    return error.value ? `${formDescriptionId} ${formMessageId}` : formDescriptionId
  })

  return {
    id,
    name,
    formItemId,
    formDescriptionId,
    formMessageId,
    error,
    isTouched,
    invalid,
    ariaDescribedby,
  }
}
