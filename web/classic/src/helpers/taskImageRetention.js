export function getTaskImageStatus(record, now = Date.now() / 1000) {
  if (
    record?.image_status === 'expired' ||
    record?.image_status === 'partially_expired'
  )
    return record.image_status;
  if (
    record?.image_status === 'available' &&
    record.image_expires_at &&
    now >= record.image_expires_at
  ) {
    return record.image_has_url ? 'partially_expired' : 'expired';
  }
  return record?.image_status;
}
