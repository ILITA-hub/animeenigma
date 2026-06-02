import { cva, type VariantProps } from 'class-variance-authority'

export const buttonVariants = cva(
  'inline-flex items-center justify-center gap-2 whitespace-nowrap font-medium transition-all duration-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50 disabled:cursor-not-allowed disabled:pointer-events-none [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0',
  {
    variants: {
      variant: {
        default: 'bg-primary rounded-xl hover:bg-brand-cyan hover:shadow-[0_0_30px_rgba(0,212,255,0.3)] active:scale-95',
        brand: 'bg-brand-pink text-brand-pink-foreground rounded-xl hover:bg-pink-400 hover:shadow-[0_0_30px_rgba(255,45,124,0.3)] active:scale-95',
        ghost: 'bg-white/5 hover:bg-white/10 text-white rounded-lg border border-white/10 hover:border-white/20',
        outline: 'bg-transparent hover:bg-white/5 text-cyan-400 rounded-xl border border-cyan-400/50 hover:border-cyan-400',
        destructive: 'bg-destructive text-destructive-foreground rounded-xl hover:bg-destructive/90 active:scale-95',
        // DS-NF-04 legacy aliases — duplicate the default/brand class strings so old call-sites are unchanged:
        primary: 'bg-primary rounded-xl hover:bg-brand-cyan hover:shadow-[0_0_30px_rgba(0,212,255,0.3)] active:scale-95',
        secondary: 'bg-brand-pink text-brand-pink-foreground rounded-xl hover:bg-pink-400 hover:shadow-[0_0_30px_rgba(255,45,124,0.3)] active:scale-95',
      },
      size: {
        sm: 'px-3 py-1.5 text-sm',
        md: 'px-6 py-3 text-base',
        lg: 'px-8 py-4 text-lg',
        icon: 'h-10 w-10 p-0',
      },
    },
    defaultVariants: { variant: 'default', size: 'md' },
  },
)

export type ButtonVariants = VariantProps<typeof buttonVariants>
