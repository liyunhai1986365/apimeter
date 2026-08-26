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
import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  closePendingPaymentWindow,
  isSafePaymentUrl,
  navigatePendingPaymentWindow,
  openPendingPaymentWindow,
} from './payment-window'

function createPaymentWindow() {
  let redirectedTo = ''
  let closeCalled = false
  const paymentWindow = {
    closed: false,
    opener: {},
    location: {
      replace: (url: string) => {
        redirectedTo = url
      },
    },
    close: () => {
      closeCalled = true
    },
  } as unknown as Window

  return {
    paymentWindow,
    getRedirectedTo: () => redirectedTo,
    wasCloseCalled: () => closeCalled,
  }
}

describe('payment window', () => {
  it('accepts only absolute HTTP checkout URLs', () => {
    assert.equal(isSafePaymentUrl('https://checkout.example.com/pay'), true)
    assert.equal(isSafePaymentUrl('http://localhost:3000/pay'), true)
    assert.equal(isSafePaymentUrl('javascript:alert(1)'), false)
    assert.equal(isSafePaymentUrl('/relative-checkout'), false)
  })

  it('opens a blank page synchronously and removes its opener', () => {
    const { paymentWindow } = createPaymentWindow()
    let openArgs: unknown[] = []

    const result = openPendingPaymentWindow((...args) => {
      openArgs = args
      return paymentWindow
    })

    assert.equal(result, paymentWindow)
    assert.deepEqual(openArgs, ['about:blank', '_blank'])
    assert.equal(paymentWindow.opener, null)
  })

  it('navigates the pre-opened page after checkout creation', () => {
    const { paymentWindow, getRedirectedTo } = createPaymentWindow()

    navigatePendingPaymentWindow(
      paymentWindow,
      'https://checkout.example.com/pay',
      () => {
        throw new Error('fallback should not be used')
      }
    )

    assert.equal(getRedirectedTo(), 'https://checkout.example.com/pay')
  })

  it('falls back to a protected new page when the placeholder is unavailable', () => {
    let openArgs: unknown[] = []

    navigatePendingPaymentWindow(
      null,
      'https://checkout.example.com/pay',
      (...args) => {
        openArgs = args
        return null
      }
    )

    assert.deepEqual(openArgs, [
      'https://checkout.example.com/pay',
      '_blank',
      'noopener,noreferrer',
    ])
  })

  it('closes an unused placeholder after a failed payment request', () => {
    const { paymentWindow, wasCloseCalled } = createPaymentWindow()

    closePendingPaymentWindow(paymentWindow)

    assert.equal(wasCloseCalled(), true)
  })
})
