const adminIdeaNumberFormat = new Intl.NumberFormat('zh-CN')

export function formatIdeaAmount(credits: number): string {
  return `${adminIdeaNumberFormat.format(credits)} ${credits === 1 ? 'Idea' : 'Ideas'}`
}
