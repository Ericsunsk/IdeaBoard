const ideaNumberFormat = new Intl.NumberFormat('zh-CN')

export function formatIdeaAmount(credits: number) {
  return `${ideaNumberFormat.format(credits)} ${credits === 1 ? 'Idea' : 'Ideas'}`
}
