/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useCallback, useState } from 'react'
import i18next from 'i18next'
import { toast } from 'sonner'
import { useStatus } from '@/hooks/use-status'

export function useGoCaptcha() {
  const { status } = useStatus()
  const [goCaptchaToken, setGoCaptchaToken] = useState('')
  const isGoCaptchaEnabled = Boolean(
    status?.go_captcha_check ?? status?.data?.go_captcha_check
  )

  const validateGoCaptcha = useCallback(() => {
    if (isGoCaptchaEnabled && !goCaptchaToken) {
      toast.info(i18next.t('Please complete the behavior verification'))
      return false
    }
    return true
  }, [goCaptchaToken, isGoCaptchaEnabled])

  const resetGoCaptcha = useCallback(() => {
    setGoCaptchaToken('')
  }, [])

  return {
    isGoCaptchaEnabled,
    goCaptchaToken,
    setGoCaptchaToken,
    validateGoCaptcha,
    resetGoCaptcha,
  }
}
