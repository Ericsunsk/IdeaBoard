import { describe, expect, it } from 'vitest'
import { h } from 'vue'
import { fireEvent } from '@testing-library/vue'
import { renderWithUser, screen, within } from '@/test/render'
import AdminColumn from '../AdminColumn.vue'
import AdminTable from './AdminTable.vue'

type Row = { id: string; name: string }

const rows: Row[] = [
  { id: 'a', name: 'Alpha' },
  { id: 'b', name: 'Beta' },
]

function renderTable(props: Record<string, unknown> = {}, slots: Record<string, unknown> = {}) {
  return renderWithUser(
    h(AdminTable as unknown as Parameters<typeof h>[0], props, {
      default: () => [
        h(AdminColumn as unknown as Parameters<typeof h>[0], { field: 'name', header: '名称' }),
      ],
      ...slots,
    }),
  )
}

describe('AdminTable', () => {
  it('renders headers as columnheaders and body cells', async () => {
    renderTable({ value: rows })

    expect(await screen.findByRole('columnheader', { name: '名称' })).toBeVisible()
    expect(screen.getByText('Alpha')).toBeVisible()
    expect(screen.getByText('Beta')).toBeVisible()
  })

  it('shows the empty state with role status when there are no rows', async () => {
    renderTable({ value: [], emptyTitle: '暂无数据' })

    expect(await screen.findByRole('status')).toHaveTextContent('暂无数据')
  })

  it('shows a loading spinner while keeping the table mounted', async () => {
    const { container } = renderTable({ value: [], loading: true })

    expect(await screen.findByRole('progressbar')).toBeVisible()
    expect(container.querySelector('table')).not.toBeNull()
  })

  it('supports select-all and per-row selection through v-model', async () => {
    const { user, emitted } = renderWithUser(
      h(
        AdminTable as unknown as Parameters<typeof h>[0],
        { value: rows, dataKey: 'id' },
        {
          default: () => [
            h(AdminColumn as unknown as Parameters<typeof h>[0], { selectionMode: 'multiple' }),
            h(AdminColumn as unknown as Parameters<typeof h>[0], { field: 'name', header: '名称' }),
          ],
        },
      ),
    )

    await user.click(await screen.findByRole('checkbox', { name: '选择全部行' }))
    expect(emitted()['update:selection']?.[0]).toEqual([rows])

    const second = renderWithUser(
      h(
        AdminTable as unknown as Parameters<typeof h>[0],
        { value: rows, dataKey: 'id', selection: rows },
        {
          default: () => [
            h(AdminColumn as unknown as Parameters<typeof h>[0], { selectionMode: 'multiple' }),
            h(AdminColumn as unknown as Parameters<typeof h>[0], { field: 'name', header: '名称' }),
          ],
        },
      ),
    )
    const boxes = await within(second.container as HTMLElement).findAllByRole('checkbox', {
      name: /选择行/,
    })
    expect(boxes).toHaveLength(2)
    expect((boxes[0] as HTMLInputElement).checked).toBe(true)
    expect((boxes[1] as HTMLInputElement).checked).toBe(true)
    await second.user.click(boxes[0]!)
    expect((boxes[0] as HTMLInputElement).checked).toBe(false)
  })

  it('toggles expansion rows and emits row-expand with the row data', async () => {
    // Bare `expander` attributes compile to empty-string vnode props.
    const expanderProp = '' as unknown as boolean
    const { user, emitted } = renderWithUser(
      h(
        AdminTable as unknown as Parameters<typeof h>[0],
        { value: rows, dataKey: 'id', expandedRows: {} },
        {
          default: () => [
            h(AdminColumn as unknown as Parameters<typeof h>[0], { expander: expanderProp }),
            h(AdminColumn as unknown as Parameters<typeof h>[0], { field: 'name', header: '名称' }),
          ],
          expansion: (slotProps: unknown) =>
            h('div', `详情 ${(slotProps as { data: Row }).data.name}`),
        },
      ),
    )

    expect(screen.queryByText('详情 Alpha')).not.toBeInTheDocument()
    const expanders = await screen.findAllByRole('button', { name: '展开详情' })
    await user.click(expanders[0]!)
    const emittedEvents = emitted() as Record<string, Array<Array<unknown>>>
    expect(emittedEvents['update:expandedRows']?.[0]?.[0]).toEqual({ a: true })
    expect(emittedEvents['row-expand']?.[0]?.[0]).toEqual({ data: rows[0], index: 0 })

    renderWithUser(
      h(
        AdminTable as unknown as Parameters<typeof h>[0],
        { value: rows, dataKey: 'id', expandedRows: { a: true } },
        {
          default: () => [
            h(AdminColumn as unknown as Parameters<typeof h>[0], { expander: true }),
            h(AdminColumn as unknown as Parameters<typeof h>[0], { field: 'name', header: '名称' }),
          ],
          expansion: (slotProps: unknown) =>
            h('div', `详情 ${(slotProps as { data: Row }).data.name}`),
        },
      ),
    )
    expect(await screen.findByText('详情 Alpha')).toBeVisible()
    expect(screen.getByRole('button', { name: '收起详情' })).toBeVisible()
  })

  it('renders directly composed shadcn atomic table components', async () => {
    const {
      AdminTableHeader,
      AdminTableHead,
      AdminTableBody,
      AdminTableRow,
      AdminTableCell,
      AdminTableFooter,
    } = await import('./adminTableAtoms')

    renderWithUser(
      h(
        AdminTable,
        {},
        {
          default: () => [
            h(AdminTableHeader, {}, () => [
              h(AdminTableRow, {}, () => [h(AdminTableHead, {}, () => '标题列')]),
            ]),
            h(AdminTableBody, {}, () => [
              h(AdminTableRow, {}, () => [h(AdminTableCell, {}, () => '内容格')]),
            ]),
            h(AdminTableFooter, {}, () => [
              h(AdminTableRow, {}, () => [h(AdminTableCell, {}, () => '汇总格')]),
            ]),
          ],
        },
      ),
    )

    expect(await screen.findByRole('columnheader', { name: '标题列' })).toBeVisible()
    expect(screen.getByText('内容格')).toBeVisible()
    expect(screen.getByText('汇总格')).toBeVisible()
  })

  it('treats select-all as current-page only when selection holds other rows', async () => {
    const otherRow = { id: 'c', name: 'Gamma' }
    const renderSelectable = (selection: Row[]) =>
      renderWithUser(
        h(
          AdminTable as unknown as Parameters<typeof h>[0],
          { value: rows, dataKey: 'id', selection },
          {
            default: () => [
              h(AdminColumn as unknown as Parameters<typeof h>[0], { selectionMode: 'multiple' }),
              h(AdminColumn as unknown as Parameters<typeof h>[0], {
                field: 'name',
                header: '名称',
              }),
            ],
          },
        ),
      )

    // 本页全选 + 他页残留：表头仍勾选
    const full = renderSelectable([...rows, otherRow])
    const fullBoxes = await within(full.container as HTMLElement).findAllByRole('checkbox', {
      name: /选择/,
    })
    expect(fullBoxes).toHaveLength(3)
    expect((fullBoxes[0] as HTMLInputElement).checked).toBe(true)

    // 仅他页残留：表头与本页行都不勾选（查询均限定在各自容器内，互不干扰）
    const partial = renderSelectable([otherRow])
    const partialBoxes = await within(partial.container as HTMLElement).findAllByRole('checkbox', {
      name: /选择/,
    })
    expect(partialBoxes).toHaveLength(3)
    for (const box of partialBoxes) {
      expect((box as HTMLInputElement).checked).toBe(false)
    }
  })

  it('keeps the header static by default and sticks it when stickyHeader is on', async () => {
    const plain = renderTable({ value: rows })
    expect(plain.container.querySelector('thead')?.className ?? '').not.toContain('sticky')

    const sticky = renderTable({ value: rows, stickyHeader: true })
    const theadClass = sticky.container.querySelector('thead')?.className ?? ''
    for (const token of ['sticky', 'top-0', 'z-admin-sticky', 'bg-muted/80', 'backdrop-blur-xs']) {
      expect(theadClass).toContain(token)
    }
  })

  it('emits row-click for enabled rows and locks disabled rows', async () => {
    const { user, emitted, container } = renderWithUser(
      h(
        AdminTable as unknown as Parameters<typeof h>[0],
        { value: rows, dataKey: 'id', rowDisabled: (row: Row) => row.id === 'b' },
        {
          default: () => [
            h(AdminColumn as unknown as Parameters<typeof h>[0], { field: 'name', header: '名称' }),
          ],
        },
      ),
    )
    const readClicks = () =>
      ((emitted() as Record<string, Array<Array<unknown>>>)['row-click'] ?? []).map(
        (args) => args[0] as { data: Row; index: number },
      )

    const bodyRows = container.querySelectorAll('tbody tr')
    expect(bodyRows).toHaveLength(2)

    await user.click(bodyRows[0] as HTMLElement)
    expect(readClicks()).toHaveLength(1)
    expect(readClicks()[0]).toMatchObject({ data: rows[0], index: 0 })

    await user.click(bodyRows[1] as HTMLElement)
    expect(readClicks()).toHaveLength(1)
    expect(bodyRows[1]?.getAttribute('aria-disabled')).toBe('true')
    expect(bodyRows[0]?.hasAttribute('aria-disabled')).toBe(false)
    expect(bodyRows[1]?.className ?? '').not.toContain('hover:bg-muted/50')
    expect(bodyRows[0]?.className ?? '').toContain('hover:bg-muted/50')
  })

  it('marks selected rows with data-state so the shared selected token applies', async () => {
    const { container } = renderWithUser(
      h(
        AdminTable as unknown as Parameters<typeof h>[0],
        { value: rows, dataKey: 'id', selection: [rows[0]] },
        {
          default: () => [
            h(AdminColumn as unknown as Parameters<typeof h>[0], { selectionMode: 'multiple' }),
            h(AdminColumn as unknown as Parameters<typeof h>[0], { field: 'name', header: '名称' }),
          ],
        },
      ),
    )

    const bodyRows = container.querySelectorAll('tbody tr')
    expect(bodyRows[0]?.getAttribute('data-state')).toBe('selected')
    expect(bodyRows[0]?.getAttribute('aria-selected')).toBe('true')
    expect(bodyRows[1]?.hasAttribute('data-state')).toBe(false)
  })

  it('excludes disabled rows from select-all and per-row toggles', async () => {
    const { user, emitted, container } = renderWithUser(
      h(
        AdminTable as unknown as Parameters<typeof h>[0],
        { value: rows, dataKey: 'id', rowDisabled: (row: Row) => row.id === 'b' },
        {
          default: () => [
            h(AdminColumn as unknown as Parameters<typeof h>[0], { selectionMode: 'multiple' }),
            h(AdminColumn as unknown as Parameters<typeof h>[0], { field: 'name', header: '名称' }),
          ],
        },
      ),
    )
    const readSelections = () =>
      (emitted() as Record<string, Array<Array<unknown>>>)['update:selection'] ?? []

    const boxes = within(container as HTMLElement).getAllByRole('checkbox')
    expect(boxes).toHaveLength(3)
    expect((boxes[2] as HTMLInputElement).disabled).toBe(true)

    await user.click(boxes[0] as HTMLElement)
    expect(readSelections()[0]?.[0]).toEqual([rows[0]])

    // 禁用行 checkbox 即使收到 change 也不得进入 selection。
    await fireEvent.change(boxes[2] as HTMLElement, { target: { checked: true } })
    expect(readSelections()).toHaveLength(1)
  })

  it('keeps header and row checkboxes on one grade with indeterminate intact', async () => {
    const { container } = renderWithUser(
      h(
        AdminTable as unknown as Parameters<typeof h>[0],
        { value: rows, dataKey: 'id', selection: [rows[0]] },
        {
          default: () => [
            h(AdminColumn as unknown as Parameters<typeof h>[0], { selectionMode: 'multiple' }),
            h(AdminColumn as unknown as Parameters<typeof h>[0], { field: 'name', header: '名称' }),
          ],
        },
      ),
    )

    const boxes = within(container as HTMLElement).getAllByRole('checkbox') as HTMLInputElement[]
    expect(boxes).toHaveLength(3)
    for (const box of boxes) {
      expect(box.className).toContain('size-4')
      expect(box.className).toContain('accent-primary')
    }
    expect(boxes[0]?.indeterminate).toBe(true)
  })

  it('keeps rows mounted with a weak stale hint on background refresh', async () => {
    const { container } = renderTable({ value: rows, loading: true })

    const table = container.querySelector('table')
    expect(table?.getAttribute('data-stale')).toBe('true')
    expect(table?.getAttribute('aria-busy')).toBe('true')
    expect(within(container as HTMLElement).getByText('Alpha')).toBeVisible()
    expect(container.querySelector('[role="progressbar"]')).toBeNull()
    expect(container.querySelector('tbody tr')?.className ?? '').toContain('opacity-70')
  })

  it('renders skeleton rows instead of the spinner when skeletonRows is set', async () => {
    const { container } = renderTable({ value: [], loading: true, skeletonRows: 3 })

    expect(container.querySelectorAll('[data-ui="skeleton"]')).toHaveLength(3)
    expect(container.querySelector('[role="progressbar"]')).toBeNull()
    expect(within(container as HTMLElement).getByText('正在加载')).toBeInTheDocument()
  })
})
