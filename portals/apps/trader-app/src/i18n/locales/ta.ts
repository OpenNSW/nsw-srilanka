import type { Locale } from '@/i18n/types'

// Tamil. Keys not yet translated by PIU fall back to English (fallbackLng in ./index.ts).
const ta: Locale = {
  label: 'தமிழ்',
  translation: {
    auth: {
      userFallback: 'பயனர்',
      login: {
        tagline: 'உங்கள் அனுப்புப்பொருள் விவரங்களைத் தொடர்ந்து பார்க்க உள்நுழையவும்.',
        button: 'உள்நுழைக',
      },
      unauthorized: {
        title: 'அணுகல் கட்டுப்படுத்தப்பட்டுள்ளது',
        signOut: 'வெளியேறு',
      },
    },

    nav: {
      consignments: 'அனுப்புப்பொருள்',
    },

    consignments: {
      state: {
        inProgress: 'செயல்பாட்டில் உள்ளது',
        finished: 'முடிந்தது',
      },
      tradeFlow: {
        import: 'இறக்குமதி',
        export: 'ஏற்றுமதி',
      },
      list: {
        create: 'புதிய சரக்கு',
        creating: 'உருவாக்குகிறது...',
      },
      detail: {
        backToList: 'சரக்குகளுக்குத் திரும்பு',
        tryAgain: 'மீண்டும் முயற்சிக்கவும்',
        refresh: 'புதுப்பி',
        refreshing: 'புதுப்பிக்கிறது...',
        flows: {
          cancel: 'ரத்துசெய்',
          start: 'தொடங்கு',
        },
      },
    },

    preconsignment: {
      action: {
        start: 'தொடங்கு',
        view: 'காண்க',
        continue: 'தொடர்க',
      },
    },

    tasks: {
      goBack: 'பின்செல்',
      refresh: 'புதுப்பி',
      savingDraft: 'சேமிக்கிறது…',
      submitting: 'சமர்ப்பிக்கிறது...',
      unsaved: {
        stay: 'இந்தப் பக்கத்திலேயே இருக்கவும்',
        leave: 'சேமிக்காமல் வெளியேறு',
      },
      claim: {
        claim: 'கோரவும்',
        release: 'விடுவிக்கவும்',
        cancel: 'இரத்துச்செய்',
        claimedByYou: 'உங்களால் கோரப்பட்டது',
        unclaimed: 'கோரப்படவில்லை',
      },
    },

    footer: {
      links: {
        policy: 'கொள்கை',
        accessibility: 'அணுகல்தன்மை',
        support: 'ஆதரவு',
      },
    },

    common: {
      pagination: {
        previous: 'முந்தையது',
        next: 'அடுத்தது',
      },
      error: {
        tryAgain: 'மீண்டும் முயற்சிக்கவும்',
      },
    },
  },
}

export default ta
