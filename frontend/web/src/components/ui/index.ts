export { default as Badge } from './Badge.vue'
export { badgeVariants, type BadgeVariants } from './badge-variants'
export { default as Button } from './Button.vue'
export { buttonVariants, type ButtonVariants } from './button-variants'
export { default as ButtonGroup } from './ButtonGroup.vue'
export { default as Card } from './Card.vue'
export { default as Checkbox } from './Checkbox.vue'
export { default as CardHeader } from './CardHeader.vue'
export { default as CardTitle } from './CardTitle.vue'
export { default as CardContent } from './CardContent.vue'
export { default as CardFooter } from './CardFooter.vue'
export { default as DatePicker } from './DatePicker.vue'
export { default as DropdownMenu } from './DropdownMenu.vue'
// Reka DropdownMenu sub-parts re-exported so consumers (Plan 04 anime-card kebab)
// can compose the menu body inside <DropdownMenu>'s default slot without importing
// reka-ui directly. Item/Separator/Label are the parts Plan 04 needs.
export {
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuLabel,
} from 'reka-ui'
export { default as GenreFilterPopup } from './GenreFilterPopup.vue'
export { default as Input } from './Input.vue'
export { default as Modal } from './Modal.vue'
export { default as Dialog } from './Modal.vue'
export { default as PaginationBar } from './PaginationBar.vue'
export { default as Popover } from './Popover.vue'
export { default as RadioGroup } from './RadioGroup.vue'
export { default as SearchAutocomplete } from './SearchAutocomplete.vue'
export { default as ScoreDiamond } from './ScoreDiamond.vue'
export { default as Select } from './Select.vue'
export { default as Skeleton } from './Skeleton.vue'
export { default as Switch } from './Switch.vue'
export { default as Tabs } from './Tabs.vue'
export { default as SegmentedControl } from './SegmentedControl.vue'
export {
  segmentedControlVariants,
  segmentVariants,
  type SegmentedControlVariants,
} from './segmented-control-variants'
export { default as Tooltip } from './Tooltip.vue'
export { default as Spinner } from './Spinner.vue'
export { spinnerVariants, type SpinnerVariants } from './spinner-variants'
export { default as Stepper } from './Stepper.vue'
export { default as LoadingState } from './LoadingState.vue'
export { default as Alert } from './Alert.vue'
export { alertVariants, type AlertVariants } from './alert-variants'
export { default as Avatar } from './Avatar.vue'
export { avatarVariants, type AvatarVariants } from './avatar-variants'
export { default as AvatarGroup } from './AvatarGroup.vue'
export { default as EmptyState } from './EmptyState.vue'
export { emptyStateVariants, type EmptyStateVariants } from './empty-state-variants'
export { default as ConfirmDialog } from './ConfirmDialog.vue'
export { default as ConfirmDialogHost } from './ConfirmDialogHost.vue'
export type { ConfirmOptions } from '@/composables/useConfirm'

// Re-export SelectOption type
export interface SelectOption {
  value: string | number
  label: string
}
