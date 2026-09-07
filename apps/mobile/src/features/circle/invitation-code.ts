/**
 * Reading and rendering invitation codes.
 *
 * A code leaves the app in a share message and comes back through a keyboard,
 * so it returns grouped, lowercased, or still wrapped in the link it was sent
 * in. Everything that means the same invitation is accepted here, and the
 * canonical form — uppercase, ungrouped — is what the API is given.
 *
 * The rules mirror `NormaliseToken` in `apps/api/internal/invitations/token.go`.
 * The two must agree: a code this module accepts and the API rejects is a code
 * the person cannot use.
 */

/**
 * Crockford's base32 alphabet: digits and uppercase letters, less I, L, O and
 * U. Those four are missing so no reader has to decide between 0 and O.
 */
const ALPHABET = '0123456789ABCDEFGHJKMNPQRSTVWXYZ';

/** Symbols per code. Twelve over a 32-symbol alphabet is 60 bits. */
const CODE_LENGTH = 12;

/** Symbols per group when the code is shown to a person. */
const GROUP_SIZE = 4;

/**
 * The letters Crockford excluded, mapped to the digits they are mistaken for.
 * U is absent deliberately: it is excluded rather than confusable, so a code
 * containing one is wrong rather than misread.
 */
const FOLDED: Record<string, string> = { I: '1', L: '1', O: '0' };

/** Pulls the last path segment out of an invitation link, if that is what this is. */
function segmentFromLink(raw: string): string | null {
  // Matches meracare://invitations/<code> and the exp://…/--/invitations/<code>
  // form Expo Go serves, anywhere within a longer message.
  const match = /invitations\/([^\s/?#]+)/i.exec(raw);
  return match?.[1] ?? null;
}

/**
 * Reads a code as a person supplied it and returns canonical form, or null if
 * it could not be a code at all.
 *
 * Accepts the code on its own, grouped or not, in any case, and the share link
 * that carries it — including when that link sits inside a longer message.
 */
export function parseInvitationCode(raw: string): string | null {
  const candidate = segmentFromLink(raw) ?? raw;

  let canonical = '';
  for (const symbol of candidate.toUpperCase()) {
    if (symbol === '-' || symbol.trim() === '') {
      // Grouping and stray whitespace carry no meaning.
      continue;
    }
    const folded = FOLDED[symbol] ?? symbol;
    if (!ALPHABET.includes(folded)) {
      return null;
    }
    canonical += folded;
  }

  return canonical.length === CODE_LENGTH ? canonical : null;
}

/**
 * Groups a code for display. Presentation only — the API is always given the
 * canonical form.
 *
 * Partial input is grouped as far as it goes, so the field reads correctly
 * while it is still being typed.
 */
export function formatInvitationCode(canonical: string): string {
  const groups: string[] = [];
  for (let start = 0; start < canonical.length; start += GROUP_SIZE) {
    groups.push(canonical.slice(start, start + GROUP_SIZE));
  }
  return groups.join('-');
}
