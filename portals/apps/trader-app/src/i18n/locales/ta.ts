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
      menu: 'பட்டியல்',
      consignments: 'அனுப்புப்பொருள்',
      cases: 'வழக்குகள்',
      verifiedDocs: 'சரிபார்க்கப்பட்ட ஆவணங்கள்',
    },

    cases: {
      list: {
        title: 'வழக்குகள்',
        table: {
          id: 'வழக்கு',
          state: 'நிலை',
          created: 'உருவாக்கப்பட்டது',
        },
      },
      detail: {
        title: 'வழக்கு',
        backToList: 'வழக்குகளுக்குத் திரும்பு',
        field: {
          caseId: 'வழக்கு எண்',
        },
      },
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
        title: 'அனுப்புப்பொருள்',
        create: 'புதிய சரக்கு',
        creating: 'உருவாக்குகிறது...',
        searchPlaceholder: 'பெயர், எண் அல்லது HS குறியீட்டின் மூலம் தேடுக...',
        filter: {
          statePlaceholder: 'நிலை',
          allStates: 'அனைத்து நிலைகளும்',
          inProgress: 'செயல்பாட்டில் உள்ளது',
          finished: 'முடிந்தது',
          tradeFlowPlaceholder: 'வர்த்தக ஓட்டம்',
          allTypes: 'அனைத்து வகைகளும்',
          import: 'இறக்குமதி',
          export: 'ஏற்றுமதி',
        },
        table: {
          id: 'அனுப்புப்பொருள்',
          tradeFlow: 'வர்த்தக ஓட்டம்',
          state: 'நிலை',
          created: 'உருவாக்கப்பட்டது',
        },
      },
      detail: {
        title: 'அனுப்புப்பொருள் பார்வை',
        backToList: 'சரக்குகளுக்குத் திரும்பு',
        tryAgain: 'மீண்டும் முயற்சிக்கவும்',
        refresh: 'புதுப்பி',
        refreshing: 'புதுப்பிக்கிறது...',
        field: {
          consignmentId: 'அனுப்புப்பொருள் எண்',
          dateCreated: 'உருவாக்கப்பட்ட தேதி',
        },
        flows: {
          confirmTitle: '{{label}} ஐத் தொடங்கவா?',
          confirmDescription:
            'இது அனுப்புப்பொருளில் {{label}} ஐத் தொடங்குகிறது. அதன் படிகள் கீழே உள்ள பட்டியலில் தோன்றும்.',
          cancel: 'ரத்துசெய்',
          start: 'தொடங்கு',
        },
        noWorkflow: {
          title: 'பணிப்பாய்வுப் படிகள் இல்லை',
          description: 'இந்த அனுப்புப்பொருளுக்கு பணிப்பாய்வுப் படிகள் எதுவும் கட்டமைக்கப்படவில்லை.',
        },
      },
    },

    preconsignment: {
      title: 'சரிபார்க்கப்பட்ட ஆவணங்கள்',
      action: {
        start: 'தொடங்கு',
        view: 'காண்க',
        continue: 'தொடர்க',
        dependentWarning: 'முதலில் சார்ந்திருக்கும் முன்-அனுப்புப்பொருட்களை முடிக்கவும்',
      },
    },

    tasks: {
      goBack: 'பின்செல்',
      refresh: 'புதுப்பி',
      taskId: 'பணி எண்',
      continueTo: '{{name}} க்குத் தொடர்க',
      continueToNext: 'அடுத்த பணிக்குத் தொடர்க',
      backToConsignment: 'அனுப்புப்பொருளுக்குத் திரும்பு',
      backToCase: 'வழக்கிற்குத் திரும்பு',
      draftSaved: 'வரைவு சேமிக்கப்பட்டது',
      savingDraft: 'சேமிக்கிறது…',
      submitting: 'சமர்ப்பிக்கிறது...',
      handles: {
        saveAsDraft: 'வரைவாகச் சேமிக்கவும்',
        continue: 'தொடர்க',
        saveAndContinue: 'சேமித்துத் தொடர்க',
        completeSelection: 'தேர்வை முடிக்கவும்',
        proceedToPayment: 'கட்டணம் செலுத்த தொடரவும்',
        submitApplication: 'விண்ணப்பத்தைச் சமர்ப்பிக்கவும்',
        submitDecision: 'முடிவைச் சமர்ப்பிக்கவும்',
        done: 'முடிந்தது',
        uploadDocuments: 'ஆவணங்களைப் பதிவேற்றவும்',
        submitDocuments: 'ஆவணங்களைச் சமர்ப்பிக்கவும்',
        submit: 'சமர்ப்பிக்கவும்',
        scheduleInspection: 'ஆய்வைத் திட்டமிடுங்கள்',
        acknowledgeAmendment: 'திருத்தத்தை ஏற்கவும்',
        consolidate: 'ஒருங்கிணைக்கவும்',
        lookAgain: 'மீண்டும் பார்க்கவும்',
        verify: 'சரிபார்க்கவும்',
        submitBlendSheet: 'கலவை தாளைச் சமர்ப்பிக்கவும்',
        submitBoatNote: 'படகு குறிப்பைச் சமர்ப்பிக்கவும்',
        submitCDNApplication: 'CDN விண்ணப்பத்தைச் சமர்ப்பிக்கவும்',
        submitCertificates: 'சான்றிதழ்களைச் சமர்ப்பிக்கவும்',
        submitCusDecApplication: 'CusDec விண்ணப்பத்தைச் சமர்ப்பிக்கவும்',
        submitInspectionSchedule: 'ஆய்வு அட்டவணையைச் சமர்ப்பிக்கவும்',
        submitPaymentSlip: 'கட்டணச் சீட்டைச் சமர்ப்பிக்கவும்',
        submitReceipt: 'பற்றுச்சீட்டைச் சமர்ப்பிக்கவும்',
        submitResponse: 'பதிலைச் சமர்ப்பிக்கவும்',
        submitTreatmentRequest: 'சிகிச்சை கோரிக்கையைச் சமர்ப்பிக்கவும்',
        submitInspectionOutcome: 'ஆய்வு முடிவைச் சமர்ப்பிக்கவும்',
        submitToExportDivision: 'ஏற்றுமதிப் பிரிவுக்குச் சமர்ப்பிக்கவும்',
        fetchPreAdvisedContainers: 'முன்னறிவிக்கப்பட்ட கொள்கலன்களைப் பெறவும்',
        generateInvoice: 'விலைப் பட்டியலை உருவாக்கவும்',
        raiseServiceOrder: 'சேவை ஆர்டரை உருவாக்கவும்',
        raiseSundryOrder: 'இதர ஆர்டரை உருவாக்கவும்',
      },
      unsaved: {
        title: 'சேமிக்காமல் வெளியேறவா?',
        description: 'இந்தப் படிவத்தில் உங்கள் பதில்கள் சேமிக்கப்படவில்லை. இப்போது வெளியேறினால் அவை இழக்கப்படும்.',
        stay: 'இந்தப் பக்கத்திலேயே இருக்கவும்',
        leave: 'சேமிக்காமல் வெளியேறு',
      },
      claim: {
        claim: 'கோரவும்',
        release: 'விடுவிக்கவும்',
        takeOver: 'பொறுப்பேற்கவும்',
        takeOverTitle: 'இந்தப் பணியைப் பொறுப்பேற்கவா?',
        cancel: 'இரத்துச்செய்',
        claimedByYou: 'உங்களால் கோரப்பட்டது',
        claimedBy: '{{name}} ஆல் கோரப்பட்டது',
        anotherUser: 'மற்றொரு பயனர்',
        unclaimed: 'கோரப்படவில்லை',
      },
    },

    workflow: {
      tabs: {
        active: 'செயலில்',
        done: 'முடிந்தது',
      },
      actionRequired: 'நடவடிக்கை தேவை',
      waitingOnOthers: 'பிறருக்காக காத்திருக்கிறது',
      untitledTask: 'பெயரிடப்படாத பணி',
      continue: 'தொடர்க',
      continueTask: '{{name}} ஐத் தொடர்க',
      view: 'காண்க',
      viewTask: '{{name}} ஐக் காண்க',
      completedAt: 'முடிவடைந்தது {{date}}',
      since: '{{date}} முதல்',
      nothingNeeded: 'தற்போது உங்கள் நடவடிக்கை எதுவும் தேவையில்லை.',
      newStepsAppear: 'உங்கள் விண்ணப்பங்கள் மதிப்பாய்வு செய்யப்படும்போது புதிய படிகள் இங்கு தோன்றும்.',
      newStepsAppearCase: 'வழக்கு முன்னேறும்போது புதிய படிகள் இங்கு தோன்றும்.',
      consignmentComplete: 'இந்த அனுப்புப்பொருள் முழுமையடைந்தது.',
      noneDone: 'இன்னும் எந்தப் பணிகளும் முடியவில்லை.',
      status: {
        completed: 'முடிந்தது',
        ready: 'தயார்',
        inProgress: 'செயல்பாட்டில் உள்ளது',
        locked: 'பூட்டப்பட்டுள்ளது',
        failed: 'தோல்வியடைந்தது',
        awaitingFeedback: 'கருத்துக்காக காத்திருக்கிறது',
        cancelled: 'இரத்து செய்யப்பட்டது',
      },
    },

    audit: {
      title: 'செயல்பாடு',
    },

    footer: {
      links: {
        policy: 'கொள்கை',
        accessibility: 'அணுகல்தன்மை',
        support: 'ஆதரவு',
      },
    },

    common: {
      colorScheme: 'வண்ணத் திட்டம்',
      breadcrumb: 'வழிகாட்டி (Breadcrumb)',
      copied: 'நகலெடுக்கப்பட்டது',
      copyNamed: '{{name}} ஐ நகலெடு',
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
