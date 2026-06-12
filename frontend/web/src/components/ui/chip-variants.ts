import { cva, type VariantProps } from 'class-variance-authority'

// Chip — toggleable / removable filter pill (Profile status filters,
// Schedule My-List toggle + applied-filter chips). One pill language:
// rounded-full, tinted cyan when active, glass when not.
export const chipVariants = cva(
  'inline-flex items-center gap-1.5 shrink-0 rounded-full font-medium border transition-colors cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500/50',
  {
    variants: {
      size: {
        sm: 'px-2.5 py-1 text-xs',
        md: 'px-4 py-2 text-sm',
      },
      active: {
        true: 'bg-primary/15 text-primary border-primary/40',
        false: 'bg-white/5 text-white/80 border-white/10 hover:bg-white/10 hover:text-white',
      },
    },
    defaultVariants: { size: 'md', active: false },
  },
)

export type ChipVariants = VariantProps<typeof chipVariants>
