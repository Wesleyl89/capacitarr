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
      icon: 'i-lucide-heart',
      to: 'https://uanimals.org/en/',
      target: '_blank',
      'aria-label': 'Donate to UAnimals',
    }, {
      icon: 'i-lucide-paw-print',
      to: 'https://www.aspca.org/ways-to-help',
      target: '_blank',
      'aria-label': 'Donate to the ASPCA',
    }, {
      icon: 'i-simple-icons-githubsponsors',
      to: 'https://github.com/sponsors/ghent',
      target: '_blank',
      'aria-label': 'Sponsor on GitHub',
    }, {
      icon: 'i-simple-icons-kofi',
      to: 'https://ko-fi.com/ghent',
      target: '_blank',
      'aria-label': 'Support on Ko-fi',
    }, {
      icon: 'i-simple-icons-buymeacoffee',
      to: 'https://buymeacoffee.com/ghentgames',
      target: '_blank',
      'aria-label': 'Buy Me a Coffee',
    }, {
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
        icon: 'i-simple-icons-gitlab',
        label: 'View on GitLab',
        to: 'https://gitlab.com/starshadow/software/capacitarr',
        target: '_blank',
      }],
    },
  },
})
