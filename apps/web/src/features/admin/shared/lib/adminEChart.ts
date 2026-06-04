import { BarChart, LineChart, ScatterChart } from 'echarts/charts'
import {
  GraphicComponent,
  GridComponent,
  LegendComponent,
  TooltipComponent,
  VisualMapComponent,
} from 'echarts/components'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import VChart from 'vue-echarts'

use([
  CanvasRenderer,
  LineChart,
  BarChart,
  ScatterChart,
  GridComponent,
  LegendComponent,
  TooltipComponent,
  VisualMapComponent,
  GraphicComponent,
])

export { VChart }

