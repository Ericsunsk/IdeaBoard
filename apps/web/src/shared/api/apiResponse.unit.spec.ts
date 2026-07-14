import { describe, expect, it } from 'vitest'
import { assertApiOk, requireApiData } from './apiResponse'

describe('generated API response boundary', () => {
  it('returns present data from a successful response, including falsy values', () => {
    expect(requireApiData({ code: 0, msg: 'success', data: 0 }, '加载失败')).toBe(0)
    expect(requireApiData({ code: 0, msg: 'success', data: false }, '加载失败')).toBe(false)
    expect(requireApiData({ code: 0, msg: 'success', data: '' }, '加载失败')).toBe('')
  })

  it('rejects failed or incomplete data responses with safe caller-facing messages', () => {
    expect(() => requireApiData({ code: 503, msg: '服务暂不可用' }, '加载失败')).toThrow(
      '服务暂不可用',
    )
    expect(() =>
      requireApiData({ code: 0, msg: 'success', data: undefined }, '加载失败'),
    ).toThrow('加载失败')
  })

  it('falls back when the server message is sensitive or internal', () => {
    expect(() =>
      requireApiData({ code: 500, msg: 'api key invalid' }, '加载失败'),
    ).toThrow('加载失败')
    expect(() =>
      assertApiOk({ code: 500, msg: 'dial tcp 127.0.0.1:5432' }, '保存失败'),
    ).toThrow('保存失败')
  })

  it('accepts successful responses without data and rejects failed responses', () => {
    expect(() => assertApiOk({ code: 0, msg: 'success' }, '保存失败')).not.toThrow()
    expect(() => assertApiOk({ code: 409, msg: '名称已存在' }, '保存失败')).toThrow(
      '名称已存在',
    )
  })
})
