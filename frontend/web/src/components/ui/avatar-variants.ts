import { cva, type VariantProps } from 'class-variance-authority'

export const avatarVariants = cva('relative inline-flex shrink-0 align-middle', {
  variants: {
    size: {
      xs: 'size-6 text-[10px]',
      sm: 'size-8 text-xs',
      md: 'size-10 text-sm',
      lg: 'size-12 text-[17px]',
      xl: 'size-16 text-[22px]',
    },
  },
  defaultVariants: { size: 'md' },
})

export type AvatarVariants = VariantProps<typeof avatarVariants>
export type AvatarSize = NonNullable<AvatarVariants['size']>
export type AvatarStatus = 'online' | 'idle' | 'offline'

export const avatarDotSize: Record<AvatarSize, string> = {
  xs: 'size-2.5', sm: 'size-2.5', md: 'size-3', lg: 'size-3.5', xl: 'size-4',
}
export const avatarDotColor: Record<AvatarStatus, string> = {
  online: 'bg-success', idle: 'bg-warning', offline: 'bg-white/30',
}

export function avatarInitials(name?: string): string {
  const n = (name ?? '').trim()
  if (!n) return '?'
  return n.split(/\s+/).filter(Boolean).slice(0, 2).map((p) => p[0]).join('').toUpperCase()
}
