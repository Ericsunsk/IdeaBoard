import { describe, expect, it } from 'vitest'
import { render, screen } from '@/test/render'
import {
  adminSurfacePanelClass,
  adminSurfacePanelViewportClass,
  adminSurfaceRootViewportClass,
  adminSurfaceScrollContentClass,
} from '../../core/uiClasses'
import AdminSurfaceFrame from './AdminSurfaceFrame.vue'
import AdminTablePanel from '../table/AdminTablePanel.vue'

describe('Admin surface frame adapters', () => {
  it('keeps management content in the shared scroll viewport', () => {
    render(AdminSurfaceFrame, {
      props: {
        scrollContent: true,
        viewportHeight: true,
      },
      slots: {
        toolbar: 'Management toolbar',
        default: 'Management content',
      },
    })

    const content = screen.getByText('Management content')
    const panel = content.parentElement
    const root = panel?.parentElement

    expectClassContract(root, adminSurfaceRootViewportClass)
    expectClassContract(panel, adminSurfacePanelClass)
    expectClassContract(panel, adminSurfacePanelViewportClass)
    expectClassContract(content, adminSurfaceScrollContentClass)
    expect(content).toHaveClass('px-6', 'pb-6')
    expect(screen.getByText('Management toolbar')).toBeInTheDocument()
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
    expect(tableFrame).toHaveClass('min-w-0', 'overflow-hidden', 'rounded-panel')
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
})

function classTokens(className: string) {
  return className.split(' ').filter(Boolean)
}

function expectClassContract(element: Element | null | undefined, className: string) {
  expect(element).not.toBeNull()
  expect(element).toHaveClass(...classTokens(className))
}
