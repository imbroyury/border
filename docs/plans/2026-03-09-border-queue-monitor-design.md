# Border Queue Monitor — Design Document

## Overview

A self-hosted system that crawls Belarusian border crossing queue data from mon.declarant.by every 15 minutes, stores historical snapshots in PostgreSQL, and presents interactive time-series graphs via a web dashboard.

**Scope:** Passenger cars (Легковой авто) only, across all available border crossing zones (BY-PL and BY-LT).

## System Architecture

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│   Crawler   │    │   Backend   │    │  Frontend   │
│  (Go binary) │    │  (Go HTTP)  │    │ (Vue 3 SPA) │
│              │    │             │    │  served by   │
│ Every 15min  │    │  REST API   │    │   Caddy      │
│ scrapes data │    │             │    │             │
└──────┬───────┘    └──────┬──────┘    └──────┬──────┘
       │                   │                  │
       │     ┌─────────────┘                  │
       ▼     ▼                                │
  ┌──────────────┐                            │
  │  PostgreSQL  │         HTTP calls         │
  │              │◄───────────────────────────┘
  └──────────────┘   (via backend API)
```

**4 containers via docker-compose:**

1. `crawler` — Go binary, 15-min ticker, scrapes mon.declarant.by
2. `backend` — Go HTTP server exposing REST API (chi router)
3. `frontend` — Caddy serving Vue 3 + ECharts static build
4. `postgres` — PostgreSQL 16 with persistent volume

## Data Model

### Table: `zones` (reference)

| Column | Type        | Description                        |
|--------|-------------|------------------------------------|
| id     | VARCHAR PK  | e.g. "brest-bts", "kozlovichi-bts" |
| name   | VARCHAR     | Display name, e.g. "Брест"         |
| border | VARCHAR     | "BY-PL" or "BY-LT"                |

### Table: `snapshots` (one row per crawl per zone)

| Column        | Type         | Description                     |
|---------------|--------------|---------------------------------|
| id            | BIGSERIAL PK |                                 |
| zone_id       | VARCHAR FK   | References zones.id             |
| captured_at   | TIMESTAMPTZ  | When the crawl happened         |
| cars_count    | INT          | Total passenger cars in zone    |
| sent_last_hour| INT          | Cars sent to border last hour   |
| sent_last_24h | INT          | Cars sent last 24 hours         |

### Table: `vehicles` (individual queue entries)

| Column            | Type         | Description                   |
|-------------------|--------------|-------------------------------|
| id                | BIGSERIAL PK |                               |
| snapshot_id       | BIGINT FK    | References snapshots.id       |
| zone_id           | VARCHAR      | Denormalized for queries      |
| reg_number        | VARCHAR      | License plate (Latin chars)   |
| queue_type        | VARCHAR      | Queue category                |
| registered_at     | TIMESTAMPTZ  | When registered in zone       |
| status_changed_at | TIMESTAMPTZ  | Last status change            |
| status            | VARCHAR      | Current status                |

### Indexes

- `snapshots(zone_id, captured_at)`
- `vehicles(snapshot_id)`
- `vehicles(zone_id, registered_at)`

## Backend API

Go HTTP server using `net/http` + `chi` router. No auth (self-hosted internal tool).

| Method | Path                          | Description                                      |
|--------|-------------------------------|--------------------------------------------------|
| GET    | `/api/zones`                  | List all zones with latest snapshot counts        |
| GET    | `/api/zones/:id/snapshots`    | Time-series data. Params: `from`, `to`            |
| GET    | `/api/zones/:id/vehicles`     | Current vehicles in queue                         |
| GET    | `/api/zones/:id/vehicles/history` | Vehicle entries over time. Params: `from`, `to` |

### Auto-aggregation for `/snapshots`

Based on requested time range width:

- Range < 24h: raw 15-min snapshots
- 24h–7d: hourly averages
- 7d–3m: 6-hour averages
- 3m+: daily averages

## Frontend

**Vue 3 + Composition API + TypeScript + Vite + ECharts**

### Pages

1. **Dashboard** (`/`) — overview of all zones, each showing a sparkline (last 24h) + current car count
2. **Zone detail** (`/zone/:id`) — full ECharts time-series graph with duration dropdown + vehicle table

### Components

- `ZoneCard` — zone name, border label, current count, sparkline
- `QueueChart` — ECharts line chart with time-series data
- `DurationPicker` — dropdown with presets: 1h, 3h, 6h, 9h, 12h, 1d, 2d, 3d, 5d, 7d, 14d, 1m, 2m, 3m, 6m, 1y, All
- `VehicleTable` — sortable table of current vehicles

### Auto-refresh

Poll backend every 60 seconds for latest data on the active view.

## Crawler

Go binary running as a long-lived process with a 15-min ticker.

### Scraping strategy

1. **Primary:** Reverse-engineer the XHR API calls the Angular app makes and call them directly
2. **Fallback:** Use `chromedp` (headless Chrome in Go) to render the page and extract DOM data

### Crawl flow per tick

1. Fetch zone summary → extract aggregate car counts per zone
2. For each zone → fetch individual zone data → extract vehicle queue entries
3. Write one `snapshot` row + N `vehicle` rows per zone in a single DB transaction
4. Log success/failure per zone

### Error handling

If a zone fails, log it and continue with the others.

### Configuration (env vars)

- Crawl interval
- List of zones to scrape
- Database connection string

## Docker & Deployment

```yaml
services:
  postgres:
    image: postgres:16
    volumes: [pgdata:/var/lib/postgresql/data]

  crawler:
    build: ./crawler
    depends_on: [postgres]

  backend:
    build: ./backend
    depends_on: [postgres]
    ports: ["8080:8080"]

  frontend:
    build: ./frontend
    ports: ["80:80"]

volumes:
  pgdata:
```

### Docker images

- **Frontend:** multi-stage — `node:20` builds Vue app → `caddy:2-alpine` serves static files
- **Crawler & Backend:** multi-stage — `golang:1.22` builds → `alpine` runs binary
- **Migrations:** Run via backend on startup (golang-migrate)

### Deployment

`docker compose up -d` on a Linux machine.

## Storage estimate

- ~5-15 MB/week for all zones (passenger cars only)
- Aggregate snapshots: ~1 KB per crawl
- Vehicle records: ~200 bytes each

## Project structure

```
border/
├── crawler/          # Go crawler service
├── backend/          # Go API server
├── frontend/         # Vue 3 SPA
├── migrations/       # SQL migration files
├── docker-compose.yml
└── docs/plans/       # Design & planning docs
```
