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

import { LOG_TYPE_ENUM } from '../../constants'
import { getCommonLogRowClassName } from '../usage-log-row-appearance'

const surfaces = ['table', 'mobile'] as const

describe('retry log row appearance', () => {
  test('deemphasizes retry logs without blurring them', () => {
    const log = { type: LOG_TYPE_ENUM.ERROR, should_retry: 1, other: '' }
    const tableClassName = getCommonLogRowClassName(log, 'table', true)
    const mobileClassName = getCommonLogRowClassName(log, 'mobile', true)

    assert.match(tableClassName, /(?:^|\s)\[&>td\]:opacity-70(?:\s|$)/)
    assert.match(tableClassName, /(?:^|\s)hover:\[&>td\]:opacity-90(?:\s|$)/)
    assert.match(
      tableClassName,
      /(?:^|\s)focus-within:\[&>td\]:opacity-90(?:\s|$)/
    )
    assert.doesNotMatch(tableClassName, /(?:^|\s)opacity-70(?:\s|$)/)

    assert.match(mobileClassName, /(?:^|\s)opacity-70(?:\s|$)/)
    assert.match(mobileClassName, /(?:^|\s)hover:opacity-90(?:\s|$)/)
    assert.match(mobileClassName, /(?:^|\s)focus-within:opacity-90(?:\s|$)/)

    assert.doesNotMatch(tableClassName, /blur/)
    assert.doesNotMatch(mobileClassName, /blur/)
  })

  test('keeps quota saturation warning stronger than retry deemphasis', () => {
    const other = JSON.stringify({
      admin_info: {
        quota_saturation: {
          kind: 'float',
          original: '1e100',
          clamped: 2147483647,
          op: 'test',
        },
      },
    })

    for (const surface of surfaces) {
      const className = getCommonLogRowClassName(
        { type: LOG_TYPE_ENUM.ERROR, should_retry: 1, other },
        surface,
        true
      )

      assert.match(className, /(?:^|\s)bg-warning\/10(?:\s|$)/)
      assert.doesNotMatch(className, /opacity-/)
    }
  })

  test('keeps ordinary and user-visible errors on the error tint', () => {
    for (const surface of surfaces) {
      const ordinaryErrorClassName = getCommonLogRowClassName(
        { type: LOG_TYPE_ENUM.ERROR, should_retry: 0, other: '' },
        surface,
        true
      )
      const userErrorClassName = getCommonLogRowClassName(
        { type: LOG_TYPE_ENUM.ERROR, should_retry: 1, other: '' },
        surface,
        false
      )

      assert.match(ordinaryErrorClassName, /(?:^|\s)bg-rose-50\/40(?:\s|$)/)
      assert.match(userErrorClassName, /(?:^|\s)bg-rose-50\/40(?:\s|$)/)
    }
  })
})
