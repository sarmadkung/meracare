# GenxCare — Agent Instructions

## Source of Truth

The `/docs` directory is the source of truth for the product and engineering requirements.

Before implementing anything, read the relevant documentation in `/docs`.

Do not invent requirements or change documented decisions without approval.

## Product

GenxCare is a senior care and family coordination platform supporting:

- Solo self-care
- Family care
- Professional caregivers
- Mixed family + professional care

GenxCare is one application. Do not create separate applications for these user types.

A senior can use GenxCare without a caregiver.

A professional caregiver can manage multiple seniors.

Family members and professional caregivers can collaborate around the same senior.

## Locked Technology Stack

### Mobile

- React Native
- Expo
- TypeScript
- Expo Router
- TanStack Query
- Zustand
- React Native `StyleSheet`
- `expo-sqlite`
- `expo-secure-store`

### Backend

- Go
- REST API
- Modular monolith

### Database

- PostgreSQL
- Supabase

### Authentication

- Supabase Auth

### Storage

- Supabase Storage where appropriate

### Repository

- pnpm workspace

### Future Web

- Next.js
- TypeScript
- Same Go API
- Same Supabase Auth

Do not switch technologies without explicit approval.

Do not introduce:

- Flutter
- Redux
- MongoDB
- Microservices
- Styling frameworks
- Unnecessary dependencies

## Architecture Rules

The mobile application communicates with the Go API.

```text
React Native
    ↓
Supabase Auth
    ↓
Go API
    ↓
PostgreSQL / Supabase
```
