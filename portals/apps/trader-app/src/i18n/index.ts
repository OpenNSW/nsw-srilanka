import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import LanguageDetector from 'i18next-browser-languagedetector'
import en from './locales/en'


// `resources` and `languageLabels` are derived from the same map and won't drift apart.
// Add a language by creating <lang>.ts under ./locales (matching the shape
// of en.ts) and registering it here.
const locales = {
  en,
}

export const languageLabels: Record<string, string> = Object.fromEntries(
  Object.entries(locales).map(([code, locale]) => [code, locale.label]),
)

const resources = Object.fromEntries(
  Object.entries(locales).map(([code, locale]) => [code, { translation: locale.translation }]),
)

export const supportedLanguages = Object.keys(resources)

// Keeps <html lang>/[dir] in sync with the active language for accessibility,
// font/hyphenation selection, and SEO.
function syncDocumentLanguage(language: string) {
  document.documentElement.lang = language
  document.documentElement.dir = i18n.dir(language)
}

i18n.on('languageChanged', syncDocumentLanguage)

void i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources,
    // Normalizes detected browser tags like `en-US` or `si-LK` down to a
    // registered locale code instead of leaving them unresolved.
    supportedLngs: supportedLanguages,
    fallbackLng: 'en',
    detection: {
      order: ['localStorage', 'navigator'],
      caches: ['localStorage'],
    },
    interpolation: {
      escapeValue: false,
    },
  })
  .then(() => {
    syncDocumentLanguage(i18n.resolvedLanguage ?? i18n.language)
  })

export default i18n
