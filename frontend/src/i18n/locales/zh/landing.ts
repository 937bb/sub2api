export default {
  batchImageGuide: {
    title: '图片批量生成',
    description: '一次提交多条提示词，任务完成后可统一下载图片结果'
  },
  // Home Page
  home: {
    brandPage: {
      navStandards: '我们的坚持',
      navModels: '模型接入',
      navConnect: '开始接入',
      seeExample: '查看接入示例',
      routeTitle: '一次调用，清楚的路径',
      illustration: '路径示意',
      selectedRoute: '当前展示',
      pauseMotion: '暂停动画',
      resumeMotion: '播放动画',
      switchRoute: '点击模型，探索接入路径 · 仅为示意，不发送请求',
      yourApplication: '你的应用与编程工具',
      oneEntry: '统一接入 · 按分组选择模型',
      receiptTitle: '调用之后，记录可查',
      receiptModel: '模型',
      receiptModelValue: '查看本次调用的模型名称',
      receiptUsage: '用量',
      receiptUsageValue: '查看 Token 消耗与缓存用量',
      receiptCharge: '费用',
      receiptChargeValue: '核对每笔实际扣费',
      standardsTitle: '不掺，是做中转的底线。',
      standardsDescription: '从选择模型到核对账单，把每一次调用用明白。',
      modelTitle: '选什么，心里有数',
      modelDescription: '接入前查看分组中的模型与价格，根据自己的任务选择服务。',
      usageNote: '用量明细，在控制台查看',
      quotaNote: '一个项目，一把专属密钥',
      modelsTitle: '你的工作流，\n不必只有一种模型。',
      modelsDescription: '编程助手、自建应用与日常工作，在一个入口管理不同模型的接入。',
      modelsNote: '具体模型版本、协议与能力，请以控制台配置为准。',
      providers: { gpt: '连接编程助手与自建应用', claude: '接入现有 Messages 工作流', gemini: '为应用增加模型选择' },
      connectTitle: '从一把密钥，\n开始第一次调用。',
      connectDescription: '选择客户端支持的协议，填入接入地址、密钥和模型名称。',
      steps: {
        keyTitle: '创建 API 密钥',
        keyDescription: '登录控制台，选择所需分组，创建用于项目的密钥。',
        configureTitle: '配置你的客户端',
        configureDescription: '填入 Base URL 与 API Key，选择分组内可用的模型。',
        usageTitle: '发起调用，核对用量',
        usageDescription: '先完成一次测试，再到使用记录中查看用量与费用。'
      },
      faqTitle: '接入前，你可能想了解',
      faqDescription: '把模型、协议和计费确认清楚，再开始使用。',
      faq: {
        modelsQuestion: '页面里的模型都可以直接调用吗？',
        modelsAnswer: '模型是否可用取决于密钥所属的分组。请在控制台确认模型名称、可用协议与价格，示例中的 YOUR_MODEL 需要替换为实际名称。',
        priceQuestion: '具体怎么计费，在哪里查看？',
        priceAnswer: '请以控制台展示的分组价格和计费规则为准。调用后可以在使用记录中查看 Token 用量、缓存用量和实际费用；不同模型的计费项目可能不同。',
        clientQuestion: '已有的客户端需要重新开发吗？',
        clientAnswer: '如果客户端支持自定义接入地址，并兼容对应的 API 协议，通常只需修改 Base URL、API Key 和模型名称。工具调用、图片等能力还需匹配具体模型与渠道。',
        startQuestion: '第一次接入，应该先做什么？',
        startAnswer: '先登录控制台并确认分组权限与可用余额，再创建密钥。按照接入示例发送一次简单请求，确认返回和扣费后再接入完整工作流。'
      },
      closingTitle: '把选择交给你，把标准留给我们。',
      closingDescription: '从下一次调用开始，认识 APIHelm。'
    },
    gatewayTitle: '一个密钥，\n连接多种 AI 模型。',
    requestExample: '接入示例',
    protocol: '选择 API 协议',
    exampleNote: '示例中的密钥和模型名称，请替换为控制台中的实际配置。',
    accessNote: '密钥、用量和账单，在一个控制台管理。',
    usageTitle: '每笔用量，清楚可查',
    usageDescription: '按模型和时间查看调用记录，核对 Token 用量与实际费用。',
    quotaTitle: '用量由你掌控',
    quotaDescription: '为 API 密钥设置额度，按项目管理访问权限与使用范围。',
    modelAvailability: '可用模型与价格以账号所属分组为准。',
    viewOnGithub: '在 GitHub 上查看',
    viewDocs: '查看文档',
    docs: '文档',
    switchToLight: '切换到浅色模式',
    switchToDark: '切换到深色模式',
    dashboard: '控制台',
    login: '登录',
    getStarted: '立即开始',
    goToDashboard: '进入控制台',
    // 新增：面向用户的价值主张
    heroSubtitle: '你可以说我贵\n但不能说我掺',
    heroDescription: 'APIHelm 做中转，只坚持一件事：不掺。',
    tags: {
      subscriptionToApi: '订阅转 API',
      stickySession: '会话保持',
      realtimeBilling: '按量计费'
    },
    // 用户痛点区块
    painPoints: {
      title: '你是否也遇到这些问题？',
      items: {
        expensive: {
          title: '订阅费用高',
          desc: '每个 AI 服务都要单独订阅，每月支出越来越多'
        },
        complex: {
          title: '多账号难管理',
          desc: '不同平台的账号、密钥分散各处，管理起来很麻烦'
        },
        unstable: {
          title: '服务不稳定',
          desc: '单一账号容易触发限制，影响正常使用'
        },
        noControl: {
          title: '用量无法控制',
          desc: '不知道钱花在哪了，也无法限制团队成员的使用'
        }
      }
    },
    // 解决方案区块
    solutions: {
      title: '我们帮你解决',
      subtitle: '简单三步，开始省心使用 AI'
    },
    features: {
      unifiedGateway: '一键接入',
      unifiedGatewayDesc: '获取一个 API 密钥，即可调用所有已接入的 AI 模型，无需分别申请。',
      multiAccount: '稳定可靠',
      multiAccountDesc: '智能调度多个上游账号，自动切换和负载均衡，告别频繁报错。',
      balanceQuota: '用多少付多少',
      balanceQuotaDesc: '按实际使用量计费，支持设置配额上限，团队用量一目了然。'
    },
    // 优势对比
    comparison: {
      title: '为什么选择我们？',
      headers: {
        feature: '对比项',
        official: '官方订阅',
        us: '本平台'
      },
      items: {
        pricing: {
          feature: '付费方式',
          official: '固定月费，用不完也付',
          us: '按量付费，用多少付多少'
        },
        models: {
          feature: '模型选择',
          official: '单一服务商',
          us: '多模型随意切换'
        },
        management: {
          feature: '账号管理',
          official: '每个服务单独管理',
          us: '统一密钥，一站管理'
        },
        stability: {
          feature: '服务稳定性',
          official: '单账号易触发限制',
          us: '多账号池，自动切换'
        },
        control: {
          feature: '用量控制',
          official: '无法限制',
          us: '可设配额、查明细'
        }
      }
    },
    providers: {
      title: '已支持的 AI 模型',
      description: '一个 API，多种选择',
      supported: '已支持',
      soon: '即将推出',
      claude: 'Claude',
      gemini: 'Gemini',
      antigravity: 'Antigravity',
      more: '更多'
    },
    // CTA 区块
    cta: {
      title: '准备好开始了吗？',
      description: '注册即可获得免费试用额度，体验一站式 AI 服务',
      button: '免费注册'
    },
    footer: {
      allRightsReserved: '保留所有权利。'
    }
  },

  // Key Usage Query Page
  keyUsage: {
    title: 'API Key 用量查询',
    subtitle: '输入您的 API Key 以查看实时消费金额与使用状态',
    placeholder: 'sk-ant-mirror-xxxxxxxxxxxx',
    query: '查询',
    querying: '查询中...',
    privacyNote: '您的 Key 仅在浏览器本地处理，不会被存储',
    dateRange: '统计范围:',
    dateRangeToday: '今日',
    dateRange7d: '7 天',
    dateRange30d: '30 天',
    dateRange90d: '90 天',
    dateRangeCustom: '自定义',
    apply: '应用',
    used: '已使用',
    detailInfo: '详细信息',
    tokenStats: 'Token 统计',
    dailyDetail: '按日明细',
    modelStats: '模型用量统计',
    // Table headers
    date: '日期',
    model: '模型',
    requests: '请求数',
    inputTokens: '输入 Tokens',
    outputTokens: '输出 Tokens',
    cacheCreationTokens: '缓存创建',
    cacheReadTokens: '缓存读取',
    cacheWriteTokens: '缓存写入',
    totalTokens: '总 Tokens',
    cost: '费用',
    // Status
    quotaMode: 'Key 限额模式',
    walletBalance: '钱包余额',
    // Ring card titles
    totalQuota: '总额度',
    limit5h: '5 小时限额',
    limitDaily: '日限额',
    limit7d: '7 天限额',
    limitWeekly: '周限额',
    limitMonthly: '月限额',
    // Detail rows
    remainingQuota: '剩余额度',
    expiresAt: '过期时间',
    todayExpires: '(今日到期)',
    daysLeft: '({days} 天)',
    usedQuota: '已用额度',
    resetNow: '即将重置',
    subscriptionType: '订阅类型',
    billingType: '计费方式',
    subscriptionExpires: '订阅到期',
    // Usage stat cells
    todayRequests: '今日请求',
    todayInputTokens: '今日输入',
    todayOutputTokens: '今日输出',
    todayTokens: '今日 Tokens',
    todayCacheCreation: '今日缓存创建',
    todayCacheRead: '今日缓存读取',
    todayCost: '今日费用',
    rpmTpm: 'RPM / TPM',
    totalRequests: '累计请求',
    totalInputTokens: '累计输入',
    totalOutputTokens: '累计输出',
    totalTokensLabel: '累计 Tokens',
    totalCacheCreation: '累计缓存创建',
    totalCacheRead: '累计缓存读取',
    totalCost: '累计费用',
    avgDuration: '平均耗时',
    // Messages
    enterApiKey: '请输入 API Key',
    querySuccess: '查询成功',
    queryFailed: '查询失败',
    queryFailedRetry: '查询失败，请稍后重试',
    noDailyUsage: '暂无按日用量数据',
  },

  // Setup Wizard
  setup: {
    title: 'Sub2API 安装向导',
    description: '配置您的 Sub2API 实例',
    database: {
      title: '数据库配置',
      description: '连接到您的 PostgreSQL 数据库',
      host: '主机',
      port: '端口',
      username: '用户名',
      password: '密码',
      databaseName: '数据库名称',
      sslMode: 'SSL 模式',
      passwordPlaceholder: '密码',
      ssl: {
        disable: '禁用',
        require: '要求',
        verifyCa: '验证 CA',
        verifyFull: '完全验证'
      }
    },
    redis: {
      title: 'Redis 配置',
      description: '连接到您的 Redis 服务器',
      host: '主机',
      port: '端口',
      username: '用户名（可选）',
      password: '密码（可选）',
      database: '数据库',
      usernamePlaceholder: '默认用户留空',
      passwordPlaceholder: '密码',
      enableTls: '启用 TLS',
      enableTlsHint: '连接 Redis 时使用 TLS（公共 CA 证书）'
    },
    admin: {
      title: '管理员账户',
      description: '创建您的管理员账户',
      email: '邮箱',
      password: '密码',
      confirmPassword: '确认密码',
      passwordPlaceholder: '至少 8 个字符',
      confirmPasswordPlaceholder: '确认密码',
      passwordMismatch: '密码不匹配'
    },
    ready: {
      title: '准备安装',
      description: '检查您的配置并完成安装',
      database: '数据库',
      redis: 'Redis',
      adminEmail: '管理员邮箱'
    },
    status: {
      testing: '测试中...',
      success: '连接成功',
      testConnection: '测试连接',
      installing: '安装中...',
      completeInstallation: '完成安装',
      completed: '安装完成！',
      redirecting: '正在跳转到登录页面...',
      restarting: '服务正在重启，请稍候...',
      timeout: '服务重启时间超出预期，请手动刷新页面。'
    }
  },

  // Common
}
