import { describe, expect, it } from 'vitest'
import {
  extractSafeApiErrorMessage,
  formatDisplayErrorMessage,
  sanitizeApiErrorMessage,
  sanitizeTaskErrorMessage,
} from './errorMessages'

describe('error message helpers', () => {
  it('prefers safe explicit messages over fallbacks', () => {
    expect(formatDisplayErrorMessage(new Error('服务暂时不可用'), '默认错误')).toBe(
      '服务暂时不可用',
    )
    expect(extractSafeApiErrorMessage({ msg: '保存失败' }, '默认错误')).toBe('保存失败')
    expect(
      formatDisplayErrorMessage(
        new Error('无效的 Stripe API Key：请使用 sk_ 或 rk_ 开头的对应环境 Key'),
        '默认错误',
      ),
    ).toBe('无效的 Stripe API Key：请使用 sk_ 或 rk_ 开头的对应环境 Key')
    expect(
      formatDisplayErrorMessage(
        new Error('仍有未完成的充值订单，请稍后再更换 Stripe API Key'),
        '默认错误',
      ),
    ).toBe('仍有未完成的充值订单，请稍后再更换 Stripe API Key')
  })

  it('falls back for blank, sensitive, or internal messages', () => {
    expect(sanitizeApiErrorMessage(' ', '默认错误')).toBe('默认错误')
    expect(formatDisplayErrorMessage(new Error('sk_live_exposed'), '默认错误')).toBe('默认错误')
    expect(formatDisplayErrorMessage(new Error('rk_live_exposed'), '默认错误')).toBe('默认错误')
    expect(formatDisplayErrorMessage(new Error('api_key=exposed'), '默认错误')).toBe('默认错误')
    expect(extractSafeApiErrorMessage({ msg: 'dial tcp 127.0.0.1:5432' }, '默认错误')).toBe(
      '默认错误',
    )
  })

  it('ignores unsupported API error payload shapes', () => {
    expect(extractSafeApiErrorMessage({ message: '不会读取 message 字段' }, '默认错误')).toBe(
      '默认错误',
    )
    expect(formatDisplayErrorMessage(null, '默认错误')).toBe('默认错误')
  })

  it('uses the caller fallback for transport errors whose original message was unsafe', () => {
    const error = Object.assign(new Error('请求失败'), { hasSafeDisplayMessage: false })

    expect(formatDisplayErrorMessage(error, '加载失败')).toBe('加载失败')
  })

  it('sanitizes task errors against the caller-supplied fallback', () => {
    expect(
      sanitizeTaskErrorMessage('Image generation failed. Please try again.', '调用方兜底'),
    ).toBe('Image generation failed. Please try again.')
    expect(sanitizeTaskErrorMessage('dial tcp 127.0.0.1:5432', '调用方兜底')).toBe('调用方兜底')
    expect(sanitizeTaskErrorMessage('', '调用方兜底')).toBe('调用方兜底')
  })
})
