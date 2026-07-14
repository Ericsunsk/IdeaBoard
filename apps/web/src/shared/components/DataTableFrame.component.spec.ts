import PrimeVue from 'primevue/config'
import Column from 'primevue/column'
import { defineComponent, h, ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { primeVueTestOptions } from '../../test/primeVue'
import { renderWithUser, screen } from '../../test/render'
import DataTableFrame from './DataTableFrame.vue'

const rows = [{ id: 'row-1', name: '第一条记录' }]

describe('DataTableFrame', () => {
  it('uses the PrimeVue table container as the only accessible scroll surface', () => {
    const { container } = renderTable({ value: [], scrollable: true })

    const scrollRegion = screen.getByRole('region', { name: '记录表格' })
    const afterContent = screen.getByText('表格后置内容')

    expect(scrollRegion).toHaveClass('p-datatable-table-container', 'app-scrollbar')
    expect(scrollRegion).toHaveAttribute('tabindex', '0')
    expect(scrollRegion).toContainElement(screen.getByRole('table', { name: '记录表格' }))
    expect(scrollRegion).not.toContainElement(afterContent)
    expect(container).toContainElement(afterContent)
    expect(container.querySelectorAll('.app-scrollbar')).toHaveLength(1)
    expect(screen.getByText('暂无记录')).toBeVisible()
    expect(screen.getByText('记录会显示在这里')).toBeVisible()
  })

  it('does not focus an unlabeled table container', () => {
    renderWithUser(DataTableFrame, {
      props: { value: rows },
      slots: { default: () => h(Column, { field: 'name', header: '名称' }) },
      global: { plugins: [[PrimeVue, primeVueTestOptions]] },
    })

    const tableContainer = screen.getByRole('table').parentElement

    expect(tableContainer).toHaveClass('app-scrollbar')
    expect(tableContainer).not.toHaveAttribute('role')
    expect(tableContainer).not.toHaveAttribute('aria-label')
    expect(tableContainer).not.toHaveAttribute('tabindex')
  })

  it('keeps PrimeVue loading feedback inside the themed loading mask', () => {
    renderTable({ value: [], loading: true })

    expect(screen.getByRole('table', { name: '记录表格' })).toBeVisible()
    const loadingFeedback = screen.getByText('正在加载记录...')
    expect(loadingFeedback).toBeVisible()
    expect(loadingFeedback.parentElement).toHaveClass('p-datatable-mask', 'p-overlay-mask')
  })

  it('forwards columns, row data, and DataTable events in styled mode', async () => {
    const onRowClick = vi.fn()
    const { user } = renderTable({ value: rows }, onRowClick)

    expect(screen.getByRole('columnheader', { name: '名称' })).toHaveClass(
      'p-datatable-header-cell',
    )
    expect(screen.getByRole('cell', { name: '第一条记录' })).toBeVisible()

    await user.click(screen.getByRole('cell', { name: '第一条记录' }))
    expect(onRowClick).toHaveBeenCalledOnce()
  })

  it('preserves native PrimeVue selection state through the wrapper', async () => {
    const SelectionHarness = defineComponent({
      components: { DataTableFrame, Column },
      setup() {
        return {
          rows,
          selectedRows: ref<typeof rows>([]),
        }
      },
      template: `
        <DataTableFrame v-model:selection="selectedRows" :value="rows" data-key="id">
          <Column selection-mode="multiple" header-style="width: 3.5rem" />
          <Column field="name" header="名称" />
        </DataTableFrame>
        <output aria-label="选中数量">{{ selectedRows.length }}</output>
      `,
    })
    const { user } = renderWithUser(SelectionHarness, {
      global: { plugins: [[PrimeVue, primeVueTestOptions]] },
    })

    const rowCheckbox = screen.getAllByRole('checkbox')[1]!
    expect(rowCheckbox).toHaveClass('p-checkbox-input')

    await user.click(rowCheckbox)
    expect(rowCheckbox).toBeChecked()
    expect(screen.getByRole('status', { name: '选中数量' })).toHaveTextContent('1')

    await user.click(rowCheckbox)
    expect(screen.getByRole('status', { name: '选中数量' })).toHaveTextContent('0')
  })
})

function renderTable(
  props: { value: typeof rows; loading?: boolean; scrollable?: boolean },
  onRowClick = () => undefined,
) {
  return renderWithUser(DataTableFrame, {
    props: {
      ...props,
      dataKey: 'id',
      emptyTitle: '暂无记录',
      emptyDescription: '记录会显示在这里',
      loadingLabel: '正在加载记录...',
      scrollAriaLabel: '记录表格',
      tableAriaLabel: '记录表格',
    },
    slots: {
      default: () => h(Column, { field: 'name', header: '名称' }),
      after: () => h('div', '表格后置内容'),
    },
    attrs: { onRowClick },
    global: { plugins: [[PrimeVue, primeVueTestOptions]] },
  })
}
