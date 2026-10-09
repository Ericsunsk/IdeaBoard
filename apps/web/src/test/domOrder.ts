import { expect } from 'vitest'

// 断言一组元素按给定顺序出现在 DOM 中（列序/按钮序契约），替代各 spec
// 手写的 compareDocumentPosition 循环。
export function expectDomOrder(elements: Array<Element | null | undefined>): void {
  for (let index = 0; index < elements.length - 1; index += 1) {
    const current = elements[index]
    const next = elements[index + 1]
    expect(current, `element at index ${index} is missing`).toBeTruthy()
    expect(
      current!.compareDocumentPosition(next!) & Node.DOCUMENT_POSITION_FOLLOWING,
      `element at index ${index} should come before index ${index + 1}`,
    ).toBeTruthy()
  }
}
