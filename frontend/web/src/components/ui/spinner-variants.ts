import { cva, type VariantProps } from 'class-variance-authority'

// Dual counter-rotating arc spinner. The rings are drawn with pure-CSS
// pseudo-elements in Spinner.vue; this cva only maps size + tone to the
// marker classes those scoped styles key off of.
export const spinnerVariants = cva('ae-spinner inline-block align-middle', {
  variants: {
    size: {
      xs: 'ae-spinner--xs',
      sm: 'ae-spinner--sm',
      md: 'ae-spinner--md',
      lg: 'ae-spinner--lg',
      xl: 'ae-spinner--xl',
    },
    tone: {
      signature: 'ae-spinner--signature',
      mono: 'ae-spinner--mono',
    },
  },
  defaultVariants: { size: 'md', tone: 'signature' },
})

export type SpinnerVariants = VariantProps<typeof spinnerVariants>
