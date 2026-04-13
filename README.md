# Capacitarr

[![Pipeline][pipeline-badge]][pipeline-url]
[![Release][release-badge]][release-url]
[![License][license-badge]][license-url]
[![Docker Hub][dockerhub-badge]][dockerhub-url]
[![Security][security-badge]][security-url]
[![GitHub Stars][stars-badge]][stars-url]
[![GitHub Last Commit][lastcommit-badge]][lastcommit-url]

> *Intelligent media library capacity manager for the \*arr ecosystem.*

Capacitarr scores every media item across seven pluggable dimensions — watch history, recency, file size, ratings, age, series status, and request popularity — then removes the least-valuable items first when disk space runs low. A visual rule builder lets you protect specific content from ever being deleted.

<p align="center">
  <img src="site/public/screenshots/dashboard.webp" alt="Capacitarr Dashboard" width="800" />
</p>

## ✨ Highlights

- **Intelligent Scoring** — Seven pluggable weighted factors rank every item for deletion priority with dynamic UI-driven weights
- **Smart Filters** — Dead content, stale content, requested, and protected presets with configurable staleness thresholds
- **Visual Rule Builder** — Protect content with `always_keep`, `prefer_keep`, `prefer_remove`, and `always_remove` rules with impact previews
- **11 Integrations** — Sonarr, Radarr, Lidarr, Readarr, Plex, Jellyfin, Emby, Tautulli, Jellystat, Tracearr, Seerr (Overseerr/Jellyseerr)
- **Approval Queue** — Per-disk-group approval mode queues deletions for manual review on the dashboard before they execute
- **Real-Time Dashboard** — 70 granular SSE event types push everything to the browser instantly
- **Capacity Forecast** — Linear regression predicts when disk thresholds will be reached
- **Watch Intelligence** — Dead content detection, stale content analysis, multi-user watch aggregation, and watchlist/favorites enrichment
- **TMDb Matching** — Reliable TMDb ID-based matching replaces error-prone title normalization
- **Single Container** — Go + Nuxt + SQLite in one ~30 MB Docker image

## 🚀 Quick Start

```yaml
services:
  capacitarr:
    image: ghcr.io/ghent/capacitarr:stable
    container_name: capacitarr
    ports:
      - "2187:2187"
    environment:
      - PUID=1000
      - PGID=1000
      - JWT_SECRET=change-me-to-a-random-string
    volumes:
      - capacitarr-config:/config
    restart: unless-stopped

volumes:
  capacitarr-config:
```

```bash
docker compose up -d
```

Open **http://localhost:2187** and create your admin account.

## 📖 Documentation

Full docs at **[capacitarr.app](https://capacitarr.app/)** — or browse locally:

[Quick Start](docs/getting-started/quick-start.md) · [Configuration](docs/getting-started/configuration.md) · [Scoring](docs/guides/scoring.md) · [Architecture](docs/reference/architecture.md) · [API Reference](docs/reference/api/README.md) · [Deployment](docs/getting-started/deployment.md)

## 🔐 Security

Developed with AI assistance. Hardened with 7 blocking SAST/SCA tools, DAST scanning, and container hardening — every exception individually documented with rationale. [Full security posture →](SECURITY.md)

## � Community

[Discord](https://discord.gg/fbFkND5qgt) · [Reddit](https://www.reddit.com/r/capacitarr/) · [Contributing](CONTRIBUTING.md)

## 📊 Activity

![Repobeats analytics image](https://repobeats.axiom.co/api/embed/7203908b271be233de70d8c6f15d716c661f5bbe.svg "Repobeats analytics image")

## ⭐ Star History

<a href="https://star-history.com/#Ghent/capacitarr&Date">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=Ghent/capacitarr&type=Date&theme=dark" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/svg?repos=Ghent/capacitarr&type=Date" />
   <img alt="Star History Chart" src="https://api.star-history.com/svg?repos=Ghent/capacitarr&type=Date" />
 </picture>
</a>

## 🇺🇦 Ukraine

I stand with Ukraine. This project is built with the belief that freedom, sovereignty, and self-determination matter — for people and for software.

## 🐾 Support Animal Rescue

Capacitarr is free software. The creator strongly prefers that donations go to animal rescue over developer support:

- **[UAnimals](https://uanimals.org/en/)** — rescuing and protecting animals in Ukraine 🇺🇦
- **[ASPCA](https://www.aspca.org/ways-to-help)** — preventing cruelty to animals

If you still want to support development directly: [GitHub Sponsors](https://github.com/sponsors/ghent) · [Ko-fi](https://ko-fi.com/ghent) · [Buy Me a Coffee](https://buymeacoffee.com/ghentgames)

## License

[PolyForm Noncommercial 1.0.0](LICENSE) — free for any noncommercial use.

<!-- Badge references -->
[pipeline-badge]: https://img.shields.io/github/actions/workflow/status/Ghent/capacitarr/ci.yml?branch=main&logo=github&label=CI
[pipeline-url]: https://github.com/Ghent/capacitarr/actions
[release-badge]: https://img.shields.io/github/v/release/Ghent/capacitarr?logo=github&label=release
[release-url]: https://github.com/Ghent/capacitarr/releases
[license-badge]: https://img.shields.io/badge/license-PolyForm%20NC%201.0-blue
[license-url]: LICENSE
[dockerhub-badge]: https://img.shields.io/docker/v/ghentstarshadow/capacitarr?label=Docker%20Hub&logo=docker&sort=semver
[dockerhub-url]: https://hub.docker.com/r/ghentstarshadow/capacitarr
[security-badge]: https://img.shields.io/badge/security-hardened-brightgreen?logo=owasp&logoColor=white
[security-url]: SECURITY.md
[stars-badge]: https://img.shields.io/github/stars/Ghent/capacitarr?style=flat&logo=github&label=stars
[stars-url]: https://github.com/Ghent/capacitarr/stargazers
[lastcommit-badge]: https://img.shields.io/github/last-commit/Ghent/capacitarr?logo=github
[lastcommit-url]: https://github.com/Ghent/capacitarr/commits/main
