import { describe, it, expect, vi } from 'vitest'
import { defineComponent, h, nextTick } from 'vue'
import { render, screen, fireEvent } from '@testing-library/vue'
import * as v from 'valibot'
import {
  Form,
  FormField,
  FormItem,
  FormLabel,
  FormControl,
  FormDescription,
  FormMessage,
  useForm,
} from './index'

describe('useForm with Valibot Schema', () => {
  const TestSchema = v.object({
    username: v.pipe(v.string(), v.minLength(3, '用户名至少需要3个字符')),
    age: v.pipe(v.number(), v.minValue(18, '必须年满18岁')),
  })

  it('initializes with default values and clean status', () => {
    const form = useForm({
      schema: TestSchema,
      initialValues: { username: 'alex', age: 20 },
    })

    expect(form.values.username).toBe('alex')
    expect(form.values.age).toBe(20)
    expect(form.isDirty.value).toBe(false)
    expect(form.isValid.value).toBe(true)
    expect(Object.keys(form.errors.value)).toHaveLength(0)
  })

  it('detects schema validation errors on validate()', async () => {
    const form = useForm({
      schema: TestSchema,
      initialValues: { username: 'al', age: 16 },
    })

    const valid = await form.validate()
    expect(valid).toBe(false)
    expect(form.errors.value.username).toBe('用户名至少需要3个字符')
    expect(form.errors.value.age).toBe('必须年满18岁')
  })

  it('updates field value and validates field on change', async () => {
    const form = useForm({
      schema: TestSchema,
      initialValues: { username: 'al', age: 20 },
    })

    form.setFieldValue('username', 'alexander')
    await nextTick()

    expect(form.values.username).toBe('alexander')
    expect(form.isDirty.value).toBe(true)
    expect(form.errors.value.username).toBeUndefined()
  })

  it('blocks submit when invalid and invokes onValid when valid', async () => {
    const form = useForm({
      schema: TestSchema,
      initialValues: { username: 'al', age: 20 },
    })

    const onValid = vi.fn()
    const onInvalid = vi.fn()
    const submit = form.handleSubmit(onValid, onInvalid)

    await submit()
    expect(onValid).not.toHaveBeenCalled()
    expect(onInvalid).toHaveBeenCalled()

    form.setFieldValue('username', 'validName')
    await submit()
    expect(onValid).toHaveBeenCalledWith({ username: 'validName', age: 20 })
  })

  it('resets form to initial values and clears errors', async () => {
    const form = useForm({
      schema: TestSchema,
      initialValues: { username: 'initial', age: 25 },
    })

    form.setFieldValue('username', 'changed')
    await form.validate()
    expect(form.isDirty.value).toBe(true)

    form.resetForm()
    expect(form.values.username).toBe('initial')
    expect(form.isDirty.value).toBe(false)
    expect(form.errors.value).toEqual({})
  })
})

describe('Form compound components', () => {
  const Schema = v.object({
    email: v.pipe(v.string(), v.email('请输入有效的邮箱地址')),
  })

  const TestFormComponent = defineComponent({
    setup() {
      const form = useForm({
        schema: Schema,
        initialValues: { email: '' },
      })
      const onSubmit = form.handleSubmit(() => {})
      return { form, onSubmit }
    },
    render() {
      return h(
        Form,
        { onSubmit: this.onSubmit },
        {
          default: () => [
            h(
              FormField,
              { name: 'email' },
              {
                default: () =>
                  h(
                    FormItem,
                    {},
                    {
                      default: () => [
                        h(FormLabel, {}, { default: () => '电子邮箱' }),
                        h(
                          FormControl,
                          {},
                          {
                            default: (slotProps: Record<string, unknown>) =>
                              h('input', {
                                ...slotProps,
                                value: this.form.values.email,
                                onInput: (e: Event) => {
                                  const target = e.target as HTMLInputElement | null
                                  if (target) this.form.setFieldValue('email', target.value)
                                },
                              }),
                          },
                        ),
                        h(FormDescription, {}, { default: () => '我们将通过此邮箱联系您' }),
                        h(FormMessage),
                      ],
                    },
                  ),
              },
            ),
            h('button', { type: 'submit' }, '提交'),
          ],
        },
      )
    },
  })

  it('renders labels, inputs and descriptions with matching accessible attributes', () => {
    render(TestFormComponent)

    const label = screen.getByText('电子邮箱')
    const input = screen.getByRole('textbox')
    const description = screen.getByText('我们将通过此邮箱联系您')

    expect(label).toHaveAttribute('for', input.id)
    expect(input).toHaveAttribute('aria-describedby', description.id)
  })

  it('renders validation message and marks input invalid on error', async () => {
    render(TestFormComponent)

    const submitBtn = screen.getByRole('button', { name: '提交' })
    await fireEvent.click(submitBtn)

    const errorMessage = await screen.findByText('请输入有效的邮箱地址')
    expect(errorMessage).toBeInTheDocument()

    const input = screen.getByRole('textbox')
    expect(input).toHaveAttribute('aria-invalid', 'true')
  })

  it('exposes field, error, and invalid slot props directly on FormField', async () => {
    let capturedSlotProps: Record<string, unknown> = {}

    const Harness = defineComponent({
      setup() {
        const form = useForm({
          schema: Schema,
          initialValues: { email: '' },
        })
        return { form }
      },
      render() {
        return h(
          Form,
          {},
          {
            default: () => [
              h(
                FormField,
                { name: 'email' },
                {
                  default: (slotProps: Record<string, unknown>) => {
                    capturedSlotProps = slotProps
                    return h('span', 'inner')
                  },
                },
              ),
            ],
          },
        )
      },
    })

    render(Harness)
    expect(capturedSlotProps.field).toBeDefined()
    expect((capturedSlotProps.field as { name: string }).name).toBe('email')
    expect(capturedSlotProps.invalid).toBe(false)
    expect(capturedSlotProps.error).toBeUndefined()
  })

  it('handles asynchronous StandardSchema validation correctly', async () => {
    const asyncSchema = {
      '~standard': {
        validate: async (value: unknown) => {
          const val = value as { username?: string }
          if (val.username === 'taken') {
            return {
              issues: [{ path: ['username'], message: '该用户名已被占用' }],
            }
          }
          return { issues: [] }
        },
      },
    }

    const form = useForm({
      schema: asyncSchema,
      initialValues: { username: 'available' },
    })

    const validFirst = await form.validate()
    expect(validFirst).toBe(true)
    expect(form.errors.value.username).toBeUndefined()

    form.setFieldValue('username', 'taken')
    const validSecond = await form.validate()
    expect(validSecond).toBe(false)
    expect(form.errors.value.username).toBe('该用户名已被占用')
  })

  it('preserves complex types like Date during initial clone and resetForm', async () => {
    const birthday = new Date('2000-01-01T00:00:00Z')
    const form = useForm({
      initialValues: { name: 'Bob', birthday },
    })

    expect(form.values.birthday).toBeInstanceOf(Date)
    expect(form.values.birthday.getTime()).toBe(birthday.getTime())
    expect(form.isDirty.value).toBe(false)

    // 修改 Date
    form.setFieldValue('birthday', new Date('2005-05-05T00:00:00Z'))
    expect(form.isDirty.value).toBe(true)

    // 重置后
    form.resetForm()
    expect(form.values.birthday).toBeInstanceOf(Date)
    expect(form.values.birthday.getTime()).toBe(birthday.getTime())
    expect(form.isDirty.value).toBe(false)
  })
})
