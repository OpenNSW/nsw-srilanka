const en = {
  auth: {
    login: {
      tagline: 'Sign in to continue to your consignments.',
      button: 'Sign In',
    },
    unauthorized: {
      title: 'Access Restricted',
      message: 'Your account is signed in, but it does not currently have an application role.',
      signOut: 'Sign out',
    },
  },

  sidebar: {
    nav: {
      consignments: 'Consignments',
      verifiedDocs: 'Verified Docs',
    },
    version: {
      label: 'NSW',
    },
    toggle: {
      collapse: 'Collapse',
      expand: 'Expand',
      collapseTitle: 'Collapse sidebar',
      expandTitle: 'Expand sidebar',
    },
  },

  roles: {
    primary: '(Primary)',
    trader: {
      label: 'Trader',
      description: 'Managing consignments',
      dropdownDescription: 'Create and manage consignments',
    },
    cha: {
      label: 'CHA',
      description: 'Handling Customs Clearances',
      dropdownDescription: 'Handle customs clearances',
    },
  },

  consignments: {
    list: {
      loading: 'Loading Consignments...',
      title: 'Consignments',
      create: 'New Consignment',
      creating: 'Creating...',
      searchPlaceholder: 'Search by Name, ID, or HS Code...',
      filter: {
        statePlaceholder: 'State',
        allStates: 'All States',
        initialized: 'Initialized',
        inProgress: 'In Progress',
        finished: 'Finished',
        failed: 'Failed',
        tradeFlowPlaceholder: 'Trade Flow',
        allTypes: 'All Types',
        import: 'Import',
        export: 'Export',
      },
      empty: {
        cha: 'No consignments yet.',
        trader: 'No consignments yet. Click "New Consignment" to create your first one.',
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
      },
      back: 'Back',
      backToList: 'Back to Consignments',
      tryAgain: 'Try Again',
      refresh: 'Refresh',
      refreshing: 'Refreshing...',
      title: 'Consignment View',
      field: {
        consignmentId: 'Consignment ID',
        dateCreated: 'Date Created',
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
    back: 'Back to Tasks',
    goBack: 'Go Back',
    error: {
      missingId: 'Task ID is missing.',
      fetchFailed: 'Failed to fetch task details.',
      notFound: 'Task not found.',
      submitFailed: 'Failed to submit task. Please try again.',
    },
  },

  workflow: {
    taskHistory: 'Task History',
    actionRequired: 'Action Required',
    processHistory: 'Process History',
    updatingList: 'Updating your list...',
    refresh: 'Refresh',
    processComplete: {
      title: 'Process Complete',
      description: 'All workflow steps have been finished successfully. No further actions are required.',
    },
    waitingForUpdates: {
      title: 'Waiting for Updates',
      description: 'Current steps are being processed. Next tasks will unlock automatically.',
    },
    status: {
      completed: 'Completed',
      ready: 'Ready',
      inProgress: 'In Progress',
      locked: 'Locked',
      failed: 'Failed',
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

  common: {
    dateTimeAt: '{{date}} at {{time}}',
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
} as const

export default en
