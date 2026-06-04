import { cva, type VariantProps } from 'class-variance-authority'

export const buttonVariants = cva(
  'inline-flex items-center justify-center gap-2 whitespace-nowrap font-medium transition-all duration-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50 disabled:cursor-not-allowed disabled:pointer-events-none [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0',
  {
    variants: {
      variant: {
        default: 'bg-primary rounded-xl hover:bg-brand-cyan hover:shadow-glow-cyan active:scale-95',
        brand: 'bg-brand-pink text-brand-pink-foreground rounded-xl hover:bg-pink-400 hover:shadow-glow-pink active:scale-95',
        ghost: 'bg-white/5 hover:bg-white/10 text-white rounded-lg border border-white/10 hover:border-white/20',
        outline: 'bg-transparent hover:bg-white/5 text-cyan-400 rounded-xl border border-cyan-400/50 hover:border-cyan-400',
        destructive: 'bg-destructive text-destructive-foreground rounded-xl hover:bg-destructive/90 active:scale-95',
        soft: 'bg-white/10 hover:bg-white/20 text-white rounded-lg',
        link: 'bg-transparent text-cyan-400 hover:text-cyan-300 hover:underline underline-offset-4',
        // DS-NF-04 back-compat aliases — mirror default/brand so old call-sites are unchanged:
        primary: 'bg-primary rounded-xl hover:bg-brand-cyan hover:shadow-glow-cyan active:scale-95',
        secondary: 'bg-brand-pink text-brand-pink-foreground rounded-xl hover:bg-pink-400 hover:shadow-glow-pink active:scale-95',
      },
      size: {
        xs: 'px-2 py-1 text-xs',
        sm: 'px-3 py-1.5 text-sm',
        md: 'px-6 py-3 text-base',
        lg: 'px-8 py-4 text-lg',
        icon: 'h-10 w-10 p-0',
        'icon-sm': 'h-8 w-8 p-0',
      },
    },
    compoundVariants: [
      { variant: 'link', class: 'px-0! py-0! h-auto' },
    ],
    defaultVariants: { variant: 'default', size: 'md' },
  },
)

export type ButtonVariants = VariantProps<typeof buttonVariants>
