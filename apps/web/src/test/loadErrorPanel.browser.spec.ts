import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/vue'
import { afterEach, describe, expect, it } from 'vitest'
import { renderWithUser, screen } from './render'
import '../app/style.css'
import LoadErrorPanel from '../core/components/LoadErrorPanel.vue'

afterEach(() => {
  cleanup()
  document.documentElement.classList.remove('light')
})

describe.each(['dark', 'light'])('LoadErrorPanel in %s mode', (mode) => {
  it('uses the destructive surface and a compact primary retry action', () => {
    document.documentElement.classList.toggle('light', mode === 'light')
    renderWithUser(LoadErrorPanel, {
      props: { message: 'Load failed', retryLabel: 'Retry' },
    })
    const alert = screen.getByRole('alert')
    const style = getComputedStyle(alert)
    const probe = document.createElement('div')
    probe.style.cssText =
      'border-color: color-mix(in oklab, var(--destructive) 30%, transparent); background: color-mix(in oklab, var(--destructive) 10%, transparent);'
    alert.append(probe)
    expect(style.borderColor).toBe(getComputedStyle(probe).borderColor)
    expect(style.backgroundColor).toBe(getComputedStyle(probe).backgroundColor)
    expect(style.borderWidth).toBe('1px')
    expect(style.padding).toBe('16px')
    expect(style.alignItems).toBe('flex-start')
    expect(style.textAlign).toBe('left')
    const retry = screen.getByRole('button', { name: 'Retry' })
    expect(retry.getBoundingClientRect().height).toBe(32)
    probe.style.background = 'var(--primary)'
    expect(getComputedStyle(retry).backgroundColor).toBe(getComputedStyle(probe).backgroundColor)
  })

  it('centers the optional ghost retry treatment', () => {
    document.documentElement.classList.toggle('light', mode === 'light')
    renderWithUser(LoadErrorPanel, {
      props: {
        message: 'Load failed',
        retryLabel: 'Retry',
        align: 'center',
        retryVariant: 'ghost',
      },
    })
    const style = getComputedStyle(screen.getByRole('alert'))
    expect(style.alignItems).toBe('center')
    expect(style.textAlign).toBe('center')
    const retry = screen.getByRole('button', { name: 'Retry' })
    expect(getComputedStyle(retry).backgroundColor).toBe('rgba(0, 0, 0, 0)')
    expect(retry.getBoundingClientRect().height).toBe(32)
  })
})
