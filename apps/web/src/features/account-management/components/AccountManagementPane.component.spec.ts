import { describe, expect, it } from 'vitest'
import { renderWithUser, screen } from '../../../test/render'
import AccountManagementPane from './AccountManagementPane.vue'

describe('AccountManagementPane', () => {
  it('associates the named page region with its heading and renders the page content', () => {
    renderWithUser(AccountManagementPane, {
      props: { title: '账户测试页' },
      slots: { default: '<p>页面内容</p>' },
    })

    expect(screen.getByRole('heading', { name: '账户测试页', level: 2 })).toBeVisible()
    expect(screen.getByRole('region', { name: '账户测试页' })).toHaveTextContent('页面内容')
  })
})
