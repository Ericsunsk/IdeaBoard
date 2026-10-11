import { describe, expect, it } from 'vitest'
import { h } from 'vue'
import { renderWithUser, screen } from '../../../../../test/render'
import { AdminTabs, AdminTabsContent, AdminTabsList, AdminTabsTrigger } from './adminTabsAtoms'

describe('AdminTabs', () => {
  it('renders tab triggers and active content, and switches tabs on click', async () => {
    const { user } = renderWithUser(
      h(
        AdminTabs,
        { defaultValue: 'tab1' },
        {
          default: () => [
            h(AdminTabsList, null, {
              default: () => [
                h(AdminTabsTrigger, { value: 'tab1' }, { default: () => '概览' }),
                h(AdminTabsTrigger, { value: 'tab2' }, { default: () => '详情' }),
              ],
            }),
            h(AdminTabsContent, { value: 'tab1' }, { default: () => '概览内容' }),
            h(AdminTabsContent, { value: 'tab2' }, { default: () => '详情内容' }),
          ],
        },
      ),
    )

    expect(await screen.findByText('概览内容')).toBeVisible()
    expect(screen.queryByText('详情内容')).not.toBeInTheDocument()

    const trigger2 = screen.getByRole('tab', { name: '详情' })
    await user.click(trigger2)

    expect(await screen.findByText('详情内容')).toBeVisible()
    expect(screen.queryByText('概览内容')).not.toBeInTheDocument()
  })
})
