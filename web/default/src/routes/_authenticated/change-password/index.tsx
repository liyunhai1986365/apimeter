import { createFileRoute } from '@tanstack/react-router'
import { RequiredPasswordChange } from '@/features/profile/components/required-password-change'

export const Route = createFileRoute('/_authenticated/change-password/')({
  component: RequiredPasswordChange,
})
