import {
  computed,
  getCurrentInstance,
  provide,
  reactive,
  ref,
  unref,
  type Ref,
} from 'vue'
import * as v from 'valibot'
import { FORM_CONTEXT_KEY, type FormContext } from './context'

export type FormSchema = unknown

export interface UseFormOptions<TValues extends Record<string, unknown>> {
  schema?: FormSchema
  initialValues: TValues
  validateOnChange?: boolean
  validateOnBlur?: boolean
}

interface StandardSchemaIssue {
  path?: ReadonlyArray<{ key?: unknown } | unknown>
  message: string
}

interface StandardSchema {
  '~standard': {
    validate: (value: unknown) => { issues?: readonly StandardSchemaIssue[] } | Promise<unknown>
  }
}

function isStandardSchema(schema: unknown): schema is StandardSchema {
  if (!schema || typeof schema !== 'object') return false
  return '~standard' in schema && typeof (schema as StandardSchema)['~standard']?.validate === 'function'
}

function parseStandardIssues(issues?: readonly StandardSchemaIssue[]): Record<string, string> {
  const errors: Record<string, string> = {}
  if (!issues) return errors

  for (const issue of issues) {
    const firstSegment = issue.path?.[0]
    const key =
      typeof firstSegment === 'object' && firstSegment !== null && 'key' in firstSegment
        ? (firstSegment as { key?: unknown }).key
        : firstSegment
    if (key != null && !errors[String(key)]) {
      errors[String(key)] = issue.message
    }
  }
  return errors
}

async function extractStandardSchemaIssues(
  schema: StandardSchema,
  data: unknown,
): Promise<Record<string, string>> {
  const result = await schema['~standard'].validate(data)
  const issues = (result as { issues?: readonly StandardSchemaIssue[] })?.issues
  return parseStandardIssues(issues)
}

async function extractValibotDirectIssues(schema: unknown, data: unknown): Promise<Record<string, string>> {
  const errors: Record<string, string> = {}
  try {
    const result = await v.safeParseAsync(schema as v.GenericSchema, data)
    if (!result.success && result.issues) {
      for (const issue of result.issues) {
        const key = issue.path?.[0]?.key
        if (key != null && !errors[String(key)]) {
          errors[String(key)] = issue.message
        }
      }
    }
  } catch {
    // If schema parsing threw, return empty
  }
  return errors
}

async function extractIssues(schema: FormSchema, data: unknown): Promise<Record<string, string>> {
  const resolved = typeof schema === 'function' ? schema() : unref(schema)
  if (isStandardSchema(resolved)) {
    return extractStandardSchemaIssues(resolved, data)
  }
  return extractValibotDirectIssues(resolved, data)
}

export function cloneFormValue<T>(value: T): T {
  if (value === null || typeof value !== 'object') {
    return value
  }
  if (value instanceof Date) {
    return new Date(value.getTime()) as unknown as T
  }
  if (typeof File !== 'undefined' && value instanceof File) {
    return value
  }
  if (typeof Blob !== 'undefined' && value instanceof Blob) {
    return value
  }
  if (Array.isArray(value)) {
    return value.map((item) => cloneFormValue(item)) as unknown as T
  }
  const cloned: Record<string, unknown> = {}
  for (const key of Object.keys(value as Record<string, unknown>)) {
    cloned[key] = cloneFormValue((value as Record<string, unknown>)[key])
  }
  return cloned as T
}

export function areFormValuesEqual(a: unknown, b: unknown): boolean {
  if (a === b) return true
  if (a instanceof Date && b instanceof Date) return a.getTime() === b.getTime()
  if (typeof a !== 'object' || a === null || typeof b !== 'object' || b === null) return false
  const keysA = Object.keys(a)
  const keysB = Object.keys(b)
  if (keysA.length !== keysB.length) return false
  for (const key of keysA) {
    if (!Object.prototype.hasOwnProperty.call(b, key)) return false
    if (!areFormValuesEqual((a as Record<string, unknown>)[key], (b as Record<string, unknown>)[key])) {
      return false
    }
  }
  return true
}

export function useForm<TValues extends Record<string, unknown>>(
  options: UseFormOptions<TValues>,
): FormContext<TValues> {
  const initial = cloneFormValue(options.initialValues)
  const values = reactive<TValues>(cloneFormValue(options.initialValues)) as TValues
  const errors = ref<Partial<Record<keyof TValues, string>>>({}) as Ref<
    Partial<Record<keyof TValues, string>>
  >
  const touched = ref<Partial<Record<keyof TValues, boolean>>>({}) as Ref<
    Partial<Record<keyof TValues, boolean>>
  >
  const isSubmitting = ref(false)

  const isDirty = computed(() => {
    return !areFormValuesEqual(values, initial)
  })

  const isValid = computed(() => {
    return Object.values(errors.value).every((err) => !err)
  })

  function setFieldValue<K extends keyof TValues>(field: K, value: TValues[K]) {
    values[field] = value
    touched.value[field] = true
    if (options.validateOnChange !== false && options.schema) {
      void validateField(field)
    }
  }

  function setFieldError<K extends keyof TValues>(field: K, message: string) {
    errors.value[field] = message
  }

  function clearErrors(field?: keyof TValues) {
    if (field) {
      delete errors.value[field]
    } else {
      errors.value = {}
    }
  }

  async function validate(): Promise<boolean> {
    if (!options.schema) {
      return true
    }
    const detected = await extractIssues(options.schema, values)
    errors.value = detected as Partial<Record<keyof TValues, string>>
    return Object.keys(detected).length === 0
  }

  async function validateField<K extends keyof TValues>(field: K): Promise<boolean> {
    if (!options.schema) {
      return true
    }
    const detected = await extractIssues(options.schema, values)
    const fieldKey = String(field)
    if (detected[fieldKey]) {
      errors.value[field] = detected[fieldKey]
      return false
    } else {
      delete errors.value[field]
      return true
    }
  }

  function handleSubmit(
    onValid: (values: TValues) => Promise<void> | void,
    onInvalid?: (errors: Partial<Record<keyof TValues, string>>) => void,
  ) {
    return async (e?: Event) => {
      if (e && typeof e.preventDefault === 'function') {
        e.preventDefault()
      }
      isSubmitting.value = true
      try {
        const valid = await validate()
        if (valid) {
          await onValid(values)
        } else if (onInvalid) {
          onInvalid(errors.value)
        }
      } finally {
        isSubmitting.value = false
      }
    }
  }

  function resetForm(nextValues?: Partial<TValues>) {
    const target = nextValues ? { ...initial, ...nextValues } : initial
    Object.assign(values, cloneFormValue(target))
    errors.value = {}
    touched.value = {}
    isSubmitting.value = false
  }

  const context: FormContext<TValues> = {
    values,
    errors,
    touched,
    isSubmitting,
    isDirty,
    isValid,
    setFieldValue,
    setFieldError,
    clearErrors,
    validateField,
    validate,
    handleSubmit,
    resetForm,
  }

  if (getCurrentInstance()) {
    provide(FORM_CONTEXT_KEY, context as FormContext<Record<string, unknown>>)
  }

  return context
}
