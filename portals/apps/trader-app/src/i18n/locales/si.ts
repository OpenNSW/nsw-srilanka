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
      menu: 'මෙනුව',
      consignments: 'භාණ්ඩ තොග',
      cases: 'නඩු/ගොනු',
      verifiedDocs: 'තහවුරු කළ ලියකියවිලි',
    },

    cases: {
      list: {
        title: 'නඩු/ගොනු',
        table: {
          id: 'නඩුව/ගොනුව',
          state: 'තත්ත්වය',
          created: 'සාදන ලද දිනය',
        },
      },
      detail: {
        title: 'නඩුව/ගොනුව',
        backToList: 'නඩු/ගොනු වෙත ආපසු',
        field: {
          caseId: 'නඩු/ගොනු අංකය',
        },
      },
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
        title: 'භාණ්ඩ තොග',
        create: 'නව භාණ්ඩ තොගයක්',
        creating: 'සාදමින්...',
        searchPlaceholder: 'නම, අංකය, හෝ HS කේතය මගින් සොයන්න...',
        filter: {
          statePlaceholder: 'තත්ත්වය',
          allStates: 'සියලු තත්ත්වයන්',
          inProgress: 'ක්‍රියාත්මක වෙමින් පවතී',
          finished: 'අවසන් කරන ලදී',
          tradeFlowPlaceholder: 'වෙළඳ ප්‍රවාහය',
          allTypes: 'සියලු වර්ග',
          import: 'ආනයන',
          export: 'අපනයන',
        },
        table: {
          id: 'භාණ්ඩ තොගය',
          tradeFlow: 'වෙළඳ ප්‍රවාහය',
          state: 'තත්ත්වය',
          created: 'සාදන ලද දිනය',
        },
      },
      detail: {
        title: 'භාණ්ඩ තොග විස්තරය',
        backToList: 'භාණ්ඩ තොග වෙත ආපසු',
        tryAgain: 'නැවත උත්සාහ කරන්න',
        refresh: 'නැවුම් කරන්න',
        refreshing: 'නැවුම් කරමින්...',
        field: {
          consignmentId: 'භාණ්ඩ තොග අංකය',
          dateCreated: 'සාදන ලද දිනය',
        },
        flows: {
          confirmTitle: '{{label}} ආරම්භ කරන්නද?',
          confirmDescription: 'මෙයින් භාණ්ඩ තොගය සඳහා {{label}} ආරම්භ වේ. එහි පියවර පහත ලැයිස්තුවේ දිස්වේ.',
          cancel: 'අවලංගු කරන්න',
          start: 'අරඹන්න',
        },
        noWorkflow: {
          title: 'කාර්ය ප්‍රවාහ පියවර නොමැත',
          description: 'මෙම භාණ්ඩ තොගය සඳහා කාර්ය ප්‍රවාහ පියවර කිසිවක් සකසා නොමැත.',
        },
      },
    },

    preconsignment: {
      title: 'තහවුරු කළ ලියකියවිලි',
      action: {
        start: 'අරඹන්න',
        view: 'බලන්න',
        continue: 'ඉදිරියට',
        dependentWarning: 'පළමුව අදාළ පූර්ව භාණ්ඩ තොග සම්පූර්ණ කරන්න',
      },
    },

    tasks: {
      goBack: 'ආපසු යන්න',
      refresh: 'නැවුම් කරන්න',
      taskId: 'කාර්ය අංකය',
      continueTo: '{{name}} වෙත ඉදිරියට යන්න',
      continueToNext: 'ඊළඟ කාර්යය වෙත ඉදිරියට යන්න',
      backToConsignment: 'භාණ්ඩ තොගය වෙත ආපසු',
      backToCase: 'නඩුව වෙත ආපසු',
      draftSaved: 'කෙටුම්පත සුරකින ලදී',
      savingDraft: 'සුරකිමින්…',
      submitting: 'ඉදිරිපත් කරමින්...',
      unsaved: {
        title: 'සුරැකීමකින් තොරව ඉවත් වෙනවාද?',
        description: 'මෙම පෝරමයේ ඔබේ පිළිතුරු සුරැකී නොමැත. ඔබ දැන් පිටව ගියහොත් ඒවා නැතිවී යනු ඇත.',
        stay: 'මෙම පිටුවේ රැඳී සිටින්න',
        leave: 'නොසුරකා ඉවත් වන්න',
      },
      claim: {
        claim: 'භාරගැනීම',
        release: 'නිකුත් කිරීම',
        takeOver: 'භාරයට ගන්න',
        takeOverTitle: 'මෙම කාර්යය ඔබේ භාරයට ගන්නද?',
        cancel: 'අවලංගු කිරීම',
        claimedByYou: 'ඔබ විසින් භාරගන්නා ලදී.',
        claimedBy: '{{name}} විසින් භාරගන්නා ලදී',
        anotherUser: 'වෙනත් පරිශීලකයෙක්',
        unclaimed: 'භාරගෙන නොමැත',
      },
    },

    workflow: {
      tabs: {
        active: 'සක්‍රිය',
        done: 'අවසන්',
      },
      actionRequired: 'ක්‍රියාමාර්ග අවශ්‍යයි',
      waitingOnOthers: 'අනෙක් අයගේ ප්‍රතිචාර අපේක්ෂාවෙන්',
      untitledTask: 'නම් නොකළ කාර්යය',
      continue: 'ඉදිරියට',
      continueTask: '{{name}} වෙත ඉදිරියට',
      view: 'බලන්න',
      viewTask: '{{name}} බලන්න',
      completedAt: 'අවසන් කළේ {{date}}',
      since: '{{date}} සිට',
      nothingNeeded: 'දැනට ඔබගේ ක්‍රියාමාර්ගයක් අවශ්‍ය නොවේ.',
      newStepsAppear: 'ඔබේ අයදුම්පත් සමාලෝචනය වන විට නව පියවර මෙහි දිස්වනු ඇත.',
      newStepsAppearCase: 'නඩුව ඉදිරියට යන විට නව පියවර මෙහි දිස්වනු ඇත.',
      consignmentComplete: 'මෙම භාණ්ඩ තොගය සම්පූර්ණයි.',
      noneDone: 'තවමත් අවසන් කළ කාර්යයන් නොමැත.',
      status: {
        completed: 'අවසන් කරන ලදී',
        ready: 'සූදානම්',
        inProgress: 'ක්‍රියාත්මක වෙමින් පවතී',
        locked: 'අගුළු දමා ඇත',
        failed: 'අසාර්ථක විය',
        awaitingFeedback: 'ප්‍රතිපෝෂණ අපේක්ෂාවෙන්',
        cancelled: 'අවලංගු කරන ලදී',
      },
    },

    audit: {
      title: 'ක්‍රියාකාරකම්',
    },

    footer: {
      links: {
        policy: 'ප්‍රතිපත්තිය',
        accessibility: 'ප්‍රවේශ්‍යතාවය',
        support: 'සහාය',
      },
    },

    common: {
      colorScheme: 'වර්ණ තේමාව',
      breadcrumb: 'මාර්ගය (Breadcrumb)',
      copied: 'පිටපත් කරන ලදී',
      copyNamed: '{{name}} පිටපත් කරන්න',
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
