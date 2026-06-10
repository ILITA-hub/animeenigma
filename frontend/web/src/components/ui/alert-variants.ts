import { cva, type VariantProps } from 'class-variance-authority'

export const alertVariants = cva('flex items-start gap-3 rounded-xl border p-4 text-sm', {
  variants: {
    variant: {
      info: 'bg-info-soft border-info/30',
      success: 'bg-success-soft border-success/30',
      warning: 'bg-warning-soft border-warning/30',
      destructive: 'bg-destructive-soft border-destructive/30',
    },
  },
  defaultVariants: { variant: 'info' },
})

export type AlertVariants = VariantProps<typeof alertVariants>

export type AlertVariant = NonNullable<AlertVariants['variant']>

// Per-variant icon tint (separate from the cva so tailwind-merge keeps bg/border
// and text colors in independent groups).
export const alertIconColor: Record<AlertVariant, string> = {
  info: 'text-info',
  success: 'text-success',
  warning: 'text-warning',
  destructive: 'text-destructive',
}
