# GenXcare

Senior care and family coordination platform.

`docs/` is the engineering source of truth. Current build state, decisions, and
next tasks live in [`docs/IMPLEMENTATION_STATUS.md`](docs/IMPLEMENTATION_STATUS.md).
The implemented REST route contract is in
[`docs/openapi.yaml`](docs/openapi.yaml), and launch gates are tracked in
[`docs/21-release-readiness.md`](docs/21-release-readiness.md).

## Repository

```text
apps/mobile        Expo / React Native app
apps/api           Go modular monolith + REST API
packages/contracts TypeScript contracts mirroring the Go API
packages/config    Shared TypeScript / Prettier configuration
```

## Getting Started

```bash
pnpm install

pnpm db:up                              # local PostgreSQL on port 55432
cp apps/api/.env.example apps/api/.env
pnpm api:migrate
pnpm api:run                            # http://localhost:8080

cp apps/mobile/.env.example apps/mobile/.env
pnpm mobile
```

Checks: `pnpm typecheck`, `pnpm test`, `pnpm api:test`. The integration tests
read `TEST_DATABASE_URL` from `apps/api/.env`, so they run once the database is
up; unset, they skip. It must be a local database — the suite truncates every
application table, and anything else is refused before it connects.

## Architecture Status

The MVP baseline is **accepted and locked**.

### Mobile

React Native + Expo + TypeScript

### State

TanStack Query + small Zustand

### Styling

Native React Native `StyleSheet` with semantic theme tokens

### Visual Theme

Deep Teal / green-blue direction:

`#0F766E`

### Illustrations

- unDraw --- primary
- Storyset --- secondary

### Local/offline

`expo-sqlite`

### Backend

Go modular monolith + REST API

### Database

PostgreSQL hosted on Supabase

### Authentication

Supabase Auth, with Apple + Google initially

### Storage

Supabase Storage where appropriate

### Web later

Next.js + TypeScript using the same Go API and Supabase Auth

### Repository

pnpm workspace

## Product Modes

The same application supports:

1.  Solo self-care.
2.  Family care.
3.  Professional caregiver.
4.  Mixed family + professional care.

A person can start alone and invite family or professional caregivers
later.

## Start Here

1.  `17-architecture-decision-record.md`
2.  `00-product-overview.md`
3.  `01-roles-and-care-model.md`
4.  `03-domain-model.md`
5.  `04-care-events-and-workflows.md`
6.  `12-tech-stack.md`
7.  `18-visual-theme-and-illustrations.md`
8.  `14-mvp-roadmap-and-tasks.md`
9.  `16-agent-development-guide.md`

## Important Architectural Principle

The product is not two apps.

It is one care platform where the user's relationship to a senior
determines permissions and available workflows.

## MVP Objective

Prove that seniors, families, and professional caregivers can reliably
coordinate daily care through a shared workspace.

Do not implement V2/V3/V4 functionality until the MVP has been
validated.

## Locked Visual System

### Color palette

Green with a slight blue/teal bias.

Primary:

`#0F766E` --- Deep Teal

Supporting colors are defined in
`docs/18-visual-theme-and-illustrations.md`.

### Illustrations

- **unDraw** --- primary illustration source
- **Storyset** --- secondary illustration source

The complete visual specification is
`docs/18-visual-theme-and-illustrations.md`.
