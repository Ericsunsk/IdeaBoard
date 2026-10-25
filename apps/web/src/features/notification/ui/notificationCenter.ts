export type TimelineGroupKey = 'today' | 'yesterday' | 'earlier'

export function getTimelineGroup(
  dateStr?: string | null,
  referenceDate: Date = new Date(),
): TimelineGroupKey {
  if (!dateStr) return 'earlier'
  const date = new Date(dateStr)
  if (isNaN(date.getTime())) return 'earlier'

  const ref = new Date(referenceDate)
  const todayStart = new Date(ref.getFullYear(), ref.getMonth(), ref.getDate()).getTime()
  const yesterdayStart = new Date(ref.getFullYear(), ref.getMonth(), ref.getDate() - 1).getTime()

  const time = date.getTime()
  if (time >= todayStart) {
    return 'today'
  }
  if (time >= yesterdayStart) {
    return 'yesterday'
  }
  return 'earlier'
}
