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
import { render } from '@testing-library/react'
import { describe, expect, test } from 'vitest'

const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { ApiKeyGroupCell } = await import('../api-key-group-cell')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        Auto: 'Auto',
        Ratio: 'Ratio',
      },
    },
  },
})

function CellHarness(props: { group: string; ratio?: number | string }) {
  return (
    <I18nextProvider i18n={i18n}>
      <ApiKeyGroupCell group={props.group} ratio={props.ratio} />
    </I18nextProvider>
  )
}

describe('API key group table cell', () => {
  test('renders Auto as one standard badge without retry, ratio, or animated borders', () => {
    const { container } = render(<CellHarness group='auto' ratio='自动' />)

    const badges = container.querySelectorAll('[data-slot="status-badge"]')
    expect(badges).toHaveLength(1)
    expect(badges[0]).toHaveTextContent('Auto')
    expect(container).not.toHaveTextContent('自动')
    expect(container).not.toHaveTextContent('Cross-group')
    expect(container).not.toHaveTextContent('Ratio')
    expect(container.querySelector('[data-auto-group-frame]')).toBeNull()
    expect(container.querySelector('[data-auto-group-flow-border]')).toBeNull()
  })

  test('narrows normal group ratios to numbers and never applies Auto rings', () => {
    const { container, rerender } = render(
      <CellHarness group='vip' ratio='自动' />
    )

    expect(container).toHaveTextContent('vip')
    expect(container).not.toHaveTextContent('自动')
    expect(container.querySelector('[data-auto-group-frame]')).toBeNull()
    expect(container.querySelector('[data-auto-group-flow-border]')).toBeNull()

    rerender(<CellHarness group='vip' ratio={3} />)

    expect(container).toHaveTextContent('3x')
    expect(container.querySelector('[data-auto-group-frame]')).toBeNull()
  })
})
