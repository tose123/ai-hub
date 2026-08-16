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
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, test, vi } from 'vitest'

vi.mock('@/lib/lobe-icon', () => ({
  getLobeIcon: () => null,
}))

const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { ModelBadge } = await import('../model-badge')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        'Model Mapping': 'Model Mapping',
        'Request Model:': 'Request Model:',
        'Actual Model:': 'Actual Model:',
      },
    },
  },
})

describe('model badge mapping popover', () => {
  test('opens the mapping relation from an independent icon button', () => {
    const { container } = render(
      <I18nextProvider i18n={i18n}>
        <ModelBadge modelName='model-b' requestModel='alias-a' />
      </I18nextProvider>
    )

    expect(container).toHaveTextContent('model-b')
    fireEvent.click(screen.getByRole('button', { name: 'Model Mapping' }))
    expect(document.body).toHaveTextContent('Request Model:')
    expect(document.body).toHaveTextContent('Actual Model:')
    expect(document.body).toHaveTextContent('alias-a')
    expect(document.body).toHaveTextContent('model-b')
  })

  test('does not render the mapping icon without an external mapping', () => {
    const { container } = render(
      <I18nextProvider i18n={i18n}>
        <ModelBadge modelName='model-b' />
      </I18nextProvider>
    )

    expect(
      container.querySelector('button[aria-label="Model Mapping"]')
    ).toBeNull()
  })
})
