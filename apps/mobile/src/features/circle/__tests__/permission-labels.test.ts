import {
  CARE_PERMISSIONS,
  permissionLabel,
  permissionLabels,
  permissionLabelsByGroup,
  roleLabel,
  type CarePermission,
} from '@genxcare/contracts';

/**
 * The invite and access screens must never show a raw permission identifier.
 * These tests pin that: every permission the domain defines has a plain-language
 * label, and the grouping used by the forms covers all of them.
 */

describe('permissionLabel', () => {
  it('describes every permission in plain language', () => {
    for (const permission of CARE_PERMISSIONS) {
      const described = permissionLabel(permission);

      expect(described.label.length).toBeGreaterThan(0);
      expect(described.description.length).toBeGreaterThan(0);
      // The identifier itself must not leak into what the user reads.
      expect(described.label).not.toContain('.');
      expect(described.label).not.toBe(permission);
      expect(described.description).not.toContain(permission);
    }
  });

  it('returns the permission it described', () => {
    expect(permissionLabel('medications.record').permission).toBe('medications.record');
    expect(permissionLabel('medications.record').label).toBe('Record medications');
  });
});

describe('permissionLabels', () => {
  it('covers the whole vocabulary exactly once', () => {
    const described = permissionLabels();

    expect(described).toHaveLength(CARE_PERMISSIONS.length);
    expect(new Set(described.map((entry) => entry.permission)).size).toBe(CARE_PERMISSIONS.length);
  });
});

describe('permissionLabelsByGroup', () => {
  it('places every permission in exactly one group', () => {
    const sections = permissionLabelsByGroup();
    const grouped = sections.flatMap((section) => section.permissions.map((e) => e.permission));

    expect(grouped.sort()).toEqual([...CARE_PERMISSIONS].sort());
  });

  // The invite form shows only what the inviter can actually delegate.
  it('narrows to the permissions it is given', () => {
    const grantable: CarePermission[] = ['senior.view', 'tasks.view', 'members.view'];

    const sections = permissionLabelsByGroup(grantable);
    const shown = sections.flatMap((section) => section.permissions.map((e) => e.permission));

    expect(shown.sort()).toEqual([...grantable].sort());
  });

  it('omits groups with nothing to show', () => {
    const sections = permissionLabelsByGroup(['senior.view']);

    expect(sections).toHaveLength(1);
    expect(sections[0]?.group).toBe('Care information');
  });

  it('returns no sections when nothing can be granted', () => {
    expect(permissionLabelsByGroup([])).toHaveLength(0);
  });
});

describe('roleLabel', () => {
  it('describes each role in plain language', () => {
    expect(roleLabel('senior')).toBe('The person being cared for');
    expect(roleLabel('family_member')).toBe('Family member');
    expect(roleLabel('professional_caregiver')).toBe('Professional caregiver');
  });
});
