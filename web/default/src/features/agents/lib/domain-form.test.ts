import { zodResolver } from '@hookform/resolvers/zod'
import { createInstance } from 'i18next'
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { createAgentDomainsSchema } from './domain-form'

test('invalid batch entries produce a visible error on the domain textarea', async () => {
  const i18n = createInstance()
  const t = await i18n.init({
    lng: 'en',
    resources: { en: { translation: {} } },
  })
  const resolver = zodResolver(createAgentDomainsSchema(t))
  const options = { fields: {}, shouldUseNativeValidation: false }

  const result = await resolver(
    { domains: 'valid.example.com\nhttps://invalid.example.com/path' },
    undefined,
    options
  )
  assert.equal(
    result.errors.domains?.message,
    'Enter valid domain names without a protocol or path.'
  )
  assert.deepEqual(result.values, {})

  const valid = await resolver(
    { domains: 'site.example.com， api.example.com\nbackup.example.com\n' },
    undefined,
    options
  )
  assert.deepEqual(valid.errors, {})
  assert.deepEqual(valid.values, {
    domains: ['site.example.com', 'api.example.com', 'backup.example.com'],
  })
})
