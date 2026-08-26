import { describe, expect, it, vi } from 'vitest'
import PrimeVue from 'primevue/config'
import { render, renderWithUser, screen } from '../../test/render'
import { userPrimeVueTestOptions } from '../../test/primeVue'
import UserLocaleSelect from './UserLocaleSelect.vue'
import * as i18nModule from '../i18n/userLocale'

function renderLocaleSelect(props = {}, options = {}) {
  return render(UserLocaleSelect, {
    props,
    global: {
      plugins: [[PrimeVue, userPrimeVueTestOptions]],
    },
    ...options,
  })
}

function renderLocaleSelectWithUser(props = {}, options = {}) {
  return renderWithUser(UserLocaleSelect, {
    props,
    global: {
      plugins: [[PrimeVue, userPrimeVueTestOptions]],
    },
    ...options,
  })
}

describe('UserLocaleSelect', () => {
  it('renders circular trigger matching 40px chrome control (.ib-round-control.ib-page-chrome-control)', () => {
    renderLocaleSelect()
    const selectRoot = screen.getByTestId('user-locale-select')
    expect(selectRoot).toBeVisible()
    expect(selectRoot.classList.contains('ib-round-control')).toBe(true)
    expect(selectRoot.classList.contains('ib-page-chrome-control')).toBe(true)
  })

  it('closed state renders ONLY the Translate icon without text or chevron', () => {
    renderLocaleSelect()
    const selectRoot = screen.getByTestId('user-locale-select')
    const translateIcon = screen.getByTestId('icon-translate')

    expect(translateIcon).toBeVisible()
    expect(selectRoot).not.toHaveTextContent('EN')
    expect(selectRoot).not.toHaveTextContent('English')
    expect(selectRoot).not.toHaveTextContent('zh-CN')
    expect(screen.queryByRole('listbox')).toBeNull()
  })

  it('exposes accessible attributes (aria-label, title, aria-haspopup)', () => {
    renderLocaleSelect()
    const selectRoot = screen.getByTestId('user-locale-select')

    expect(selectRoot).toHaveAttribute('title')
    expect(selectRoot).toHaveAttribute('aria-label')
    expect(selectRoot.getAttribute('aria-label')).toMatch(/select language|语言/i)
    expect(selectRoot).toHaveAttribute('aria-haspopup', 'menu')
    expect(selectRoot).toHaveAttribute('aria-expanded', 'false')
    expect(selectRoot).toHaveAttribute('aria-busy', 'false')
  })

  it('opens dropdown overlay and displays all 4 native language names', async () => {
    const { user } = renderLocaleSelectWithUser()
    const selectRoot = screen.getByTestId('user-locale-select')

    await user.click(selectRoot)
    expect(await screen.findByRole('menu')).toBeVisible()

    expect(screen.getByRole('menuitem', { name: 'English' })).toBeVisible()
    expect(screen.getByRole('menuitem', { name: '简体中文' })).toBeVisible()
    expect(screen.getByRole('menuitem', { name: 'Español' })).toBeVisible()
    expect(screen.getByRole('menuitem', { name: '한국어' })).toBeVisible()
  })

  it('marks the active language without adding a second indicator icon', async () => {
    const { user } = renderLocaleSelectWithUser()
    await user.click(screen.getByTestId('user-locale-select'))

    const englishItem = await screen.findByRole('menuitem', { name: 'English' })
    const englishAction = englishItem.querySelector('a')
    expect(englishAction).toHaveAttribute('aria-current', 'true')
    expect(englishAction).toHaveAttribute('data-control-state', 'active')
    expect(screen.queryByTestId('icon-locale-selected')).toBeNull()

    const spanishItem = screen.getByRole('menuitem', { name: 'Español' })
    expect(spanishItem.querySelector('a')).not.toHaveAttribute('aria-current')
    expect(spanishItem.querySelector('a')).toHaveAttribute('data-control-state', 'idle')
  })

  it('selecting an option invokes setUserLocale and emits events', async () => {
    const setUserLocaleSpy = vi.spyOn(i18nModule, 'setUserLocale').mockResolvedValue(true)
    const onUpdateModelValue = vi.fn()
    const onChange = vi.fn()

    const { user } = renderLocaleSelectWithUser({
      'onUpdate:modelValue': onUpdateModelValue,
      onChange: onChange,
    })

    const selectRoot = screen.getByTestId('user-locale-select')
    await user.click(selectRoot)

    const spanishOption = await screen.findByRole('menuitem', { name: 'Español' })
    const spanishLink = spanishOption.querySelector('a') as HTMLElement | null
    expect(spanishLink).not.toBeNull()
    await user.click(spanishLink!)

    expect(setUserLocaleSpy).toHaveBeenCalledWith('es')
    expect(onUpdateModelValue).toHaveBeenCalledWith('es')
    expect(onChange).toHaveBeenCalledWith('es')

    setUserLocaleSpy.mockRestore()
  })

  it('disables trigger when disabled prop is set', () => {
    renderLocaleSelect({ disabled: true })
    const trigger = screen.getByRole('button')
    expect(trigger).toBeDisabled()
  })

  it('merges custom wrapClass into root classes', () => {
    renderLocaleSelect({ wrapClass: 'custom-locale-wrap' })
    const selectRoot = screen.getByTestId('user-locale-select')
    expect(selectRoot.classList.contains('custom-locale-wrap')).toBe(true)
    expect(selectRoot.classList.contains('ib-round-control')).toBe(true)
  })
})
