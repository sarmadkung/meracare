# GenxCare Rename Rollout

**Goal:** Finish the MeraCare → GenxCare rename outside the repository, once PR #16 is merged.

**Why this exists:** the PR renames every identifier in the code, but several things that carry the old name live somewhere the code can't reach: a developer's local environment, the apps already installed on test phones, and third-party dashboards. None of them fail loudly. Each one below either breaks quietly or leaves something stale behind, so work through them in order.

**Scope:** the GitHub repository (`sarmadkung/meracare`) and the local checkout folder keep their names. Two seeder strings also keep the old name on purpose, because they carry behaviour: the default seed password `MeraCare-Seed-2026!` and the `meracare-seed-invite:` salt. See the PR description.

---

## 1. Before pulling the merge — on every test phone

- [ ] **Sign out of the app on every device that has it installed.**

  The rename changes the names the app stores data under, so an updated app can't find anything the old build saved:

  | Stored as | Holds | If you skip signing out |
  | --- | --- | --- |
  | `meracare.db` | the offline database, including care updates queued while offline | queued updates that never synced are lost |
  | `meracare.deviceId` | this phone's push registration | the old registration stays active on the server |
  | `meracare-snooze-*` | scheduled reminder notifications | old reminders keep firing and can't be cancelled |
  | `meracare.themePreference` | the chosen theme | resets to System (harmless) |

  Signing out already syncs the offline queue, unregisters the device and clears reminders ([sign-out.ts](../../../apps/mobile/src/features/auth/sign-out.ts)), which covers the first three. The app has never been released, so only development installs are affected.

## 2. Before pulling the merge — local Postgres

- [ ] **Remove the old container and its volume while the old compose file is still checked out.**

  ```
  docker compose down -v
  ```

  The container and its volume are both renamed (`meracare-postgres-data` → `genxcare-postgres-data`). Once you've pulled, `docker compose` only knows the new names, so the old volume is left behind with nothing managing it. This only holds integration-test data, which the test suite wipes anyway.

## 3. Pull and rebuild

- [ ] Pull the merge.

  ```
  git checkout main && git pull
  ```

- [ ] Relink the renamed workspace packages. `@meracare/*` becomes `@genxcare/*`, and `node_modules` still has links under the old names.

  ```
  pnpm install
  ```

- [ ] Clear Expo's cached module map, which still points at `@meracare/*`, then restart Metro with a clean cache.

  ```
  rm -rf apps/mobile/.expo
  cd apps/mobile && npx expo start --ios --localhost --clear
  ```

- [ ] Recreate local Postgres under the new name.

  ```
  docker compose up -d postgres
  ```

## 4. Local environment files

These files are gitignored, so the PR can't update them.

- [ ] In `apps/api/.env`, point `TEST_DATABASE_URL` at the new role and database:

  ```
  TEST_DATABASE_URL=postgres://genxcare:genxcare@localhost:55432/genxcare?sslmode=disable
  ```

  Also update the commented-out local `DATABASE_URL` line, if you use it. The live `DATABASE_URL` points at Supabase and doesn't change.

- [ ] Confirm the Go suite runs against the new database. The integration tests are the part that exercises the renamed credentials.

  ```
  cd apps/api && go test ./...
  ```

## 5. Supabase

- [ ] **Add `genxcare://auth/callback`** under Authentication → URL Configuration → Redirect URLs.

  The app's scheme changes from `meracare` to `genxcare`, so a development or production build now sends this URL. Until it's on the list, Google sign-in fails the confusing way: the browser lands on Site URL (`localhost:3000`) instead of returning to the app. Expo Go sends `exp://…` and isn't affected.

- [ ] Remove `meracare://auth/callback`, once no installed build still uses the old scheme.

## 6. External accounts and branding

These fall outside the code, so check each one rather than assuming.

- [ ] **Google Cloud → OAuth consent screen:** if the app name reads MeraCare, rename it. People see this name on the Google sign-in screen.
- [ ] **Domain and mailbox:** the marketing site now points at `genxcare.app` and `hello@genxcare.app`. Make sure you own the domain and that the mailbox receives mail before anything public links to them.
- [ ] **Apple:** the bundle identifier is now `app.genxcare.mobile`. When setting up Apple sign-in, the App Store record or push credentials, register this identifier, not the old one. None of these exist yet, so there's nothing to migrate.

## 7. Clean up

- [ ] Delete the local rename branch if it still exists (`gh pr merge --delete-branch` normally removes it).

  ```
  git branch -D rename-to-genxcare
  ```

- [ ] Search the checkout for the old name. Exactly two hits are expected, both in the seeder, both deliberate (the command skips this document, which names the old brand throughout):

  ```
  git grep -niI 'mera.\?care' -- ':!docs/superpowers/plans/2026-09-14-genxcare-rename-rollout.md'
  ```
