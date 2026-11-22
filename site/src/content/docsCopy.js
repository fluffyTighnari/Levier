export const DOCS_COPY = {
  en: {
    htmlLang: "en",
    brandSub: "Documentation",
    skipDocs: "Skip to documentation",
    home: "Home",
    menuAria: "Open documentation navigation",
    closeNavAria: "Close documentation navigation",
    githubAria: "Levier on GitHub (opens in a new tab)",
    switchLabel: "中文",
    switchAria: "Switch to Chinese",
    navAria: "Documentation sections",
    sectionNav: [
      { slug: "onboarding", label: "Getting started" },
      { slug: "pre-model", label: "Configuration" },
      { slug: "app-core", label: "App" },
      { slug: "levier-install", label: "Levier" },
      { slug: "runtime-core", label: "Runtime" },
      { slug: "cli", label: "Reference" }
    ],
    sidebar: {
      title: "Documentation",
      pages: (count) => `${count} pages`,
      filtered: (shown, total) => `${shown} of ${total}`,
      loading: "Loading…",
      unavailable: "Unavailable",
      filter: "Filter documentation…",
      filterAria: "Filter documentation",
      clearFilter: "Clear filter",
      errorTitle: "Navigation unavailable",
      retry: "Retry",
      emptyTitle: "No documentation yet",
      emptyBody: "Add pages to manifest.json to populate the sidebar.",
      noResultsTitle: "No matching pages",
      noResultsBody: "Try a command, term or page title.",
      footer: "Commands and fields follow the repository. If something fails, start with ",
      troubleshooting: "Troubleshooting",
      footerTail: ".",
      languageNote: "English documentation"
    },
    docHeader: {
      documentation: "Documentation",
      breadcrumbAria: "Breadcrumb",
      updated: "Updated",
      published: "Published",
      draft: "Draft",
      language: "English"
    },
    toc: {
      title: "On this page",
      count: (count) => `${count} sections`,
      aria: "On this page"
    },
    pager: {
      previous: "Previous",
      next: "Next",
      aria: "Previous and next pages"
    },
    skeleton: {
      aria: "Loading document"
    },
    error: {
      pageTitle: "This page could not load",
      pageIntro: "Could not read",
      retry: "Retry",
      fileHint: "Browsers block file:// pages from reading Markdown. Run python -m http.server 4173 --directory site/dist and open the page over HTTP.",
      pathHint: "Check the file path in manifest.json, then confirm the Markdown file exists in docs/content/en or docs/content.",
      manifestTitle: "Documentation unavailable",
      manifestHint: "Check that manifest.json is present, served over HTTP, and valid JSON."
    },
    empty: {
      title: "No documentation available yet",
      body: "Add pages to manifest.json and the Markdown files under docs/content."
    },
    footer: {
      label: "Levier documentation · MVP",
      home: "Home",
      github: "GitHub",
      scope: "SCOPE"
    },
    markdown: {
      callouts: {
        NOTE: "Note",
        TIP: "Tip",
        IMPORTANT: "Important",
        WARNING: "Warning",
        CAUTION: "Caution",
        OUTPUT: "Expected output",
        FAIL: "If it fails"
      },
      copy: "Copy code",
      copied: "Copied",
      copyFailed: "Copy failed",
      copiedLive: "Copied to clipboard",
      copyFailedLive: "Copy failed",
      codeBlock: "Code block",
      codeSample: "Code sample",
      table: (number) => `Table ${number}`,
      newTab: "(opens in a new tab)"
    },
    routeAnnounce: (title, section) => `${title}. ${section}.`,
    docsTitle: (title) => `${title} — Levier Documentation`,
    metaDescription: "Levier documentation: setup, configuration, App, Levier, Runtime and the CLI / SDK / environment reference."
  },
  zh: {
    htmlLang: "zh-CN",
    brandSub: "文档",
    skipDocs: "跳到文档正文",
    home: "首页",
    menuAria: "打开文档导航",
    closeNavAria: "关闭文档导航",
    githubAria: "Levier 的 GitHub（在新标签页打开）",
    switchLabel: "EN",
    switchAria: "切换到英文",
    navAria: "文档分区",
    sectionNav: [
      { slug: "onboarding", label: "接入" },
      { slug: "pre-model", label: "配置" },
      { slug: "app-core", label: "App" },
      { slug: "levier-install", label: "Levier" },
      { slug: "runtime-core", label: "Runtime" },
      { slug: "cli", label: "总览" }
    ],
    sidebar: {
      title: "文档",
      pages: (count) => `${count} 页`,
      filtered: (shown, total) => `${shown} / ${total}`,
      loading: "加载中…",
      unavailable: "不可用",
      filter: "筛选文档…",
      filterAria: "筛选文档",
      clearFilter: "清除筛选",
      errorTitle: "导航不可用",
      retry: "重试",
      emptyTitle: "暂无文档",
      emptyBody: "在 manifest.json 中添加页面以填充侧栏。",
      noResultsTitle: "没有匹配的文档",
      noResultsBody: "换一个命令、术语或页面标题。",
      footer: "命令与字段以仓库为准。遇到问题先看",
      troubleshooting: "错误与排查",
      footerTail: "。",
      languageNote: "中文文档"
    },
    docHeader: {
      documentation: "文档",
      breadcrumbAria: "面包屑",
      updated: "更新于",
      published: "已发布",
      draft: "草稿",
      language: "中文"
    },
    toc: {
      title: "本页内容",
      count: (count) => `${count} 节`,
      aria: "本页内容"
    },
    pager: {
      previous: "上一篇",
      next: "下一篇",
      aria: "上一篇 / 下一篇"
    },
    skeleton: {
      aria: "正在载入文档"
    },
    error: {
      pageTitle: "这篇文档未能载入",
      pageIntro: "无法读取",
      retry: "重试",
      fileHint: "浏览器禁止 file:// 页面读取 Markdown。请运行 python -m http.server 4173 --directory site/dist，并通过 HTTP 打开页面。",
      pathHint: "检查 manifest.json 中的路径，并确认 Markdown 文件位于 docs/content/en 或 docs/content。",
      manifestTitle: "文档暂不可用",
      manifestHint: "检查 manifest.json 是否存在、可通过 HTTP 访问且为有效 JSON。"
    },
    empty: {
      title: "暂无文档",
      body: "在 manifest.json 中添加页面，并将 Markdown 文件放入 docs/content。"
    },
    footer: {
      label: "Levier 文档 · MVP",
      home: "首页",
      github: "GitHub",
      scope: "SCOPE"
    },
    markdown: {
      callouts: {
        NOTE: "说明",
        TIP: "提示",
        IMPORTANT: "重要",
        WARNING: "注意",
        CAUTION: "注意",
        OUTPUT: "应该看到",
        FAIL: "失败时"
      },
      copy: "复制代码",
      copied: "已复制",
      copyFailed: "复制失败",
      copiedLive: "已复制到剪贴板",
      copyFailedLive: "复制失败",
      codeBlock: "代码块",
      codeSample: "代码示例",
      table: (number) => `表格 ${number}`,
      newTab: "（在新标签页打开）"
    },
    routeAnnounce: (title, section) => `${title}。${section}。`,
    docsTitle: (title) => `${title} — Levier 文档`,
    metaDescription: "Levier 文档：接入、配置、App 端、Levier 端、Runtime 端与 CLI / SDK / 环境变量总览。"
  }
};
