import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  getLegalConsentAvailability,
  isLegalConsentAccepted,
  markLegalConsentAccepted,
  subscribeLegalConsentAccepted,
  shouldShowLegalConsentPrompt,
} from './legal-consent-storage'

describe('legal consent prompt storage', () => {
  test('shows the prompt on first use when any legal document is enabled', () => {
    assert.equal(
      shouldShowLegalConsentPrompt({
        userAgreementEnabled: true,
        privacyPolicyEnabled: false,
        promptDismissed: false,
      }),
      true
    )
    assert.equal(
      shouldShowLegalConsentPrompt({
        userAgreementEnabled: false,
        privacyPolicyEnabled: true,
        promptDismissed: false,
      }),
      true
    )
  })

  test('does not show the prompt after it has been dismissed', () => {
    assert.equal(
      shouldShowLegalConsentPrompt({
        userAgreementEnabled: true,
        privacyPolicyEnabled: true,
        promptDismissed: true,
      }),
      false
    )
  })

  test('does not show the prompt when no legal document is enabled', () => {
    assert.equal(
      shouldShowLegalConsentPrompt({
        userAgreementEnabled: false,
        privacyPolicyEnabled: false,
        promptDismissed: false,
      }),
      false
    )
  })

  test('reads legal document availability from nested status data', () => {
    assert.deepEqual(
      getLegalConsentAvailability({
        data: {
          user_agreement_enabled: true,
          privacy_policy_enabled: true,
        },
      }),
      {
        userAgreementEnabled: true,
        privacyPolicyEnabled: true,
      }
    )
  })

  test('stores accepted legal consent and notifies current page listeners', () => {
    const storage = new Map<string, string>()
    const eventTarget = new EventTarget()
    const previousWindow = globalThis.window

    Object.defineProperty(globalThis, 'window', {
      value: {
        localStorage: {
          getItem: (key: string) => storage.get(key) ?? null,
          setItem: (key: string, value: string) => storage.set(key, value),
        },
        addEventListener: eventTarget.addEventListener.bind(eventTarget),
        removeEventListener: eventTarget.removeEventListener.bind(eventTarget),
        dispatchEvent: eventTarget.dispatchEvent.bind(eventTarget),
        Event,
      },
      configurable: true,
    })

    try {
      let notificationCount = 0
      const unsubscribe = subscribeLegalConsentAccepted(() => {
        notificationCount += 1
      })

      assert.equal(isLegalConsentAccepted(), false)

      markLegalConsentAccepted()

      assert.equal(isLegalConsentAccepted(), true)
      assert.equal(notificationCount, 1)

      unsubscribe()
      markLegalConsentAccepted()
      assert.equal(notificationCount, 1)
    } finally {
      if (previousWindow === undefined) {
        delete (globalThis as { window?: Window }).window
      } else {
        Object.defineProperty(globalThis, 'window', {
          value: previousWindow,
          configurable: true,
        })
      }
    }
  })
})
