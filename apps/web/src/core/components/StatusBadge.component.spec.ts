import { describe, expect, it } from 'vitest'
import { render, screen } from '../../test/render'
import StatusBadge, { type AppStatusTone } from './StatusBadge.vue'

// Tone pins mirror the operational status contract: Badge carrier
// (rounded-full/inline-flex/border) + canonical semantic tone + dot color.
// Pulse breathes by default only on success/warning (processing signal).
const tonePins: Array<{
  tone: AppStatusTone
  classes: string[]
  dot: string
  pulseByDefault: boolean
}> = [
  {
    tone: 'success',
    classes: ['bg-success-soft', 'text-success', 'border-success/20', 'shadow-xs'],
    dot: 'bg-success',
    pulseByDefault: true,
  },
  {
    tone: 'danger',
    classes: ['bg-destructive-soft', 'text-destructive', 'border-destructive/20', 'shadow-xs'],
    dot: 'bg-destructive',
    pulseByDefault: false,
  },
  {
    tone: 'warning',
    classes: ['bg-warning-soft', 'text-warning', 'border-warning/20', 'shadow-xs'],
    dot: 'bg-warning',
    pulseByDefault: true,
  },
  {
    tone: 'neutral',
    classes: ['bg-muted', 'text-muted-foreground', 'border-border'],
    dot: 'bg-muted-foreground',
    pulseByDefault: false,
  },
  {
    tone: 'info',
    classes: ['bg-info-soft', 'text-info', 'border-info/20', 'shadow-xs'],
    dot: 'bg-info',
    pulseByDefault: false,
  },
]

describe('StatusBadge', () => {
  it.each(tonePins)('renders $tone on the Badge carrier with tone $classes', (pin) => {
    render(StatusBadge, { props: { label: `status-${pin.tone}`, tone: pin.tone } })

    const label = screen.getByText(`status-${pin.tone}`)
    const root = label.parentElement!
    expect(root).toHaveClass('inline-flex', 'rounded-full', 'border')
    for (const cls of pin.classes) {
      expect(root).toHaveClass(cls)
    }
    const dot = root.querySelector('[aria-hidden="true"] span:last-child')!
    expect(dot).toHaveClass(pin.dot)
    expect(root.querySelector('.animate-ping') !== null).toBe(pin.pulseByDefault)
  })

  it('lets pulse prop override the default', async () => {
    const { container, rerender } = render(StatusBadge, {
      props: { label: 'forced', tone: 'neutral', pulse: true },
    })
    expect(container.querySelector('.animate-ping')).not.toBeNull()

    await rerender({ label: 'forced', tone: 'success', pulse: false })
    expect(container.querySelector('.animate-ping')).toBeNull()
  })

  it('applies size classes without changing the label API', async () => {
    const { rerender } = render(StatusBadge, {
      props: { label: 'small-status', tone: 'info', size: 'small' },
    })
    const smallRoot = screen.getByText('small-status').parentElement!
    expect(smallRoot).toHaveClass('h-5', 'px-2', 'text-admin-caption')

    await rerender({ label: 'medium-status', tone: 'info', size: 'medium' })
    const mediumRoot = screen.getByText('medium-status').parentElement!
    expect(mediumRoot).toHaveClass('h-6', 'px-2.5', 'text-xs')
  })

  it('renders slot content and supports dotless display', () => {
    const { container } = render(StatusBadge, {
      props: { tone: 'info', dot: false },
      slots: { default: 'slot-label' },
    })
    expect(screen.getByText('slot-label')).toBeVisible()
    expect(container.querySelector('[aria-hidden="true"]')).toBeNull()
  })

  it('forwards caller classes after the status chrome', () => {
    render(StatusBadge, {
      props: { label: 'extra', tone: 'neutral', class: 'w-full font-bold' },
    })
    const classes = screen.getByText('extra').parentElement!.className.split(' ')
    expect(classes).toContain('w-full')
    expect(classes).toContain('font-bold')
  })
})
