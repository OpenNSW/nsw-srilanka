const en = {
  label: 'English',
  translation: {
    auth: {
      login: {
        tagline: 'Sign in to continue to your consignments.',
        button: 'Sign In',
        errorTitle: 'Authentication Failed',
      },
      unauthorized: {
        title: 'Access Restricted',
        message: 'Your account is signed in, but it does not currently have an application role.',
        signOut: 'Sign out',
      },
    },

    nav: {
      menu: 'Menu',
      consignments: 'Consignments',
      cases: 'Cases',
      verifiedDocs: 'Verified Docs',
    },

    cases: {
      list: {
        loading: 'Loading Cases...',
        title: 'Cases',
        empty: 'No cases yet.',
        error: 'Could not load cases.',
        table: {
          id: 'Case',
          state: 'State',
          created: 'Created',
        },
      },
      detail: {
        title: 'Case',
        backToList: 'Back to Cases',
        field: {
          caseId: 'Case ID',
        },
      },
    },

    consignments: {
      list: {
        loading: 'Loading Consignments...',
        title: 'Consignments',
        create: 'New Consignment',
        creating: 'Creating...',
        error: {
          loadFailed: 'Could not load consignments.',
          createFailed: 'Could not create the consignment. Please try again.',
        },
        searchPlaceholder: 'Search by Name, ID, or HS Code...',
        filter: {
          statePlaceholder: 'State',
          allStates: 'All States',
          inProgress: 'In Progress',
          finished: 'Finished',
          tradeFlowPlaceholder: 'Trade Flow',
          allTypes: 'All Types',
          import: 'Import',
          export: 'Export',
        },
        empty: {
          none: 'No consignments yet.',
          filtered: 'No consignments match your filters.',
        },
        table: {
          id: 'Consignment',
          tradeFlow: 'Trade Flow',
          state: 'State',
          created: 'Created',
        },
      },
      detail: {
        loading: {
          processing: 'Processing your submission...',
          consignment: 'Loading consignment...',
          settingUp: 'Setting up your consignment...',
        },
        backToList: 'Back to Consignments',
        tryAgain: 'Try Again',
        refresh: 'Refresh',
        refreshing: 'Refreshing...',
        title: 'Consignment View',
        field: {
          consignmentId: 'Consignment ID',
          dateCreated: 'Date Created',
        },
        flows: {
          confirmTitle: 'Start {{label}}?',
          confirmDescription: 'This starts {{label}} on the consignment. Its steps appear in the list below.',
          cancel: 'Cancel',
          start: 'Start',
          failed: 'Could not start {{label}}. Please try again.',
        },
        noWorkflow: {
          title: 'No Workflow Steps',
          description: "This consignment doesn't have any workflow steps configured.",
        },
        error: {
          idRequired: 'Consignment ID is required',
          notFound: 'Consignment not found',
          loadFailed: 'Failed to load consignment',
          loadFailedDescription: 'There was a problem loading the consignment details. Please try again.',
          notFoundDescription: "The consignment you're looking for doesn't exist or you don't have access to it.",
        },
      },
    },

    preconsignment: {
      title: 'Verified Documents',
      error: {
        loadFailed: 'Failed to load pre-consignments list.',
        noReadyTask: 'No ready task found in pre-consignment.',
        startFailed: 'Failed to start registration process.',
        noTask: 'No task found in pre-consignment.',
        loadDetailFailed: 'An error occurred while loading the process details.',
      },
      action: {
        start: 'Start',
        view: 'View',
        continue: 'Continue',
      },
    },

    tasks: {
      loading: 'Loading task...',
      goBack: 'Go Back',
      refresh: 'Refresh',
      taskId: 'Task ID',
      continueTo: 'Continue to {{name}}',
      continueToNext: 'Continue to next task',
      backToConsignment: 'Back to consignment',
      backToCase: 'Back to case',
      draftSaved: 'Draft saved',
      savingDraft: 'Saving…',
      unsaved: {
        title: 'Leave without saving?',
        description: 'Your answers on this form have not been saved. If you leave now, they will be lost.',
        stay: 'Stay on this page',
        leave: 'Leave without saving',
      },
      validation: {
        requiredFields: 'Please fill in all required fields.',
      },
      error: {
        missingId: 'Task ID is missing.',
        fetchFailed: 'Failed to fetch task details.',
        notFound: 'Task not found.',
        submitFailed: 'Failed to submit task. Please try again.',
        draftFailed: 'Failed to save the draft. Please try again.',
        staleStep: 'This task has moved on since you opened it. It now shows its latest state.',
      },
    },

    workflow: {
      tabs: {
        active: 'Active',
        done: 'Done',
      },
      actionRequired: 'Action Required',
      waitingOnOthers: 'Waiting on Others',
      untitledTask: 'Untitled task',
      continue: 'Continue',
      continueTask: 'Continue {{name}}',
      view: 'View',
      viewTask: 'View {{name}}',
      completedAt: 'Completed {{date}}',
      since: 'Since {{date}}',
      nothingNeeded: 'Nothing needs your action right now.',
      newStepsAppear: 'New steps appear here as your applications are reviewed.',
      newStepsAppearCase: 'New steps appear here as the case progresses.',
      consignmentComplete: 'This consignment is complete.',
      noneDone: 'No tasks are finished yet.',
      status: {
        completed: 'Completed',
        ready: 'Ready',
        inProgress: 'In Progress',
        locked: 'Locked',
        failed: 'Failed',
        awaitingFeedback: 'Awaiting Feedback',
      },
    },

    audit: {
      title: 'Activity',
      entries: '· {{count}} entries',
      time: {
        justNow: 'just now',
        minutesAgo: '{{mins}}m ago',
        hoursAgo: '{{hours}}h ago',
        daysAgo: '{{days}}d ago',
      },
    },

    // Footer.tsx. Labels are resolved by footerLinks[].key, not carried in branding.json.
    footer: {
      links: {
        policy: 'Policy',
        accessibility: 'Accessibility',
        support: 'Support',
      },
    },

    common: {
      breadcrumb: 'Breadcrumb',
      copied: 'Copied',
      copyNamed: 'Copy {{name}}',
      dateTimeAt: '{{date}} at {{time}}',
      poweredBy: 'Powered by <0>OpenNSW</0> from the <1>Lanka Software Foundation</1>',
      pagination: {
        total: 'Total: {{count}}',
        page: 'Page {{page}} of {{totalPages}}',
        previous: 'Previous',
        next: 'Next',
      },
      error: {
        title: 'Something went wrong',
        unexpected: 'An unexpected error occurred',
        tryAgain: 'Try Again',
      },
    },
  },
} as const

export default en
