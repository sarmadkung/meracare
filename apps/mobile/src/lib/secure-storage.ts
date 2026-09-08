import * as SecureStore from 'expo-secure-store';

/**
 * Keychain/Keystore-backed storage for the Supabase session.
 *
 * docs/09-security-privacy.md forbids storing auth tokens in plain local
 * storage. SecureStore warns above ~2KB per entry and a Supabase session with a
 * long JWT can exceed that, so values are split across numbered chunks.
 */

const CHUNK_SIZE = 1800;

/** Chunk 0 stores the chunk count so a value can be reassembled or removed. */
const countKey = (key: string) => `${key}.chunks`;
const chunkKey = (key: string, index: number) => `${key}.${index}`;

async function removeChunks(key: string, count: number): Promise<void> {
  const deletions: Promise<void>[] = [SecureStore.deleteItemAsync(countKey(key))];
  for (let index = 0; index < count; index += 1) {
    deletions.push(SecureStore.deleteItemAsync(chunkKey(key, index)));
  }
  await Promise.all(deletions);
}

async function readChunkCount(key: string): Promise<number> {
  const raw = await SecureStore.getItemAsync(countKey(key));
  const count = raw === null ? 0 : Number.parseInt(raw, 10);
  return Number.isFinite(count) && count > 0 ? count : 0;
}

export const secureStorage = {
  async getItem(key: string): Promise<string | null> {
    const count = await readChunkCount(key);
    if (count === 0) return null;

    const chunks = await Promise.all(
      Array.from({ length: count }, (_, index) => SecureStore.getItemAsync(chunkKey(key, index))),
    );
    // A partially written value is unusable; treat it as absent so the user is
    // asked to sign in again rather than handed a corrupt session.
    if (chunks.some((chunk) => chunk === null)) {
      await removeChunks(key, count);
      return null;
    }
    return chunks.join('');
  },

  async setItem(key: string, value: string): Promise<void> {
    const previousCount = await readChunkCount(key);

    const chunks: string[] = [];
    for (let offset = 0; offset < value.length; offset += CHUNK_SIZE) {
      chunks.push(value.slice(offset, offset + CHUNK_SIZE));
    }
    // An empty string still needs one chunk so it round-trips as "" not null.
    if (chunks.length === 0) chunks.push('');

    await Promise.all(
      chunks.map((chunk, index) => SecureStore.setItemAsync(chunkKey(key, index), chunk)),
    );
    await SecureStore.setItemAsync(countKey(key), String(chunks.length));

    // Drop chunks left over from a longer previous value.
    for (let index = chunks.length; index < previousCount; index += 1) {
      await SecureStore.deleteItemAsync(chunkKey(key, index));
    }
  },

  async removeItem(key: string): Promise<void> {
    await removeChunks(key, await readChunkCount(key));
  },
};

/** Native Supabase sessions use the same Keychain/Keystore-backed adapter. */
export const sessionStorage = secureStorage;
