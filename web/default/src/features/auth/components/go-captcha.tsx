/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useCallback, useMemo, useState } from 'react'
import GoCaptchaReact from 'go-captcha-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  generateGoCaptcha,
  verifyGoCaptcha,
  type CaptchaChallenge,
  type CaptchaPosition,
  type CaptchaScene,
} from '@/features/auth/api'

const CAPTCHA_IMAGE_WIDTH = 300
const CAPTCHA_IMAGE_HEIGHT = 220

type GoCaptchaProps = {
  scene: CaptchaScene
  token: string
  onVerify: (token: string) => void
}

export function GoCaptcha({ scene, token, onVerify }: GoCaptchaProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [challenge, setChallenge] = useState<CaptchaChallenge | null>(null)
  const [isVerifying, setIsVerifying] = useState(false)

  const loadChallenge = useCallback(async () => {
    setChallenge(null)
    onVerify('')
    try {
      const response = await generateGoCaptcha(scene)
      if (response.success && response.data) {
        setChallenge(response.data)
        return
      }
      toast.error(t('Failed to load behavior verification'))
    } catch (_error) {
      toast.error(t('Failed to load behavior verification'))
    }
  }, [onVerify, scene, t])

  const openCaptcha = () => {
    setOpen(true)
    void loadChallenge()
  }

  const verify = useCallback(
    async (position: CaptchaPosition, reset: () => void) => {
      if (!challenge || isVerifying) return
      setIsVerifying(true)
      try {
        const response = await verifyGoCaptcha(
          scene,
          challenge.captcha_key,
          position
        )
        const proof = response.data?.token
        if (response.success && proof) {
          onVerify(proof)
          setOpen(false)
          toast.success(t('Behavior verification completed'))
          return
        }
        reset()
        await loadChallenge()
      } catch (_error) {
        reset()
        await loadChallenge()
      } finally {
        setIsVerifying(false)
      }
    },
    [challenge, isVerifying, loadChallenge, onVerify, scene, t]
  )

  const slideConfig = useMemo(
    () => ({
      width: CAPTCHA_IMAGE_WIDTH,
      height: CAPTCHA_IMAGE_HEIGHT,
      title: t('Drag the slider to fit the puzzle piece into the gap.'),
      showTheme: true,
      scope: true,
    }),
    [t]
  )
  const slideData = useMemo(
    () => ({
      image: challenge?.image ?? '',
      thumb: challenge?.tile ?? '',
      thumbX: challenge?.tile_x ?? 0,
      thumbY: challenge?.tile_y ?? 0,
      thumbWidth: challenge?.tile_width ?? 0,
      thumbHeight: challenge?.tile_height ?? 0,
    }),
    [challenge]
  )
  const slideEvents = useMemo(
    () => ({
      close: () => setOpen(false),
      refresh: () => void loadChallenge(),
      confirm: (position: CaptchaPosition, reset: () => void) =>
        void verify(position, reset),
    }),
    [loadChallenge, verify]
  )

  return (
    <>
      <div className='flex w-full justify-center [&>div]:!w-full'>
        <GoCaptchaReact.Button
          config={{ width: 330, height: 44 }}
          type={token ? 'success' : 'default'}
          title={token ? t('Verified') : t('Slide to verify')}
          clickEvent={openCaptcha}
        />
      </div>

      {open && (
        <div
          className='fixed inset-0 z-50 flex items-center justify-center overflow-auto bg-black/50 p-2'
          role='dialog'
          aria-modal='true'
          aria-label={t('Behavior verification')}
        >
          <GoCaptchaReact.Slide
            config={slideConfig}
            data={slideData}
            events={slideEvents}
          />
        </div>
      )}
    </>
  )
}
