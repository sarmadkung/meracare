import type { Href } from 'expo-router';
import { create } from 'zustand';

export interface PendingMedicationNotificationAction {
  action: 'take' | 'skip';
  doseId: string;
  seniorId: string;
}

/**
 * Small client-only state.
 *
 * Server data belongs in TanStack Query; this store holds nothing that the API
 * owns (docs/06-mobile-architecture.md).
 */
interface UIState {
  /** The senior currently in focus. A caregiver may manage several. */
  selectedSeniorId: string | null;
  setSelectedSeniorId: (seniorId: string | null) => void;

  /** Whether the user has completed the onboarding flow on this device. */
  hasCompletedOnboarding: boolean;
  setHasCompletedOnboarding: (completed: boolean) => void;

  /**
   * Where a tapped notification was trying to go, when it could not go there
   * yet.
   *
   * A reminder can be tapped from a lock screen while the app is signed out or
   * still restoring its session. Navigating immediately would land on a screen
   * that bounces to sign-in, and the destination would be lost — so it is kept
   * here and consumed once the user is through (plans/phase9.md §26).
   *
   * Client-only navigation state, which is what this store is for. It is
   * deliberately not persisted: a destination is only worth honouring in the
   * moments after the tap.
   */
  pendingDestination: Href | null;
  setPendingDestination: (destination: Href | null) => void;

  /** A notification action waiting for session restoration or sign-in. */
  pendingMedicationNotificationAction: PendingMedicationNotificationAction | null;
  setPendingMedicationNotificationAction: (
    action: PendingMedicationNotificationAction | null,
  ) => void;

  reset: () => void;
}

export const useUIStore = create<UIState>((set) => ({
  selectedSeniorId: null,
  setSelectedSeniorId: (selectedSeniorId) => set({ selectedSeniorId }),

  hasCompletedOnboarding: false,
  setHasCompletedOnboarding: (hasCompletedOnboarding) => set({ hasCompletedOnboarding }),

  pendingDestination: null,
  setPendingDestination: (pendingDestination) => set({ pendingDestination }),

  pendingMedicationNotificationAction: null,
  setPendingMedicationNotificationAction: (pendingMedicationNotificationAction) =>
    set({ pendingMedicationNotificationAction }),

  reset: () =>
    set({
      selectedSeniorId: null,
      hasCompletedOnboarding: false,
      pendingDestination: null,
      pendingMedicationNotificationAction: null,
    }),
}));
