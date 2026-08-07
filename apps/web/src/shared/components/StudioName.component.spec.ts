import { describe, expect, it } from 'vitest'
import StudioName from './StudioName.vue'
import { render } from '../../test/render'

describe('StudioName', () => {
  it('renders the studio name', () => {
    const view = render(StudioName)

    expect(view.getByText('Eric Studio')).toBeVisible()
    expect(view.container.querySelector('img')).not.toBeNull()
  })
})
