import { useEffect, useState } from 'react'
import { getTaskImageStatus } from '../lib/task-display'
import type { TaskLog } from '../types'

export type TaskImageMetadata = Pick<
  TaskLog,
  'image_status' | 'image_expires_at' | 'image_has_url'
>

export function useTaskImageStatus(log: TaskImageMetadata) {
  const [now, setNow] = useState(() => Date.now())
  const expires = log.image_expires_at ?? 0
  useEffect(() => {
    const update = () => setNow(Date.now())
    const remaining = expires * 1000 - Date.now()
    const timer =
      expires > 0
        ? setTimeout(update, Math.max(0, Math.min(remaining, 2147483647)))
        : undefined
    window.addEventListener('focus', update)
    document.addEventListener('visibilitychange', update)
    return () => {
      clearTimeout(timer)
      window.removeEventListener('focus', update)
      document.removeEventListener('visibilitychange', update)
    }
  }, [expires])
  return getTaskImageStatus(log, now / 1000)
}
