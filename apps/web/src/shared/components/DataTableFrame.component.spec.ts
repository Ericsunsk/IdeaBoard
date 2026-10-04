import { defineComponent, h, ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen } from '../../test/render'
import DataTableFrame from './DataTableFrame.vue'
import DataTableColumn from './DataTableColumn.vue'

const rows = [{ id: 'row-1', name: '第一条记录' }]

describe('DataTableFrame', () => {
  it('uses the scroll container as the only accessible scroll surface', () => {
    const { container } = renderTable({ value: [], scrollable: true })

    const scrollRegion = screen.getByRole('region', { name: '记录表格' })
    const afterContent = screen.getByText('表格后置内容')

    expect(scrollRegion).toHaveClass('app-scrollbar', 'overflow-x-auto')
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
      slots: { default: () => h(DataTableColumn, { field: 'name', header: '名称' }) },
    })

    const tableContainer = screen.getByRole('table').parentElement

    expect(tableContainer).toHaveClass('app-scrollbar')
    expect(tableContainer).not.toHaveAttribute('role')
    expect(tableContainer).not.toHaveAttribute('aria-label')
    expect(tableContainer).not.toHaveAttribute('tabindex')
  })

  it('keeps loading feedback inside the table with aria-busy', () => {
    renderTable({ value: [], loading: true })

    const table = screen.getByRole('table', { name: '记录表格' })
    expect(table).toBeVisible()
    expect(table).toHaveAttribute('aria-busy', 'true')
    expect(screen.getByRole('status')).toBeVisible()
    expect(document.querySelector('svg')).not.toBeNull()
  })

  it('renders columns, row data, and forwards row-click', async () => {
    const onRowClick = vi.fn()
    const { user } = renderTable({ value: rows }, onRowClick)

    const headerCell = screen.getByRole('columnheader', { name: '名称' })
    const bodyCell = screen.getByRole('cell', { name: '第一条记录' })

    expect(headerCell).toBeVisible()
    expect(headerCell).toHaveClass('text-foreground')
    expect(bodyCell).toBeVisible()

    await user.click(bodyCell)
    expect(onRowClick).toHaveBeenCalledOnce()
    expect(onRowClick.mock.calls[0]?.[0]).toMatchObject({ data: rows[0], index: 0 })
  })

  it('renders scoped body slots with row data', () => {
    const BodyHarness = defineComponent({
      components: { DataTableFrame, DataTableColumn },
      setup() {
        return { rows }
      },
      template: `
        <DataTableFrame :value="rows" data-key="id">
          <DataTableColumn field="name" header="名称">
            <template #body="{ data }">记录-{{ data.name }}</template>
          </DataTableColumn>
        </DataTableFrame>
      `,
    })
    renderWithUser(BodyHarness)

    expect(screen.getByRole('cell', { name: '记录-第一条记录' })).toBeVisible()
  })

  it('keeps row identity stable through data-key', () => {
    const KeyHarness = defineComponent({
      components: { DataTableFrame, DataTableColumn },
      setup() {
        return { rows: ref([...rows]) }
      },
      template: `
        <DataTableFrame :value="rows" data-key="id">
          <DataTableColumn field="name" header="名称" />
        </DataTableFrame>
      `,
    })
    const { container } = renderWithUser(KeyHarness)

    const bodyRows = container.querySelectorAll('tbody tr')
    expect(bodyRows).toHaveLength(1)
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
      scrollAriaLabel: '记录表格',
      tableAriaLabel: '记录表格',
    },
    slots: {
      default: () => h(DataTableColumn, { field: 'name', header: '名称' }),
      after: () => h('div', '表格后置内容'),
    },
    attrs: { onRowClick },
  })
}
