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
import { describe, expect, test, vi } from 'vitest'

import { createSetupStatusChecker } from './setup-status'

describe('background setup status checks', () => {
  test('deduplicates concurrent checks and caches only initialized status', async () => {
    const load = vi.fn(async () => ({
      success: true,
      data: { status: true, root_init: true, database_type: 'sqlite' },
    }))
    const writeChecked = vi.fn()
    const check = createSetupStatusChecker({
      load,
      readChecked: () => false,
      writeChecked,
    })

    await expect(Promise.all([check(), check()])).resolves.toEqual([
      'initialized',
      'initialized',
    ])
    await expect(check()).resolves.toBe('initialized')
    expect(load).toHaveBeenCalledTimes(1)
    expect(writeChecked).toHaveBeenCalledTimes(1)
  })

  test('keeps failed checks retryable', async () => {
    const load = vi
      .fn()
      .mockRejectedValueOnce(new Error('unavailable'))
      .mockResolvedValueOnce({
        success: true,
        data: { status: true, root_init: true, database_type: 'sqlite' },
      })
    const check = createSetupStatusChecker({
      load,
      readChecked: () => false,
      writeChecked: vi.fn(),
    })

    await expect(check()).resolves.toBe('unavailable')
    await expect(check()).resolves.toBe('initialized')
    expect(load).toHaveBeenCalledTimes(2)
  })

  test('does not cache an instance that still requires setup', async () => {
    const writeChecked = vi.fn()
    const check = createSetupStatusChecker({
      load: vi.fn(async () => ({
        success: true,
        data: { status: false, root_init: false, database_type: 'sqlite' },
      })),
      readChecked: () => false,
      writeChecked,
    })

    await expect(check()).resolves.toBe('requires_setup')
    expect(writeChecked).not.toHaveBeenCalled()
  })
})
