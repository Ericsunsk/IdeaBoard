import { describe, expect, it } from 'vitest'
import {
  extractSafeApiErrorMessage,
  formatDisplayErrorMessage,
  sanitizeApiErrorMessage,
} from './errorMessages'

describe('error message helpers', () => {
  it('prefers safe explicit messages over fallbacks', () => {
    expect(formatDisplayErrorMessage(new Error('服务暂时不可用'), '默认错误')).toBe(
      '服务暂时不可用',
    )
    expect(extractSafeApiErrorMessage({ msg: '保存失败' }, '默认错误')).toBe('保存失败')
  })

  it('falls back for blank, sensitive, or internal messages', () => {
    expect(sanitizeApiErrorMessage(' ', '默认错误')).toBe('默认错误')
    expect(formatDisplayErrorMessage(new Error('api key invalid'), '默认错误')).toBe('默认错误')
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
})
