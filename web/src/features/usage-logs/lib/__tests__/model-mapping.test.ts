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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

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

    assert.deepEqual(result, {
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

    assert.equal(result.isMapped, true)
    assert.equal(result.requestModel, 'model-a')
  })

  test('ignores legacy internal upstream model metadata', () => {
    const result = formatModelName(
      makeLog('model-b', {
        is_model_mapped: true,
        upstream_model_name: 'model-c',
      })
    )

    assert.deepEqual(result, {
      name: 'model-b',
      isMapped: false,
      requestModel: undefined,
    })
  })

  test('does not show an indicator without an external mapping', () => {
    const result = formatModelName(makeLog('model-b', {}))

    assert.equal(result.name, 'model-b')
    assert.equal(result.isMapped, false)
  })
})
