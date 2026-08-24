import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  migrateSystemConfigState,
  useSystemConfigStore,
} from './system-config-store'

describe('system config currency preference', () => {
  test('applies the admin default until the user selects a currency', (t) => {
    const originalState = useSystemConfigStore.getState()
    t.after(() => useSystemConfigStore.setState(originalState, true))

    useSystemConfigStore.setState({
      displayCurrency: 'USD',
      hasDisplayCurrencyPreference: false,
    })

    useSystemConfigStore.getState().setConfig({
      defaultUserDisplayCurrency: 'CNY',
    })
    assert.equal(useSystemConfigStore.getState().displayCurrency, 'CNY')

    useSystemConfigStore.getState().setDisplayCurrency('USD')
    useSystemConfigStore.getState().setConfig({
      defaultUserDisplayCurrency: 'CNY',
    })
    assert.equal(useSystemConfigStore.getState().displayCurrency, 'USD')
    assert.equal(
      useSystemConfigStore.getState().hasDisplayCurrencyPreference,
      true
    )
  })

  test('treats a legacy persisted currency as a user preference', () => {
    const currentState = useSystemConfigStore.getState()
    const migrated = migrateSystemConfigState(
      {
        config: currentState.config,
        displayCurrency: 'CNY',
        loadedLogoUrl: currentState.loadedLogoUrl,
      },
      0
    )

    assert.equal(migrated.displayCurrency, 'CNY')
    assert.equal(migrated.hasDisplayCurrencyPreference, true)
  })
})
