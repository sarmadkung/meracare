/**
 * Browser storage adapters.
 *
 * Browsers do not expose a Keychain/Keystore equivalent to JavaScript. Auth is
 * therefore kept in sessionStorage, not localStorage: reloads in the same tab
 * restore the session, while closing the tab removes it. The non-secret device
 * identifier uses localStorage so one installation remains stable.
 *
 * A memory fallback keeps static rendering and privacy-restricted browsers
 * usable without pretending persistence succeeded.
 */

interface StringStorage {
  getItem(key: string): Promise<string | null>;
  setItem(key: string, value: string): Promise<void>;
  removeItem(key: string): Promise<void>;
}

function browserStorage(kind: 'localStorage' | 'sessionStorage'): StringStorage {
  const fallback = new Map<string, string>();

  function storage(): Storage | null {
    try {
      if (typeof window === 'undefined') return null;
      return window[kind] ?? null;
    } catch {
      return null;
    }
  }

  return {
    async getItem(key) {
      return storage()?.getItem(key) ?? fallback.get(key) ?? null;
    },
    async setItem(key, value) {
      const target = storage();
      if (target === null) fallback.set(key, value);
      else target.setItem(key, value);
    },
    async removeItem(key) {
      fallback.delete(key);
      storage()?.removeItem(key);
    },
  };
}

/** Used only for the installation id, which is not a credential. */
export const secureStorage = browserStorage('localStorage');

/** Used by Supabase Auth; deliberately scoped to the current browser tab. */
export const sessionStorage = browserStorage('sessionStorage');
