import { describe, expect, it } from 'vitest'
import { renderWithUser, screen } from '../../../../../test/render'
import AdminStatusPill, { type AdminStatusPillVariant } from './AdminStatusPill.vue'

// variant → Badge 语义色 pin：Badge 载体类（rounded-full/inline-flex）+
// admin tone 覆盖类（cn/tailwind-merge 保留后者，压住 badgeVariants 底色）。
const variantPins: Array<{
  variant: AdminStatusPillVariant
  tone: string[]
  dot: string
  pulseByDefault: boolean
}> = [
  {
    variant: 'success',
    tone: ['bg-success-soft', 'text-success', 'border-success/20', 'shadow-xs'],
    dot: 'bg-success',
    pulseByDefault: true,
  },
  {
    variant: 'danger',
    tone: ['bg-destructive-soft', 'text-destructive', 'border-destructive/20', 'shadow-xs'],
    dot: 'bg-destructive',
    pulseByDefault: false,
  },
  {
    variant: 'warning',
    tone: ['bg-warning-soft', 'text-warning', 'border-warning/20', 'shadow-xs'],
    dot: 'bg-warning',
    pulseByDefault: true,
  },
  {
    variant: 'neutral',
    tone: ['bg-muted', 'text-muted-foreground', 'border-border'],
    dot: 'bg-muted-foreground',
    pulseByDefault: false,
  },
  {
    variant: 'info',
    tone: ['bg-info-soft', 'text-info', 'border-info/20', 'shadow-xs'],
    dot: 'bg-info',
    pulseByDefault: false,
  },
]

describe('AdminStatusPill', () => {
  it.each(variantPins)('renders $variant on the Badge carrier with tone $tone', (pin) => {
    renderWithUser(AdminStatusPill, {
      props: { label: `pill-${pin.variant}`, variant: pin.variant },
    })

    const label = screen.getByText(`pill-${pin.variant}`)
    const root = label.parentElement!
    // shadcn Badge 载体：rounded 来自 Badge rounded 满圆，inline-flex/border 来自 badgeVariants。
    expect(root).toHaveClass('inline-flex', 'rounded-full', 'border')
    for (const cls of pin.tone) {
      expect(root).toHaveClass(cls)
    }
    // 圆点语义色保留。
    const dot = root.querySelector('[aria-hidden="true"] span:last-child')!
    expect(dot).toHaveClass(pin.dot)
    // 脉冲默认语义：仅 success/warning。
    expect(root.querySelector('.animate-ping') !== null).toBe(pin.pulseByDefault)
  })

  it('lets pulse prop override the default', async () => {
    const { container, rerender } = renderWithUser(AdminStatusPill, {
      props: { label: 'forced', variant: 'neutral', pulse: true },
    })
    expect(container.querySelector('.animate-ping')).not.toBeNull()

    await rerender({ label: 'forced', variant: 'success', pulse: false })
    expect(container.querySelector('.animate-ping')).toBeNull()
  })

  it('applies size classes without changing the label API', async () => {
    const { rerender } = renderWithUser(AdminStatusPill, {
      props: { label: 'small-pill', variant: 'info', size: 'small' },
    })
    expect(screen.getByText('small-pill').parentElement).toHaveClass(
      'h-5',
      'px-2',
      'text-admin-caption',
    )
    await rerender({ label: 'medium-pill', variant: 'info', size: 'medium' })
    expect(screen.getByText('medium-pill').parentElement).toHaveClass('h-6', 'px-2.5', 'text-xs')
  })

  it('forwards caller attributes to the same root through live prop updates', async () => {
    const { container, rerender } = renderWithUser(AdminStatusPill, {
      props: { label: 'ready', variant: 'success' },
      attrs: {
        class: 'w-full font-bold',
        style: 'opacity: 0.5; width: 120px',
        'aria-label': 'status',
      },
    })
    const root = screen.getByLabelText('status')
    expect(container.firstElementChild).toBe(root)
    expect(root).toHaveClass('w-full', 'font-bold')
    expect(root).toHaveStyle({ opacity: '0.5', width: '120px' })
    expect(root.children).toHaveLength(2)

    await rerender({ label: 'failed', variant: 'danger', pulse: false, size: 'small' })
    expect(screen.getByLabelText('status')).toBe(root)
    expect(root).toHaveTextContent('failed')
    expect(root).toHaveClass('bg-destructive-soft', 'h-5', 'w-full', 'font-bold')
    expect(root.querySelector('.animate-ping')).toBeNull()
    expect(root).not.toHaveAttribute('tone')
    expect(root).not.toHaveAttribute('variant')
  })
})
