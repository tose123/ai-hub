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

import { isValidLogoUrl } from '../system-info-validation'

describe('system information logo URL validation', () => {
  test('accepts empty, absolute, and root-relative logo URLs', () => {
    expect(isValidLogoUrl('')).toBe(true)
    expect(isValidLogoUrl('https://example.com/logo.svg')).toBe(true)
    expect(isValidLogoUrl('/ai-hub-mark.svg')).toBe(true)
  })

  test('rejects bare relative and protocol-relative logo URLs', () => {
    expect(isValidLogoUrl('ai-hub-mark.svg')).toBe(false)
    expect(isValidLogoUrl('//example.com/logo.svg')).toBe(false)
  })
})
