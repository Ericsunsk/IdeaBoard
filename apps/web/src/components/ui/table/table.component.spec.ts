import { describe, expect, it, vi } from 'vitest'
import { h } from 'vue'
import { render, screen, userEvent, within } from '../../../test/render'
import Table from './Table.vue'
import TableBody from './TableBody.vue'
import TableCaption from './TableCaption.vue'
import TableCell from './TableCell.vue'
import TableFooter from './TableFooter.vue'
import TableHead from './TableHead.vue'
import TableHeader from './TableHeader.vue'
import TableRow from './TableRow.vue'

/**
 * Quota: final/table-semantics. The primitive owns native table semantics
 * (table/columnheader/row/cell), the opt-in keyboard reachability of rows and
 * cells, and the sortable-header affordance (a real button carrying
 * `aria-sort`). These specs pin all three through the wrapper components.
 */
function renderOrdersTable(sortDirection: 'ascending' | 'descending' | 'none' = 'ascending') {
  const onSort = vi.fn()
  render(Table, {
    slots: {
      default: () => [
        h(TableCaption, () => 'Recent orders'),
        h(TableHeader, () => [
          h(TableRow, () => [
            h(TableHead, () => 'Order'),
            h(TableHead, { isSortable: true, sortDirection, onSort }, () => 'Total'),
          ]),
        ]),
        h(TableBody, () => [
          h(TableRow, { tabindex: 0, 'aria-label': 'Order 1' }, () => [
            h(TableCell, () => '#1'),
            h(TableCell, () => '$9.00'),
          ]),
        ]),
        h(TableFooter, () => [
          h(TableRow, () => [h(TableCell, () => 'Total'), h(TableCell, () => '$9.00')]),
        ]),
      ],
    },
  })
  return { onSort }
}

describe('Table primitive', () => {
  it('renders native table semantics with caption, headers, rows and cells', () => {
    renderOrdersTable()

    expect(screen.getByRole('table')).toBeInTheDocument()
    expect(document.querySelector('table caption')).toHaveTextContent('Recent orders')
    expect(screen.getAllByRole('columnheader')).toHaveLength(2)
    expect(screen.getAllByRole('row')).toHaveLength(3)
    expect(screen.getAllByRole('cell')).toHaveLength(4)
  })

  it('marks column headers with column scope', () => {
    renderOrdersTable()

    for (const header of screen.getAllByRole('columnheader')) {
      expect(header.tagName).toBe('TH')
      expect(header).toHaveAttribute('scope', 'col')
    }
  })

  it('exposes the sortable header as a button carrying aria-sort', () => {
    renderOrdersTable('descending')

    const header = screen.getByRole('columnheader', { name: /total/i })
    expect(header).toHaveAttribute('aria-sort', 'descending')
    const button = within(header).getByRole('button', { name: /total/i })
    expect(button.tagName).toBe('BUTTON')
    expect(button).toHaveAttribute('type', 'button')
  })

  it('keeps plain headers free of sort affordances', () => {
    renderOrdersTable()

    const header = screen.getByRole('columnheader', { name: 'Order' })
    expect(header).not.toHaveAttribute('aria-sort')
    expect(within(header).queryByRole('button')).not.toBeInTheDocument()
  })

  it('activates sorting from the keyboard alone', async () => {
    const user = userEvent.setup()
    const { onSort } = renderOrdersTable()

    const button = within(screen.getByRole('columnheader', { name: /total/i })).getByRole(
      'button',
      { name: /total/i },
    )
    button.focus()
    expect(document.activeElement).toBe(button)

    await user.keyboard('{Enter}')
    expect(onSort).toHaveBeenCalledTimes(1)
  })

  it('makes rows with an explicit tabindex keyboard-reachable', () => {
    renderOrdersTable()

    const row = screen.getByRole('row', { name: /order 1/i })
    expect(row.tagName).toBe('TR')
    expect((row as HTMLTableRowElement).tabIndex).toBe(0)
    ;(row as HTMLTableRowElement).focus()
    expect(document.activeElement).toBe(row)
  })

  it('leaves plain cells out of the tab order', () => {
    renderOrdersTable()

    for (const cell of screen.getAllByRole('cell')) {
      expect(cell.tagName).toBe('TD')
      expect(cell).not.toHaveAttribute('tabindex')
    }
  })
})
