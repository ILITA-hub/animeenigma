import { ref } from 'vue'

/**
 * Reactive mirror of the active vue-i18n interface locale. `src/i18n.ts`'s
 * `setLocale` is the single writer, called on boot and from the Navbar
 * language switch.
 *
 * Utilities outside component templates (utils/title.ts, toCardModel.ts) read
 * this instead of importing the real `@/i18n` singleton directly — importing
 * that module runs `createI18n()` at eval time, which breaks the ~19 existing
 * specs that mock vue-i18n with a bare object
 * (project_vue_i18n_mock_createi18n_barrel_trap). Components should keep
 * reading the locale via vue-i18n's own `useI18n().locale`, not this ref.
 */
export const interfaceLocale = ref<string>('ru')

export function setInterfaceLocale(code: string): void {
  interfaceLocale.value = code
}
