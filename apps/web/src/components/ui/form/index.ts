export { default as Form } from './Form.vue'
export { default as FormField } from './FormField.vue'
export { default as FormItem } from './FormItem.vue'
export { default as FormLabel } from './FormLabel.vue'
export { default as FormControl } from './FormControl.vue'
export { default as FormDescription } from './FormDescription.vue'
export { default as FormMessage } from './FormMessage.vue'
export { useForm, firstFormError, type UseFormOptions, type FormSchema } from './useForm'
export {
  useFormField,
  type FormContext,
  type FormItemContext,
  type FormFieldContext,
} from './context'

// User-shell and Admin adapters consume the neutral contract from
// `@/core/components/formFieldContract`; this package owns only the
// reka-ui form primitives and must not import feature-specific tokens.
