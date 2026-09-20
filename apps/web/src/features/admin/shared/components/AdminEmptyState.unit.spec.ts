import { describe, expect, it } from 'vitest'
import { render, screen } from '../../../../test/render'
import AdminEmptyState from './AdminEmptyState.vue'

describe('AdminEmptyState', () => {
  it('renders default empty message and status role', () => {
    render(AdminEmptyState)
    expect(screen.getByRole('status')).toBeInTheDocument()
    expect(screen.getByText('暂无数据')).toBeInTheDocument()
  })

  it('renders customized title and description', () => {
    render(AdminEmptyState, {
      props: {
        title: '未找到匹配用户',
        description: '请尝试更换搜索关键词或重置筛选条件',
      },
    })
    expect(screen.getByText('未找到匹配用户')).toBeInTheDocument()
    expect(screen.getByText('请尝试更换搜索关键词或重置筛选条件')).toBeInTheDocument()
  })
})
