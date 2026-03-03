export default defineAppConfig({
  ui: {
    colors: {
      primary: 'violet',
      neutral: 'zinc',
    },
    footer: {
      slots: {
        root: 'border-t border-default',
        left: 'text-sm text-muted',
      },
    },
  },
  seo: {
    siteName: 'Capacitarr',
  },
  header: {
    title: 'Capacitarr',
    to: '/',
    search: true,
    colorMode: true,
    links: [{
      icon: 'i-simple-icons-gitlab',
      to: 'https://gitlab.com/starshadow/software/capacitarr',
      target: '_blank',
      'aria-label': 'GitLab',
    }],
  },
  footer: {
    credits: `© ${new Date().getFullYear()} Capacitarr`,
    colorMode: false,
    links: [{
      icon: 'i-simple-icons-gitlab',
      to: 'https://gitlab.com/starshadow/software/capacitarr',
      target: '_blank',
      'aria-label': 'Capacitarr on GitLab',
    }],
  },
  toc: {
    title: 'On this page',
    bottom: {
      title: 'Resources',
      links: [{
        icon: 'i-lucide-book-open',
        label: 'Nuxt UI Docs',
        to: 'https://ui.nuxt.com/docs/getting-started/installation/nuxt',
        target: '_blank',
      }],
    },
  },
})
