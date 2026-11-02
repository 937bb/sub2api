export default {
  batchImageGuide: {
    title: 'Batch Image Generation',
    description: 'Submit multiple prompts in one job and download the generated images when complete'
  },
  // Home Page
  home: {
    brandPage: {
      navStandards: 'Our standards',
      navModels: 'Models',
      navConnect: 'Get connected',
      seeExample: 'Explore the API',
      routeTitle: 'A clear path for every request',
      illustration: 'Illustration',
      selectedRoute: 'Selected model',
      pauseMotion: 'Pause animations',
      resumeMotion: 'Play animations',
      switchRoute: 'Select a model to explore its path · Visual demo only',
      yourApplication: 'Your applications and coding tools',
      oneEntry: 'One entry point · Models by group',
      receiptTitle: 'A record of every call',
      receiptModel: 'Model',
      receiptModelValue: 'Review the model name',
      receiptUsage: 'Usage',
      receiptUsageValue: 'Review tokens and cache usage',
      receiptCharge: 'Charges',
      receiptChargeValue: 'Check each actual charge',
      standardsTitle: 'AI without dilution. Our baseline.',
      standardsDescription: 'Understand every call, from choosing a model to reviewing your bill.',
      modelTitle: 'Know what you choose',
      modelDescription: 'Review the models and prices in your group, then choose the service that fits your task.',
      usageNote: 'Usage details in your console',
      quotaNote: 'A dedicated key for each project',
      modelsTitle: 'Your workflow.\nMore than one model.',
      modelsDescription: 'Manage model access for coding tools, custom applications, and everyday work in one place.',
      modelsNote: 'Check your console for specific model versions, protocols, and capabilities.',
      providers: { gpt: 'Connect coding tools and custom apps', claude: 'Connect your Messages workflows', gemini: 'Add model choices to your applications' },
      connectTitle: 'One key.\nYour first API call.',
      connectDescription: 'Choose a compatible protocol, then configure your endpoint, key, and model name.',
      steps: {
        keyTitle: 'Create an API key',
        keyDescription: 'Sign in, select your group, and create a key for your project.',
        configureTitle: 'Configure your client',
        configureDescription: 'Set the Base URL and API key, then select an available model.',
        usageTitle: 'Make a call and review usage',
        usageDescription: 'Start with a test request, then check its usage and charges in your records.'
      },
      faqTitle: 'Before your first request',
      faqDescription: 'Check your model, protocol, and pricing before you connect.',
      faq: {
        modelsQuestion: 'Can I call every model shown here?',
        modelsAnswer: 'Model access depends on the group assigned to your key. Check model names, protocols, and prices in the console. Replace YOUR_MODEL in the example with an available model name.',
        priceQuestion: 'How is usage billed?',
        priceAnswer: 'Your group prices and billing rules are shown in the console. After each call, review tokens, cache usage, and actual charges in your usage records. Billing categories may vary by model.',
        clientQuestion: 'Do I need to rebuild my client?',
        clientAnswer: 'If your client supports custom endpoints and the matching API protocol, you can usually update its Base URL, API key, and model name. Tools, images, and other capabilities also depend on the model and channel.',
        startQuestion: 'What should I do first?',
        startAnswer: 'Sign in and check your group access and available balance, then create a key. Send a simple request using the example and confirm the response and charge before connecting your full workflow.'
      },
      closingTitle: 'Your choice of model. Our commitment to quality.',
      closingDescription: 'Get to know APIHelm with your next request.'
    },
    gatewayTitle: 'One API key.\nMultiple AI models.',
    requestExample: 'Connect your application',
    protocol: 'Select an API protocol',
    exampleNote: 'Replace the example key and model name with your configuration from the console.',
    accessNote: 'Manage keys, usage, and billing in one console.',
    usageTitle: 'Understand every request',
    usageDescription: 'Review requests by model and date, with token usage and actual charges.',
    quotaTitle: 'Stay in control',
    quotaDescription: 'Set key quotas and manage access separately for each project.',
    modelAvailability: 'Available models and prices depend on your account group.',
    viewOnGithub: 'View on GitHub',
    viewDocs: 'View Documentation',
    docs: 'Docs',
    switchToLight: 'Switch to Light Mode',
    switchToDark: 'Switch to Dark Mode',
    dashboard: 'Dashboard',
    login: 'Login',
    getStarted: 'Get Started',
    goToDashboard: 'Go to Dashboard',
    // User-focused value proposition
    heroSubtitle: 'You can call us expensive.\nYou can’t call us watered-down.',
    heroDescription: 'APIHelm stands for one thing: AI without dilution.',
    tags: {
      subscriptionToApi: 'Subscription to API',
      stickySession: 'Session Persistence',
      realtimeBilling: 'Pay As You Go'
    },
    // Pain points section
    painPoints: {
      title: 'Sound Familiar?',
      items: {
        expensive: {
          title: 'High Subscription Costs',
          desc: 'Paying for multiple AI subscriptions that add up every month'
        },
        complex: {
          title: 'Account Chaos',
          desc: 'Managing scattered accounts and API keys across different platforms'
        },
        unstable: {
          title: 'Service Interruptions',
          desc: 'Single accounts hitting rate limits and disrupting your workflow'
        },
        noControl: {
          title: 'No Usage Control',
          desc: "Can't track where your money goes or limit team member usage"
        }
      }
    },
    // Solutions section
    solutions: {
      title: 'We Solve These Problems',
      subtitle: 'Three simple steps to stress-free AI access'
    },
    features: {
      unifiedGateway: 'One-Click Access',
      unifiedGatewayDesc: 'Get a single API key to call all connected AI models. No separate applications needed.',
      multiAccount: 'Always Reliable',
      multiAccountDesc: 'Smart routing across multiple upstream accounts with automatic failover. Say goodbye to errors.',
      balanceQuota: 'Pay What You Use',
      balanceQuotaDesc: 'Usage-based billing with quota limits. Full visibility into team consumption.'
    },
    // Comparison section
    comparison: {
      title: 'Why Choose Us?',
      headers: {
        feature: 'Comparison',
        official: 'Official Subscriptions',
        us: 'Our Platform'
      },
      items: {
        pricing: {
          feature: 'Pricing',
          official: 'Fixed monthly fee, pay even if unused',
          us: 'Pay only for what you use'
        },
        models: {
          feature: 'Model Selection',
          official: 'Single provider only',
          us: 'Switch between models freely'
        },
        management: {
          feature: 'Account Management',
          official: 'Manage each service separately',
          us: 'Unified key, one dashboard'
        },
        stability: {
          feature: 'Stability',
          official: 'Single account rate limits',
          us: 'Multi-account pool, auto-failover'
        },
        control: {
          feature: 'Usage Control',
          official: 'Not available',
          us: 'Quotas & detailed analytics'
        }
      }
    },
    providers: {
      title: 'Supported AI Models',
      description: 'One API, Multiple Choices',
      supported: 'Supported',
      soon: 'Soon',
      claude: 'Claude',
      gemini: 'Gemini',
      antigravity: 'Antigravity',
      more: 'More'
    },
    // CTA section
    cta: {
      title: 'Ready to Get Started?',
      description: 'Sign up now and get free trial credits to experience seamless AI access',
      button: 'Sign Up Free'
    },
    footer: {
      allRightsReserved: 'All rights reserved.'
    }
  },

  // Key Usage Query Page
  keyUsage: {
    title: 'API Key Usage',
    subtitle: 'Enter your API Key to view real-time spending and usage status',
    placeholder: 'sk-ant-mirror-xxxxxxxxxxxx',
    query: 'Query',
    querying: 'Querying...',
    privacyNote: 'Your Key is processed locally in the browser and will not be stored',
    dateRange: 'Date Range:',
    dateRangeToday: 'Today',
    dateRange7d: '7 Days',
    dateRange30d: '30 Days',
    dateRange90d: '90 Days',
    dateRangeCustom: 'Custom',
    apply: 'Apply',
    used: 'Used',
    detailInfo: 'Detail Information',
    tokenStats: 'Token Statistics',
    dailyDetail: 'Daily Detail',
    modelStats: 'Model Usage Statistics',
    // Table headers
    date: 'Date',
    model: 'Model',
    requests: 'Requests',
    inputTokens: 'Input Tokens',
    outputTokens: 'Output Tokens',
    cacheCreationTokens: 'Cache Creation',
    cacheReadTokens: 'Cache Read',
    cacheWriteTokens: 'Cache Write',
    totalTokens: 'Total Tokens',
    cost: 'Cost',
    // Status
    quotaMode: 'Key Quota Mode',
    walletBalance: 'Wallet Balance',
    // Ring card titles
    totalQuota: 'Total Quota',
    limit5h: '5-Hour Limit',
    limitDaily: 'Daily Limit',
    limit7d: '7-Day Limit',
    limitWeekly: 'Weekly Limit',
    limitMonthly: 'Monthly Limit',
    // Detail rows
    remainingQuota: 'Remaining Quota',
    expiresAt: 'Expires At',
    todayExpires: '(expires today)',
    daysLeft: '({days} days)',
    usedQuota: 'Used Quota',
    resetNow: 'Resetting soon',
    subscriptionType: 'Subscription Type',
    billingType: 'Billing Type',
    subscriptionExpires: 'Subscription Expires',
    // Usage stat cells
    todayRequests: 'Today Requests',
    todayInputTokens: 'Today Input',
    todayOutputTokens: 'Today Output',
    todayTokens: 'Today Tokens',
    todayCacheCreation: 'Today Cache Creation',
    todayCacheRead: 'Today Cache Read',
    todayCost: 'Today Cost',
    rpmTpm: 'RPM / TPM',
    totalRequests: 'Total Requests',
    totalInputTokens: 'Total Input',
    totalOutputTokens: 'Total Output',
    totalTokensLabel: 'Total Tokens',
    totalCacheCreation: 'Total Cache Creation',
    totalCacheRead: 'Total Cache Read',
    totalCost: 'Total Cost',
    avgDuration: 'Avg Duration',
    // Messages
    enterApiKey: 'Please enter an API Key',
    querySuccess: 'Query successful',
    queryFailed: 'Query failed',
    queryFailedRetry: 'Query failed, please try again later',
    noDailyUsage: 'No daily usage data',
  },

  // Setup Wizard
  setup: {
    title: 'Sub2API Setup',
    description: 'Configure your Sub2API instance',
    database: {
      title: 'Database Configuration',
      description: 'Connect to your PostgreSQL database',
      host: 'Host',
      port: 'Port',
      username: 'Username',
      password: 'Password',
      databaseName: 'Database Name',
      sslMode: 'SSL Mode',
      passwordPlaceholder: 'Password',
      ssl: {
        disable: 'Disable',
        require: 'Require',
        verifyCa: 'Verify CA',
        verifyFull: 'Verify Full'
      }
    },
    redis: {
      title: 'Redis Configuration',
      description: 'Connect to your Redis server',
      host: 'Host',
      port: 'Port',
      username: 'Username (optional)',
      password: 'Password (optional)',
      database: 'Database',
      usernamePlaceholder: 'Leave empty for default user',
      passwordPlaceholder: 'Password',
      enableTls: 'Enable TLS',
      enableTlsHint: 'Use TLS when connecting to Redis (public CA certs)'
    },
    admin: {
      title: 'Admin Account',
      description: 'Create your administrator account',
      email: 'Email',
      password: 'Password',
      confirmPassword: 'Confirm Password',
      passwordPlaceholder: 'Min 8 characters',
      confirmPasswordPlaceholder: 'Confirm password',
      passwordMismatch: 'Passwords do not match'
    },
    ready: {
      title: 'Ready to Install',
      description: 'Review your configuration and complete setup',
      database: 'Database',
      redis: 'Redis',
      adminEmail: 'Admin Email'
    },
    status: {
      testing: 'Testing...',
      success: 'Connection Successful',
      testConnection: 'Test Connection',
      installing: 'Installing...',
      completeInstallation: 'Complete Installation',
      completed: 'Installation completed!',
      redirecting: 'Redirecting to login page...',
      restarting: 'Service is restarting, please wait...',
      timeout: 'Service restart is taking longer than expected. Please refresh the page manually.'
    }
  },

  // Common
}
