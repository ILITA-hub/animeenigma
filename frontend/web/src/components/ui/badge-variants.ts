import { cva, type VariantProps } from 'class-variance-authority'

export const badgeVariants = cva('inline-flex items-center font-medium', {
  variants: {
    variant: {
      default: 'bg-white/10 text-white/80',
      primary: 'bg-cyan-500/20 text-cyan-400',
      secondary: 'bg-pink-500/20 text-pink-400',
      success: 'bg-emerald-500/20 text-emerald-400',
      warning: 'bg-amber-500/20 text-amber-400',
      rating: 'bg-black/60 text-amber-400 backdrop-blur-sm',
      // Phase 5 (LIB-09): purple for Nyaa provider chip — intentional literal color.
      info: 'bg-purple-500/20 text-purple-400',
      // Phase 5 (LIB-09): red for failed-job status badges — intentional literal color.
      destructive: 'bg-red-500/20 text-red-400',
    },
    size: {
      sm: 'px-2 py-0.5 text-xs rounded',
      md: 'px-2.5 py-1 text-sm rounded-md',
      lg: 'px-3 py-1.5 text-base rounded-lg',
    },
    // Overlay treatment for badges sitting on top of posters/imagery: dark glass
    // + blur + inset hairline. Declared AFTER `variant` so its bg wins the
    // tailwind-merge conflict resolution; the variant's accent TEXT colour is
    // preserved (text/bg are separate merge groups). Pair with:
    //   variant="warning" → amber ★ (MAL score)
    //   variant="primary" → cyan  ◆ (AnimeEnigma score)
    //   variant="success" → green   (ONGOING)
    //   variant="default" → white   (quality / neutral)
    overlay: {
      true: 'bg-black/[0.62] backdrop-blur-[6px] ring-1 ring-inset ring-white/10',
      false: '',
    },
  },
  defaultVariants: { variant: 'default', size: 'md', overlay: false },
})

export type BadgeVariants = VariantProps<typeof badgeVariants>
