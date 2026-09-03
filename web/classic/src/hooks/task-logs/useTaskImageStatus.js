import { useEffect, useState } from 'react';
import { getTaskImageStatus } from '../../helpers/taskImageRetention';

export function useTaskImageStatus(record) {
  const [now, setNow] = useState(() => Date.now());
  const expires = record?.image_expires_at || 0;
  useEffect(() => {
    const update = () => setNow(Date.now());
    const remaining = expires * 1000 - Date.now();
    const timer =
      remaining > 0
        ? setTimeout(update, Math.min(remaining, 2147483647))
        : undefined;
    window.addEventListener('focus', update);
    document.addEventListener('visibilitychange', update);
    return () => {
      clearTimeout(timer);
      window.removeEventListener('focus', update);
      document.removeEventListener('visibilitychange', update);
    };
  }, [expires]);
  return getTaskImageStatus(record, Math.max(now, Date.now()) / 1000);
}
