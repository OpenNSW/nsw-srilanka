import type { Locale } from '@/i18n/types'

// Sinhala. Keys not yet translated by PIU fall back to English (fallbackLng in ./index.ts).
const si: Locale = {
  label: 'සිංහල',
  translation: {
    auth: {
      userFallback: 'පරිශීලකයා',
      login: {
        tagline: 'ඔබගේ භාණ්ඩ තොග (consignments) වෙත ප්‍රවේශවීමට පුරනය වන්න',
        button: 'පුරන්න',
      },
      unauthorized: {
        title: 'ප්‍රවේශය සීමාකර තිබේ.',
        signOut: 'පිටවන්න',
      },
    },

    nav: {
      consignments: 'භාණ්ඩ තොග',
    },

    consignments: {
      state: {
        inProgress: 'ක්‍රියාත්මක වෙමින් පවතී',
        finished: 'අවසන් කරන ලදී',
      },
      tradeFlow: {
        import: 'ආනයන',
        export: 'අපනයන',
      },
      list: {
        create: 'නව භාණ්ඩ තොගයක්',
        creating: 'සාදමින්...',
      },
      detail: {
        backToList: 'භාණ්ඩ තොග වෙත ආපසු',
        tryAgain: 'නැවත උත්සාහ කරන්න',
        refresh: 'නැවුම් කරන්න',
        refreshing: 'නැවුම් කරමින්...',
        flows: {
          cancel: 'අවලංගු කරන්න',
          start: 'අරඹන්න',
        },
      },
    },

    preconsignment: {
      action: {
        start: 'අරඹන්න',
        view: 'බලන්න',
        continue: 'ඉදිරියට',
      },
    },

    tasks: {
      goBack: 'ආපසු යන්න',
      refresh: 'නැවුම් කරන්න',
      savingDraft: 'සුරකිමින්…',
      submitting: 'ඉදිරිපත් කරමින්...',
      unsaved: {
        stay: 'මෙම පිටුවේ රැඳී සිටින්න',
        leave: 'නොසුරකා ඉවත් වන්න',
      },
      claim: {
        claim: 'භාරගැනීම',
        release: 'නිකුත් කිරීම',
        cancel: 'අවලංගු කිරීම',
        claimedByYou: 'ඔබ විසින් භාරගන්නා ලදී.',
        unclaimed: 'භාරගෙන නොමැත',
      },
    },

    footer: {
      links: {
        policy: 'ප්‍රතිපත්තිය',
        accessibility: 'ප්‍රවේශ්‍යතාවය',
        support: 'සහාය',
      },
    },

    common: {
      pagination: {
        previous: 'පෙර',
        next: 'ඊළඟ',
      },
      error: {
        tryAgain: 'නැවත උත්සාහ කරන්න',
      },
    },
  },
}

export default si
