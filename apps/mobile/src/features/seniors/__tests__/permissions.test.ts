import { can, type Senior } from '@genxcare/contracts';

/**
 * The client renders actions from the caller's permission list. These tests pin
 * that behaviour: the UI must never offer an action the API would reject, and
 * must not hide one it would allow.
 *
 * Hiding an action is convenience only — the API enforces the same rules
 * (docs/02-permissions-and-authorization.md).
 */

function senior(overrides: Partial<Senior>): Senior {
  return {
    id: 'senior-1',
    displayName: 'Mrs Khan',
    dateOfBirth: null,
    photoUrl: null,
    phone: null,
    address: null,
    emergencyContact: null,
    timezone: 'Asia/Karachi',
    isSelf: false,
    role: 'family_member',
    permissions: [],
    createdAt: '2026-08-15T09:00:00Z',
    updatedAt: '2026-08-15T09:00:00Z',
    ...overrides,
  };
}

describe('can', () => {
  it('permits an action the caller holds', () => {
    const profile = senior({ permissions: ['senior.view', 'senior.edit'] });

    expect(can(profile, 'senior.edit')).toBe(true);
  });

  it('refuses an action the caller does not hold', () => {
    // A professional caregiver may view a profile but not edit it.
    const profile = senior({
      role: 'professional_caregiver',
      permissions: ['senior.view', 'tasks.complete', 'medications.record'],
    });

    expect(can(profile, 'senior.view')).toBe(true);
    expect(can(profile, 'senior.edit')).toBe(false);
  });

  it('refuses everything when no permissions are granted', () => {
    const profile = senior({ permissions: [] });

    expect(can(profile, 'senior.view')).toBe(false);
    expect(can(profile, 'tasks.view')).toBe(false);
  });

  // The role name is a label; the permission list is what the API enforces.
  it('does not infer capabilities from the role', () => {
    const profile = senior({ role: 'family_member', permissions: ['senior.view'] });

    expect(can(profile, 'senior.edit')).toBe(false);
  });
});
