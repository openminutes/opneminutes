import { defineConfig } from 'vitepress'
import llmstxt, { copyOrDownloadAsMarkdownButtons } from 'vitepress-plugin-llms'

// https://vitepress.dev/reference/site-config
export default defineConfig({
  vite: {
    plugins: [llmstxt()],
  },
  markdown: {
    config(md) {
      md.use(copyOrDownloadAsMarkdownButtons)
    },
  },
  title: 'OpenMinutes',
  description: 'A Go CLI for Feishu/Lark Minutes',
  themeConfig: {
    nav: [
      { text: 'Home', link: '/' },
      { text: 'Quick Start', link: '/quick-start' },
      { text: 'CLI', link: '/cli/openminutes' }
    ],

    sidebar: [
      {
        text: 'Getting Started',
        items: [
          { text: 'Quick Start', link: '/quick-start' }
        ]
      },
      {
        text: 'CLI Guides',
        items: [
          { text: 'openminutes', link: '/cli/openminutes' },
          { text: 'openminutes list', link: '/cli/list' },
          { text: 'openminutes get', link: '/cli/get' },
          { text: 'openminutes upload', link: '/cli/upload' },
          { text: 'openminutes delete', link: '/cli/delete' },
          { text: 'openminutes completion', link: '/cli/completion' },
          { text: 'openminutes completion bash', link: '/cli/completion-bash' },
          { text: 'openminutes completion zsh', link: '/cli/completion-zsh' },
          { text: 'openminutes completion fish', link: '/cli/completion-fish' },
          { text: 'openminutes completion powershell', link: '/cli/completion-powershell' }
        ]
      }
    ],

    socialLinks: [
      { icon: 'github', link: 'https://github.com/openminutes/openminutes' }
    ]
  }
})
