import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { getInviteRewardsApiError } from './api'

describe('invite rewards API errors', () => {
  test('prefers the backend response message for failed mutations', () => {
    assert.equal(
      getInviteRewardsApiError(
        {
          message: 'Request failed with status code 400',
          response: { data: { message: '邀请额度不足' } },
        },
        'Transfer failed'
      ),
      '邀请额度不足'
    )
  })

  test('falls back to the local error message and default text', () => {
    assert.equal(
      getInviteRewardsApiError(new Error('Network Error'), 'Transfer failed'),
      'Network Error'
    )
    assert.equal(
      getInviteRewardsApiError(undefined, 'Transfer failed'),
      'Transfer failed'
    )
  })
})
