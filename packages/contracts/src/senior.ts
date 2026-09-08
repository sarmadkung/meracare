import type { CarePermission, CareRole } from './care';

/**
 * A senior profile as returned by `/v1/seniors`.
 *
 * `role` and `permissions` describe the *caller's* relationship to this senior,
 * not the senior themselves — the same profile looks different to a daughter
 * and to a visiting caregiver. The client renders actions from `permissions`;
 * the API enforces them regardless (docs/02-permissions-and-authorization.md).
 */
export interface Senior {
  id: string;
  displayName: string;
  /** `YYYY-MM-DD`, or null when unknown. */
  dateOfBirth: string | null;
  photoUrl: string | null;
  phone: string | null;
  address: string | null;
  emergencyContact: string | null;
  /**
   * IANA timezone name. Care is scheduled in the senior's own day, so due times
   * are rendered in this zone rather than the device's.
   */
  timezone: string;
  /** True when this profile represents the caller themselves (Solo Mode). */
  isSelf: boolean;
  role: CareRole;
  permissions: CarePermission[];
  createdAt: string;
  updatedAt: string;
}

/** How the creator relates to the senior they are adding. */
export const SENIOR_CREATE_MODES = ['self', 'family_member', 'professional_caregiver'] as const;
export type SeniorCreateMode = (typeof SENIOR_CREATE_MODES)[number];

/** `POST /v1/seniors` request body. */
export interface CreateSeniorRequest {
  mode: SeniorCreateMode;
  displayName: string;
  dateOfBirth?: string | null;
  phone?: string | null;
  address?: string | null;
  emergencyContact?: string | null;
  /** IANA name. Send the device's zone at sign-up; omitted means UTC. */
  timezone?: string;
}

/** `PATCH /v1/seniors/{id}` request body. Absent fields are left unchanged. */
export interface UpdateSeniorRequest {
  displayName?: string;
  dateOfBirth?: string | null;
  phone?: string | null;
  address?: string | null;
  emergencyContact?: string | null;
  timezone?: string;
}

/** `GET /v1/seniors` response. */
export interface SeniorListResponse {
  items: Senior[];
}

/** Result of removing a managed profile. */
export interface SeniorRemovalResponse {
  disposition: 'deleted' | 'archived';
}

/** Reports whether the caller may perform an action for this senior. */
export function can(senior: Senior, permission: CarePermission): boolean {
  return senior.permissions.includes(permission);
}

/** How much of something is finished, out of how much there is. */
export interface CareCounts {
  done: number;
  total: number;
}

/**
 * One senior's day at a glance.
 *
 * `medications` and `tasks` are null when the reader lacks permission to view
 * that domain, which is not the same as zero. Zero doses is a fact about the
 * senior; null is a fact about the reader, and showing "0/0" to a caregiver who
 * simply cannot see medication would tell them something untrue.
 */
export interface SeniorSummary {
  seniorId: string;
  medications: CareCounts | null;
  tasks: CareCounts | null;
  /** Overdue tasks plus missed doses, counted only across visible domains. */
  needsAttention: number;
}

/** Response shape of `GET /v1/seniors/summary`. */
export interface SeniorSummaryListResponse {
  items: SeniorSummary[];
}
