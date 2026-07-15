const UTF8_BOM = '\uFEFF'
const CSV_HEADER = 'code,campaign_name,credit_amount,expires_at\r\n'
const FORMULA_PREFIX = /^(?:\s*[=+\-@]|[\t\r])/u

export type RewardCodeBatchCsvInput = {
  codes: readonly string[]
  campaignName: string
  creditAmount: number
  expiresAt: string
}

export function createRewardCodeBatchCsv(input: RewardCodeBatchCsvInput): string {
  const rows = input.codes.map((code) =>
    [code, input.campaignName, String(input.creditAmount), input.expiresAt]
      .map(toCsvCell)
      .join(','),
  )
  return `${UTF8_BOM}${CSV_HEADER}${rows.map((row) => `${row}\r\n`).join('')}`
}

export function downloadRewardCodeBatchCsv(input: {
  campaignId: number
  batchId: number
  csv: string
}) {
  const objectURL = URL.createObjectURL(
    new Blob([input.csv], { type: 'text/csv;charset=utf-8' }),
  )
  const anchor = document.createElement('a')
  anchor.href = objectURL
  anchor.download = `reward-campaign-${input.campaignId}-batch-${input.batchId}.csv`
  anchor.hidden = true
  document.body.append(anchor)
  try {
    anchor.click()
  } finally {
    anchor.remove()
    URL.revokeObjectURL(objectURL)
  }
}

function toCsvCell(value: string): string {
  const safeValue = FORMULA_PREFIX.test(value) ? `'${value}` : value
  return `"${safeValue.replaceAll('"', '""')}"`
}
