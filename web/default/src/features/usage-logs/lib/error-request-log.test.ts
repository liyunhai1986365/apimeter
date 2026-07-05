import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { LOG_TYPE_ENUM } from '../constants'
import { hasErrorRequestLogRef } from './error-request-log'

describe('hasErrorRequestLogRef', () => {
  test('shows the request evidence entry only for error logs with an independent reference', () => {
    assert.equal(
      hasErrorRequestLogRef(
        { type: LOG_TYPE_ENUM.ERROR },
        { error_request_log_id: 42, request_hash: 'abc' }
      ),
      true
    )

    assert.equal(
      hasErrorRequestLogRef({ type: LOG_TYPE_ENUM.ERROR }, null),
      false
    )

    assert.equal(
      hasErrorRequestLogRef(
        { type: LOG_TYPE_ENUM.CONSUME },
        { error_request_log_id: 42, request_hash: 'abc' }
      ),
      false
    )
  })
})
