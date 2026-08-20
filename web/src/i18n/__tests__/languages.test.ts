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
import { describe, expect, it } from 'vitest'

import {
  convertDetectedLanguage,
  INTERFACE_LANGUAGE_OPTIONS,
  normalizeInterfaceLanguage,
} from '../languages'

describe('interface language normalization', () => {
  it('keeps only Simplified Chinese and English as selectable languages', () => {
    expect(INTERFACE_LANGUAGE_OPTIONS).toEqual([
      { code: 'zhCN', label: '简体中文' },
      { code: 'en', label: 'English' },
    ])
  })

  it.each([
    'zh',
    'zhCN',
    'zh-CN',
    'zh_Hans',
    'zhTW',
    'zh-TW',
    'zh-Hant',
    'zh-HK',
    ' ZH_mo ',
  ])('maps legacy Chinese preference %s to zhCN', (language) => {
    expect(normalizeInterfaceLanguage(language)).toBe('zhCN')
  })

  it.each([undefined, null, '', 'en', 'en-US', 'fr', 'ja', 'ru', 'vi', 'xx'])(
    'maps unsupported preference %s to English',
    (language) => {
      expect(normalizeInterfaceLanguage(language)).toBe('en')
    }
  )

  it('normalizes browser-detected locales to the supported language codes', () => {
    expect(convertDetectedLanguage('zh-Hant-TW')).toBe('zhCN')
    expect(convertDetectedLanguage('fr-FR')).toBe('en')
    expect(convertDetectedLanguage('en-US')).toBe('en')
  })
})
