<script lang="ts">
import { defineComponent } from 'vue'
import { useFormField } from './context'

export default defineComponent({
  name: 'FormControl',
  setup(_, { slots }) {
    const { formItemId, ariaDescribedby, invalid } = useFormField()

    return () => {
      const slotProps = {
        id: formItemId,
        'aria-describedby': ariaDescribedby.value,
        'aria-invalid': invalid.value ? 'true' : 'false',
      }
      return slots.default ? slots.default(slotProps) : null
    }
  },
})
</script>
