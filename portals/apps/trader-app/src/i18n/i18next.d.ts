// Extends i18next types with our translation keys — gives t() full autocomplete and compile-time key validation.
import 'i18next'
import type en from './locales/en'

declare module 'i18next' {
  interface CustomTypeOptions {
    defaultNS: 'translation'
    resources: {
      translation: typeof en
    }
  }
}
