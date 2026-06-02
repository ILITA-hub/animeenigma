export { default as Badge } from './Badge.vue'
export { default as Button } from './Button.vue'
export { buttonVariants, type ButtonVariants } from './button-variants'
export { default as ButtonGroup } from './ButtonGroup.vue'
export { default as Card } from './Card.vue'
export { default as CardHeader } from './CardHeader.vue'
export { default as CardTitle } from './CardTitle.vue'
export { default as CardContent } from './CardContent.vue'
export { default as CardFooter } from './CardFooter.vue'
export { default as GenreFilterPopup } from './GenreFilterPopup.vue'
export { default as Input } from './Input.vue'
export { default as Modal } from './Modal.vue'
export { default as PaginationBar } from './PaginationBar.vue'
export { default as SearchAutocomplete } from './SearchAutocomplete.vue'
export { default as Select } from './Select.vue'
export { default as Skeleton } from './Skeleton.vue'
export { default as Tabs } from './Tabs.vue'

// Re-export SelectOption type
export interface SelectOption {
  value: string | number
  label: string
}
