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
      },
    } as never)

    assert.equal(config.systemName, 'Acme AI')
    assert.equal(config.serverAddress, 'https://api.example.com')
  })
})
