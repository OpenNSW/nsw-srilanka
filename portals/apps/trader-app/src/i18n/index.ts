import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import LanguageDetector from 'i18next-browser-languagedetector'
import en from './locales/en'

const resources = {
  en: { translation: en },
}

// Add a language by creating <lang>.ts under ./locales, registering it in
// `resources` and `languageLabels` here. LanguageSwitcher hides itself while
// there's only one entry, so there's no config to toggle beyond this.
export const languageLabels: Record<string, string> = {
  en: 'English',
}

export const supportedLanguages = Object.keys(resources)

void i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources,
    fallbackLng: 'en',
    detection: {
      order: ['localStorage', 'navigator'],
      caches: ['localStorage'],
    },
    interpolation: {
      escapeValue: false,
    },
  })

export default i18n
