import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { mapStatusDataToConfig } from './use-system-config'

describe('mapStatusDataToConfig', () => {
  test('maps system name and server address from the status envelope', () => {
    const config = mapStatusDataToConfig({
      success: true,
      data: {
        system_name: 'Acme AI',
        server_address: 'https://api.example.com/',
        footer_company_name: 'Example Technology Ltd.',
        mainland_china_presentation_enabled: true,
        default_user_display_currency: 'USD',
      },
    } as never)

    assert.equal(config.systemName, 'Acme AI')
    assert.equal(config.serverAddress, 'https://api.example.com')
    assert.equal(config.footerCompanyName, 'Example Technology Ltd.')
    assert.equal(config.mainlandChinaPresentationEnabled, true)
    assert.equal(config.defaultUserDisplayCurrency, 'CNY')
  })

  test('preserves an empty footer company name', () => {
    const config = mapStatusDataToConfig({
      success: true,
      data: { footer_company_name: '' },
    } as never)

    assert.equal(config.footerCompanyName, '')
  })

  test('falls back to USD for an invalid default user currency', () => {
    const config = mapStatusDataToConfig({
      success: true,
      data: { default_user_display_currency: 'EUR' },
    } as never)

    assert.equal(config.defaultUserDisplayCurrency, 'USD')
  })
})
