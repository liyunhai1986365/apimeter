import type { SystemStatus } from '../types'

type LegalConsentPromptState = {
  userAgreementEnabled: boolean
  privacyPolicyEnabled: boolean
  promptDismissed: boolean
}

const LEGAL_CONSENT_PROMPT_DISMISSED_KEY = 'auth_legal_consent_prompt_dismissed'
const LEGAL_CONSENT_ACCEPTED_KEY = 'auth_legal_consent_accepted'
const LEGAL_CONSENT_ACCEPTED_EVENT = 'auth:legal-consent-accepted'

export function shouldShowLegalConsentPrompt({
  userAgreementEnabled,
  privacyPolicyEnabled,
  promptDismissed,
}: LegalConsentPromptState) {
  if (promptDismissed) return false
  return userAgreementEnabled || privacyPolicyEnabled
}

export function getLegalConsentAvailability(status?: SystemStatus | null) {
  return {
    userAgreementEnabled: Boolean(
      status?.user_agreement_enabled ?? status?.data?.user_agreement_enabled
    ),
    privacyPolicyEnabled: Boolean(
      status?.privacy_policy_enabled ?? status?.data?.privacy_policy_enabled
    ),
  }
}

export function isLegalConsentPromptDismissed() {
  if (typeof window === 'undefined') return false

  try {
    return (
      window.localStorage.getItem(LEGAL_CONSENT_PROMPT_DISMISSED_KEY) === 'true'
    )
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to read legal consent prompt state:', error)
    return false
  }
}

export function markLegalConsentPromptDismissed() {
  if (typeof window === 'undefined') return

  try {
    window.localStorage.setItem(LEGAL_CONSENT_PROMPT_DISMISSED_KEY, 'true')
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to save legal consent prompt state:', error)
  }
}

export function isLegalConsentAccepted() {
  if (typeof window === 'undefined') return false

  try {
    return window.localStorage.getItem(LEGAL_CONSENT_ACCEPTED_KEY) === 'true'
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to read legal consent accepted state:', error)
    return false
  }
}

export function markLegalConsentAccepted() {
  if (typeof window === 'undefined') return

  try {
    window.localStorage.setItem(LEGAL_CONSENT_ACCEPTED_KEY, 'true')
    window.localStorage.setItem(LEGAL_CONSENT_PROMPT_DISMISSED_KEY, 'true')
    window.dispatchEvent(new Event(LEGAL_CONSENT_ACCEPTED_EVENT))
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to save legal consent accepted state:', error)
  }
}

export function subscribeLegalConsentAccepted(callback: () => void) {
  if (typeof window === 'undefined') return () => {}

  window.addEventListener(LEGAL_CONSENT_ACCEPTED_EVENT, callback)

  return () => {
    window.removeEventListener(LEGAL_CONSENT_ACCEPTED_EVENT, callback)
  }
}
