import { defineComponent } from 'vue'
import { renderWithUser } from '../../../../test/render'
import UsagePane from './UsagePane.vue'

// Shared usage-pane render seam (converged from `UsagePane.component.spec.ts`):
// host stubs plus the single render entry. The `useUsage` mock, its mutable
// state, and the toast mock stay in the spec entry (`vi.mock` must register
// from the entry file); this module only renders through them.
const ButtonStub = defineComponent({
  template: '<button type="button"><slot /></button>',
})

const chartStub = defineComponent({
  props: {
    data: { type: Object, required: true },
    options: { type: Object, required: true },
    canvasProps: { type: Object, required: true },
  },
  template:
    '<div data-testid="usage-trend-chart" :data-points="String(data.datasets[0].data.length)" :data-datasets="String(data.datasets.length)"><canvas v-bind="canvasProps" /></div>',
})

export function renderUsagePane() {
  return renderWithUser(UsagePane, {
    props: { active: true },
    global: { stubs: { Button: ButtonStub, ChartRenderer: chartStub } },
  })
}
