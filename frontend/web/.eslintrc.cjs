/* eslint-env node */
module.exports = {
  root: true,
  extends: [
    'plugin:vue/vue3-essential',
    'eslint:recommended',
    'plugin:@typescript-eslint/recommended'
  ],
  parser: 'vue-eslint-parser',
  parserOptions: {
    parser: '@typescript-eslint/parser',
    ecmaVersion: 'latest',
    sourceType: 'module'
  },
  plugins: ['@typescript-eslint', '@intlify/vue-i18n'],
  // Tell @intlify/vue-i18n where the locale message files live so its rules
  // (incl. valid-message-syntax) can resolve them.
  settings: {
    'vue-i18n': {
      localeDir: './src/locales/*.{json,yaml,yml}',
      messageSyntaxVersion: '^11.0.0'
    }
  },
  rules: {
    'vue/multi-word-component-names': 'off',
    '@typescript-eslint/no-explicit-any': 'warn',
    '@typescript-eslint/no-unused-vars': ['error', { argsIgnorePattern: '^_', varsIgnorePattern: '^_' }],
    // Catches the regression that broke /auth on 2026-05-11 — '@foo' in a
    // message body looked like the linked-message operator (@:other.key) to
    // the message compiler and threw at render time. This rule runs the same
    // compiler at lint time so we never ship one again.
    '@intlify/vue-i18n/valid-message-syntax': 'error'
  },
  overrides: [
    // Locale JSON files need the JSONC parser so the i18n plugin can walk
    // their AST. Without this override eslint falls back to espree and
    // refuses to parse JSON.
    {
      files: ['src/locales/*.json'],
      parser: 'jsonc-eslint-parser'
    }
  ]
}
