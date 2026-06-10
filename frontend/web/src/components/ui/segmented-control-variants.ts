import { cva, type VariantProps } from 'class-variance-authority'

/** The inset-pill container that holds the segment buttons. */
export const segmentedControlVariants = cva(
  'inline-flex items-center gap-0.5 rounded-lg bg-white/[0.06] p-0.5',
  {
    variants: {
      fullWidth: { true: 'w-full', false: '' },
    },
    defaultVariants: { fullWidth: false },
  },
)

/** A single segment button. Active state is driven by `data-active`. */
export const segmentVariants = cva(
  [
    'inline-flex items-center justify-center gap-1.5 shrink-0 cursor-pointer',
    'rounded-md font-medium transition-colors',
    'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
    'text-muted-foreground hover:text-foreground',
    'data-[active=true]:bg-primary data-[active=true]:text-primary-foreground data-[active=true]:font-semibold',
  ],
  {
    variants: {
      size: {
        sm: 'text-xs px-3.5 py-1.5',
        md: 'text-sm px-4 py-2',
      },
      iconOnly: {
        true: 'aspect-square px-0',
        false: '',
      },
      fullWidth: { true: 'flex-1', false: '' },
    },
    defaultVariants: { size: 'sm', iconOnly: false, fullWidth: false },
  },
)

export type SegmentedControlVariants = VariantProps<typeof segmentVariants>
