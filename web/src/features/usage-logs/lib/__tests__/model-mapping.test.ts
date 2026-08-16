/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { describe, expect, test } from 'vitest'

import type { UsageLog } from '../../data/schema'
import { formatModelName } from '../format'

function makeLog(modelName: string, other: Record<string, unknown>): UsageLog {
  return {
    model_name: modelName,
    other: JSON.stringify(other),
  } as UsageLog
}

describe('model mapping display', () => {
  test('shows the external final model and request model', () => {
    const result = formatModelName(
      makeLog('model-b', {
        is_model_mapped: true,
        request_model_name: 'alias-a',
      })
    )

    expect(result).toEqual({
      name: 'model-b',
      isMapped: true,
      requestModel: 'alias-a',
    })
  })

  test('keeps the mapping indicator when both names match', () => {
    const result = formatModelName(
      makeLog('model-a', {
        is_model_mapped: true,
        request_model_name: 'model-a',
      })
    )

    expect(result.isMapped).toBe(true)
    expect(result.requestModel).toBe('model-a')
  })

  test('ignores legacy internal upstream model metadata', () => {
    const result = formatModelName(
      makeLog('model-b', {
        is_model_mapped: true,
        upstream_model_name: 'model-c',
      })
    )

    expect(result).toEqual({
      name: 'model-b',
      isMapped: false,
      requestModel: undefined,
    })
  })

  test('does not show an indicator without an external mapping', () => {
    const result = formatModelName(makeLog('model-b', {}))

    expect(result.name).toBe('model-b')
    expect(result.isMapped).toBe(false)
  })
})
