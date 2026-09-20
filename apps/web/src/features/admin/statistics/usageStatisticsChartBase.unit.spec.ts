import { describe, expect, it } from 'vitest'
import { getModelColorByName } from './usageStatisticsChartBase'

describe('getModelColorByName', () => {
  it('documents hash-stable assignment: same input order gives same colors and a model inserted at front does not shift existing colors', () => {
    const baseline = ['gpt-image', 'seedream', 'opus'].map((name) => getModelColorByName(name))
    expect(['gpt-image', 'seedream', 'opus'].map((name) => getModelColorByName(name))).toEqual(
      baseline,
    )

    const withFrontInsert = ['new-model', 'gpt-image', 'seedream', 'opus'].map((name) =>
      getModelColorByName(name),
    )
    expect(withFrontInsert.slice(1)).toEqual(baseline)
  })
})
