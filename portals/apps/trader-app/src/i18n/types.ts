import type en from './locales/en'

type DeepPartial<T> = { [K in keyof T]?: T[K] extends string ? string : DeepPartial<T[K]> }

// A language other than English. It may translate only some of en.ts: a key it
// leaves out is shown in English (fallbackLng in ./index.ts).
export type Locale = {
  label: string
  translation: DeepPartial<typeof en.translation>
}
