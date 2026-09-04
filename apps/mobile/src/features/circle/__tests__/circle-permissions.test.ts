import { can, type CarePermission, type CircleMember, type Senior } from '@genxcare/contracts';

/**
 * Which controls the Care Circle screen offers.
 *
 * The screen decides from the reader's own permission list; the API enforces
 * the same rules, so these tests pin the UI to the same shape rather than
 * standing in for authorization.
 */

function senior(permissions: CarePermission[]): Senior {
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
    permissions,
    createdAt: '2026-08-16T09:00:00Z',
    updatedAt: '2026-08-16T09:00:00Z',
  };
}

function member(overrides: Partial<CircleMember> = {}): CircleMember {
  return {
    id: 'relationship-1',
    userId: 'user-1',
    displayName: 'Maria',
    role: 'professional_caregiver',
    permissions: ['senior.view', 'tasks.complete'],
    status: 'active',
    isSenior: false,
    isSelf: false,
    joinedAt: '2026-08-16T09:00:00Z',
    updatedAt: '2026-08-16T09:00:00Z',
    ...overrides,
  };
}

/** Mirrors the rule the Care Circle screen applies to each member row. */
function isRemovable(reader: Senior, target: CircleMember): boolean {
  return can(reader, 'members.manage') && !target.isSenior && !target.isSelf;
}

describe('care circle controls', () => {
  it('offers the invite control only with members.invite', () => {
    expect(can(senior(['senior.view', 'members.invite']), 'members.invite')).toBe(true);
    // A caregiver holds members.view but not members.invite.
    expect(can(senior(['senior.view', 'members.view']), 'members.invite')).toBe(false);
  });

  it('offers management controls only with members.manage', () => {
    const manager = senior(['members.view', 'members.manage']);
    const viewer = senior(['members.view']);

    expect(isRemovable(manager, member())).toBe(true);
    expect(isRemovable(viewer, member())).toBe(false);
  });

  // The senior's own membership is fixed; the API refuses to change it.
  it('never offers to remove the senior', () => {
    const manager = senior(['members.view', 'members.manage']);

    expect(isRemovable(manager, member({ isSenior: true, role: 'senior' }))).toBe(false);
  });

  // Removing yourself by accident would be a nasty surprise.
  it('never offers to remove yourself', () => {
    const manager = senior(['members.view', 'members.manage']);

    expect(isRemovable(manager, member({ isSelf: true }))).toBe(false);
  });
});

describe('invite form', () => {
  /** Mirrors how the invite screen seeds its selection. */
  function seedSelection(
    defaults: CarePermission[],
    grantable: CarePermission[],
  ): CarePermission[] {
    return defaults.filter((permission) => grantable.includes(permission));
  }

  // An inviter cannot pass on what they do not hold, so the form must not
  // pre-select it and produce a request the API will refuse.
  it('never pre-selects a permission the inviter lacks', () => {
    const defaults: CarePermission[] = ['senior.view', 'tasks.view', 'medications.record'];
    const grantable: CarePermission[] = ['senior.view', 'tasks.view'];

    expect(seedSelection(defaults, grantable)).toEqual(['senior.view', 'tasks.view']);
  });

  it('keeps everything the inviter can pass on', () => {
    const defaults: CarePermission[] = ['senior.view', 'tasks.view'];
    const grantable: CarePermission[] = ['senior.view', 'tasks.view', 'members.manage'];

    expect(seedSelection(defaults, grantable)).toEqual(defaults);
  });
});
