import { describe, expect, it } from 'vitest'
import { render, screen } from '../../../../test/render'
import {
  adminMasonryGridClass,
  adminSurfacePanelClass,
  adminSurfacePanelViewportClass,
  adminSurfaceRootViewportClass,
  adminSurfaceScrollContentClass,
  adminSurfaceToolbarClass,
} from '../lib/adminUiClasses'
import AdminManagementFrame from './AdminManagementFrame.vue'
import AdminResourceGridFrame from './AdminResourceGridFrame.vue'
import AdminTablePanel from './AdminTablePanel.vue'

describe('Admin surface frame adapters', () => {
  it('keeps Management content in the shared scroll viewport', () => {
    render(AdminManagementFrame, {
      slots: {
        'toolbar-start': 'Toolbar start',
        'toolbar-end': 'Toolbar end',
        default: 'Management content',
      },
    })

    const content = screen.getByText('Management content')
    const panel = content.parentElement
    const root = panel?.parentElement
    const toolbar = screen.getByText('Toolbar start').parentElement

    expectClassContract(root, adminSurfaceRootViewportClass)
    expectClassContract(panel, adminSurfacePanelClass)
    expectClassContract(panel, adminSurfacePanelViewportClass)
    expectClassContract(content, adminSurfaceScrollContentClass)
    expect(content).toHaveClass('px-6', 'pb-6')
    expectClassContract(toolbar, adminSurfaceToolbarClass)
    expect(screen.getByText('Toolbar end')).toBeInTheDocument()
  })

  it('keeps ordinary tables framed without opting into viewport height', () => {
    render(AdminTablePanel, {
      slots: {
        toolbar: '<div>Table toolbar</div>',
        default: 'Table content',
      },
    })

    const tableFrame = screen.getByText('Table content')
    const content = tableFrame.parentElement
    const panel = content?.parentElement
    const root = panel?.parentElement

    expect(root).toHaveClass('min-w-0')
    expectClassContract(panel, adminSurfacePanelClass)
    expect(panel).not.toHaveClass(...classTokens(adminSurfacePanelViewportClass))
    expect(content).toHaveClass('px-6', 'pb-6')
    expect(tableFrame).toHaveClass('min-w-0', 'overflow-hidden', 'rounded-lg')
    expect(tableFrame).not.toHaveClass('h-full')
    expect(screen.getByText('Table toolbar')).toBeInTheDocument()
  })

  it('keeps the Users table flex viewport and overflow contract', () => {
    const contentClass = 'min-h-0 flex-1 overflow-hidden px-6 pb-6'
    render(AdminTablePanel, {
      props: { contentClass, viewportHeight: true },
      slots: { default: 'Users table content' },
    })

    const tableFrame = screen.getByText('Users table content')
    const content = tableFrame.parentElement
    const panel = content?.parentElement
    const root = panel?.parentElement

    expectClassContract(root, adminSurfaceRootViewportClass)
    expectClassContract(panel, adminSurfacePanelViewportClass)
    expectClassContract(content, contentClass)
    expect(content).not.toHaveClass(...classTokens(adminSurfaceScrollContentClass))
    expect(tableFrame).toHaveClass('h-full')
  })

  it('keeps Resource Grid on the Management adapter interface', () => {
    render(AdminResourceGridFrame, {
      props: {
        emptyDescription: 'No resources yet',
        emptyTitle: 'No resources',
        isEmpty: false,
        isLoading: false,
      },
      slots: {
        'toolbar-start': 'Grid toolbar start',
        'toolbar-end': 'Grid toolbar end',
        default: 'Grid item',
      },
    })

    const grid = screen.getByText('Grid item')
    const content = grid.parentElement

    expectClassContract(grid, adminMasonryGridClass)
    expectClassContract(content, adminSurfaceScrollContentClass)
    expect(screen.getByText('Grid toolbar start')).toBeInTheDocument()
    expect(screen.getByText('Grid toolbar end')).toBeInTheDocument()
  })
})

function classTokens(className: string) {
  return className.split(' ').filter(Boolean)
}

function expectClassContract(element: Element | null | undefined, className: string) {
  expect(element).not.toBeNull()
  expect(element).toHaveClass(...classTokens(className))
}
