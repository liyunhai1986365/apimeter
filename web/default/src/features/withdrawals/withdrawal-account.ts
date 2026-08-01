export type WithdrawalMethod = 'alipay' | 'usdt'

export function formatWithdrawalAccount(
  method: WithdrawalMethod,
  account: string,
  network = ''
): string {
  const normalizedAccount = account.trim()
  if (method === 'alipay') return `Alipay: ${normalizedAccount}`
  return `USDT (${network.trim()}): ${normalizedAccount}`
}
