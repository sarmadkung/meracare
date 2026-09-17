# GenxCare — Phase 2: User & Senior Foundation

> **Archived brief:** this historical prompt was captured incompletely and ends
> mid-section. It is retained as implementation history, not as a current or
> complete specification. The implemented Phase 2 behavior is recorded in
> `docs/IMPLEMENTATION_STATUS.md`; current requirements remain in `/docs`.

Phase 1 is complete.

Now implement **Phase 2** according to the existing documentation.

Do not start Phase 3 or implement unrelated features.

## Objective

Build the complete foundation for:

- authenticated users
- application user profiles
- senior profiles
- care relationships
- solo/self-care mode
- senior dashboard foundation

The implementation must support both:

1. A user caring for themselves.
2. A user caring for another senior.

The architecture must also support future family and professional caregiver relationships without requiring a rewrite.

## Before Starting

Read the relevant documentation:

- `docs/00-product-overview.md`
- `docs/01-roles-and-care-model.md`
- `docs/02-permissions-and-authorization.md`
- `docs/03-domain-model.md`
- `docs/05-api-and-backend-spec.md`
- `docs/06-mobile-architecture.md`
- `docs/07-database-and-sync.md`
- `docs/09-security-privacy.md`
- `docs/12-tech-stack.md`
- `docs/13-mvp-screen-map.md`
- `docs/14-mvp-roadmap-and-tasks.md`
- `docs/18-visual-theme-and-illustrations.md`

Also inspect everything implemented during Phase 1 before making changes.

Follow `AGENTS.md`.

## Phase 2 Scope

Implement:

1. Application User
2. Senior Profile
3. Care Relationship
4. Solo/self-care mode
5. Create senior flow
6. Edit senior flow
7. Senior selection
8. Senior dashboard foundation
9. Backend authorization foundation
10. Required tests

---

# 1. Application User

Implement the application-level user model linked to the authenticated Supabase user.

The application user should contain only information required by the product.

Do not duplicate the complete Supabase Auth user.

The relationship should conceptually be:

```text
Supabase auth.users
        ↓
Application User
```

<!-- The original captured brief ends here. Do not infer missing requirements. -->
