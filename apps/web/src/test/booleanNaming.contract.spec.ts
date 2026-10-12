// @vitest-environment node
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { ESLint } from 'eslint'
import { beforeAll, describe, expect, it } from 'vitest'

const webRoot = fileURLToPath(new URL('../../', import.meta.url))
const eslint = new ESLint({ cwd: webRoot })
const ruleId = 'governance/boolean-prefix'

async function booleanMessages(code: string, file = 'src/test/booleanNaming.contract.spec.ts') {
  const [result] = await eslint.lintText(code, { filePath: path.join(webRoot, file) })
  expect(result?.messages.filter((message) => message.fatal)).toEqual([])
  return result!.messages.filter((message) => message.ruleId === ruleId)
}

describe('Boolean naming through the actual ESLint config', () => {
  beforeAll(async () => {
    await booleanMessages('export {}')
  }, 30_000)

  it('rejects new local bindings and project-owned fields', async () => {
    const messages = await booleanMessages(`
      const busy = false
      type LocalOptions = { busy: boolean }
      const isOptions: LocalOptions = { busy: true }
      console.log(busy, isOptions)
    `)
    expect(messages).toHaveLength(3)
  })

  it('keeps predicate names valid for booleans and Vue refs', async () => {
    expect(
      await booleanMessages(`
      import { ref } from 'vue'
      const isBusy = ref(false)
      const shouldSave = true
      console.log(isBusy, shouldSave)
    `),
    ).toEqual([])
  })

  it('preserves nullable Vue and DOM option contracts', async () => {
    expect(
      await booleanMessages(`
      import { ref, watch } from 'vue'
      watch(ref(false), () => {}, { immediate: true })
      new KeyboardEvent('keydown', { bubbles: true, cancelable: true })
    `),
    ).toEqual([])
  })

  it('preserves option fields across callable and primitive unions', async () => {
    expect(
      await booleanMessages(`
      import { QueryClient } from '@tanstack/vue-query'
      const client = new QueryClient()
      void client.invalidateQueries({ queryKey: ['boards'], exact: true })
      window.addEventListener('click', () => {}, { once: true, passive: true })
    `),
    ).toEqual([])
  })

  it('treats dictionary keys as identities while checking explicitly declared fields', async () => {
    const messages = await booleanMessages(`
      const providers: Record<string, boolean> = { google: true }
      const options: Record<string, unknown> & { busy: boolean } = { busy: true }
      console.log(providers, options)
    `)
    expect(messages).toHaveLength(2)
    expect(messages.every((message) => message.message.includes(':busy|'))).toBe(true)
  })

  it('preserves documented wire contracts without exempting their local consumers', async () => {
    const messages = await booleanMessages(`
      /** @externalContract Server readiness response. */
      type Readiness = { ready: boolean }
      const response: Readiness = { ready: true }
      const ready = response.ready
      console.log(ready)
    `)
    expect(messages).toHaveLength(1)
    expect(messages[0]?.message).toContain('|variable:ready|1')
  })

  it('recognizes Vue prop options through the imported constructor', async () => {
    expect(
      await booleanMessages(`
      import { defineComponent as component } from 'vue'
      const Widget = component({
        props: { isOpen: { type: Boolean, default: false, required: true } },
      })
      console.log(Widget)
    `),
    ).toEqual([])
    const messages = await booleanMessages(`
      const localOptions = { props: { panel: { default: false, required: true } } }
      console.log(localOptions)
    `)
    expect(messages).toHaveLength(2)
  })

  it('preserves generated wire option names', async () => {
    expect(
      await booleanMessages(`
      import type { ControlStripeConfigRequest } from '../api/generated-query/control/model/controlStripeConfigRequest'
      const request: ControlStripeConfigRequest = { purchases_enabled: true }
      console.log(request)
    `),
    ).toEqual([])
  })

  it('does not exempt new occurrences in a file containing a baseline declaration', async () => {
    const messages = await booleanMessages(
      `
      const apiFetch = { retry: false, retry: true }
      console.log(apiFetch)
    `,
      'src/api/client.ts',
    )
    expect(messages).toHaveLength(1)
    expect(messages[0]?.message).toContain('|property:retry|2')
  })
})
