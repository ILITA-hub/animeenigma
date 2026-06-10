import { cva, type VariantProps } from 'class-variance-authority'

/**
 * EmptyState — centered placeholder shown when a list/section has no content.
 *
 * `size` controls vertical breathing room only (the icon size is owned by the
 * caller via the #icon slot). sm for tight in-panel empties, md for the common
 * section empty, lg for full-surface empties (e.g. a player with no episodes).
 */
export const emptyStateVariants = cva(
  'flex flex-col items-center justify-center text-center text-muted-foreground',
  {
    variants: {
      size: {
        sm: 'py-8 px-4 gap-1.5',
        md: 'py-12 px-6 gap-2',
        lg: 'py-16 px-6 gap-2',
      },
    },
    defaultVariants: {
      size: 'md',
    },
  },
)

export type EmptyStateVariants = VariantProps<typeof emptyStateVariants>
