import { formatInvitationCode, parseInvitationCode } from '../invitation-code';

describe('parseInvitationCode', () => {
  it('accepts a code in canonical form', () => {
    expect(parseInvitationCode('K7M29XQP4WTZ')).toBe('K7M29XQP4WTZ');
  });

  it.each([
    ['grouped', 'K7M2-9XQP-4WTZ'],
    ['lowercase', 'k7m29xqp4wtz'],
    ['spaced', 'K7M2 9XQP 4WTZ'],
    ['grouped and lowercase', 'k7m2-9xqp-4wtz'],
    ['surrounded by whitespace', '  K7M29XQP4WTZ\n'],
  ])('accepts a code that is %s', (_label, raw) => {
    expect(parseInvitationCode(raw)).toBe('K7M29XQP4WTZ');
  });

  // The alphabet has no I, L, O or U, so a reader who saw one meant a digit.
  it('folds letters that are mistaken for digits', () => {
    expect(parseInvitationCode('OIL29XQP4WTZ')).toBe('01129XQP4WTZ');
  });

  it('extracts the code from a shared app link', () => {
    expect(parseInvitationCode('meracare://invitations/K7M29XQP4WTZ')).toBe('K7M29XQP4WTZ');
  });

  // Expo Go serves the same route under a development URL.
  it('extracts the code from an Expo development link', () => {
    expect(parseInvitationCode('exp://192.168.1.105:8084/--/invitations/K7M29XQP4WTZ')).toBe(
      'K7M29XQP4WTZ',
    );
  });

  it('extracts the code from a link pasted inside a longer message', () => {
    const message = [
      "Ahmed invited you to help with Mrs Khan's care.",
      'meracare://invitations/K7M29XQP4WTZ',
    ].join('\n');

    expect(parseInvitationCode(message)).toBe('K7M29XQP4WTZ');
  });

  it.each([
    ['empty', ''],
    ['blank', '   '],
    ['too short', 'K7M29XQP4WT'],
    ['too long', 'K7M29XQP4WTZ9'],
    ['an excluded letter', 'K7M29XQP4WTU'],
    ['punctuation', 'K7M29XQP4WT!'],
    ['a link with a malformed code', 'meracare://invitations/nope'],
  ])('rejects a code that is %s', (_label, raw) => {
    expect(parseInvitationCode(raw)).toBeNull();
  });
});

describe('formatInvitationCode', () => {
  it('groups a canonical code into threes for reading', () => {
    expect(formatInvitationCode('K7M29XQP4WTZ')).toBe('K7M2-9XQP-4WTZ');
  });

  // Whatever the field holds mid-typing must still render.
  it('groups a partial code without padding it', () => {
    expect(formatInvitationCode('K7M29')).toBe('K7M2-9');
  });

  it('returns an empty string unchanged', () => {
    expect(formatInvitationCode('')).toBe('');
  });
});
