# Capacitarr - Premium Capacity Monitoring Dashboard

Capacitarr is an enterprise-grade capacity aggregation and visualization platform. Engineered for speed and distributed environments, it pairs a lightning-fast Go (Golang) backend with a beautifully modern Vue 3 (Nuxt 3) frontend, seamlessly packaged into a single binary deployment.

## Key Features

- **Blazing Fast**: Backend purely written in Go utilizing Echo framework.
- **Single Binary Deployable**: The entire Vue 3 application is statically generated and embedded utilizing `go:embed` inside the Golang binary.
- **Zero Config Storage**: Embedded high-performance `SQLite` database utilizing GORM, negating the need for complex database infrastructure tracking.
- **Unified Authentication**: Clean JWT login for Web UI management paired with rapid API Key provisioning for programmatic telemetry ingestion.
- **Intelligent Base-Path Routing**: Deploy Capacitarr at the root domain (`http://app.com`) or seamlessly behind a load balancer subdirectory proxy (`http://app.com/capacitarr/`).
- **Premium Visualization**: Implements Nuxt UI (Tailwind CSS) paired with sophisticated Apexcharts capacity trend analysis.
- **Time-Series Rollups**: An automated background chron scheduler strictly maintains your metric timeframes, intelligently rolling up real-time pings into hourly, daily, and weekly historical plots before forcefully pruning old edge data. 

## Technology Architecture

### Backend (Go / SQLite)
The engine of Capacitarr runs atop Go 1.23 using Echo. Core components include:
- `db`: Contains the Gorm SQLite data models and bootstrapping process.
- `api`: Protected REST routes utilizing strict JWT or API-Key based context injection.
- `scheduler`: Background CRON jobs processing time-series aggregations to prevent unmanaged SQLite database bloat.
- `config`: Handles environment-variable injected bootstrapping logic targeting standard properties like ports and DB routing. 
- `engine`: The rule and scoring engine that evaluates media based on User Preferences and Protection Rules. (See `docs/plans/scoring_design.md`)

### Frontend (Vue 3 / Nuxt / Tailwind / ApexCharts)
Designed to drop jaws, the Dashboard heavily utilizes high-end pre-compiled Slate and Indigo palettes spanning responsive desktop/mobile environments and providing completely reactive dark/light mode toggling out of the box. Nuxt routes respect `NUXT_APP_BASE_URL` aligning symmetrically with Go's pathing prefix parameters logic allowing true dynamic hosting locations.

---

## Getting Started

### Prerequisites

- Go `1.23+`
- Node.js `20.x+` (for local frontend development)
- Docker (for containerized deployments)

### Running Locally (Development Mode)

If you are developing against Capacitarr, it is best to run the two servers separately:

**1. Start the Backend API**
```bash
cd backend
go run main.go
```
*Note: The backend defaults to `http://localhost:2187` if not overridden.*

**2. Start the Frontend Dev environment**
```bash
cd frontend
pnpm install
pnpm run dev
```

### Production Deployment (Single Static Binary)

Capacitarr's super-power is condensing the node-based frontend application into the backend compilation tree utilizing `go:embed`. 

**1. Build the Nuxt Frontend**
```bash
cd frontend
pnpm run build 
```

**2. Copy the Frontend assets to the Backend tree**
```bash
# Our backend main.go natively expects to embed from a 'frontend/dist' path relative to itself.
mkdir -p backend/frontend/dist
cp -R frontend/.output/public/* backend/frontend/dist/
```

**3. Build the Backend**
```bash
cd backend
go build -o capacitarr main.go

# Start the application
./capacitarr
```

## Docker Depolyment

We've provided a highly optimized multi-stage `Dockerfile` that executes the Nuxt generation task automatically right before constructing the Go executable, generating a final scratch container that only houses `alpine` dependencies and our resulting binary execution runtime. 

```bash
docker build -t capacitarr .
docker run -p 2187:2187 capacitarr 
```

### Advanced Reverse Proxying (Subdirectory Deployment)

If deploying behind Nginx or similar to intercept traffic towards a specific application route (e.g. `/system/metrics`), you must notify both Nuxt (via ENV) and Go (via ENV) to offset their routing architecture globally. 

**Docker Execution Example:**
```bash
docker run -e NUXT_APP_BASE_URL=/system/metrics/ -e BASE_URL=/system/metrics -p 2187:2187 capacitarr 
```

**Nginx Configuration Mapping Example:**
```nginx
location /system/metrics/ {
    proxy_pass http://localhost:2187/;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_addrs;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

## Licensing

Capacitarr source code is currently licensed strictly under the [PolyForm Noncommercial 1.0.0](LICENSE). 
Review our [Contributing Guidelines](CONTRIBUTING.md) for information regarding accepted PR signatures.
