import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react-native';
import type { ReactNode } from 'react';

import { useCreateNote, useNotes, useUpdateNote } from '../use-notes';

const mockRequest = jest.fn();
jest.mock('@/lib/api-client', () => ({ apiRequest: (...args: unknown[]) => mockRequest(...args) }));

function setup() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: Infinity }, mutations: { retry: false } },
  });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  return { client, wrapper };
}

beforeEach(() => mockRequest.mockReset());

it('loads senior-scoped notes', async () => {
  const { client, wrapper } = setup();
  mockRequest.mockResolvedValue({ items: [{ id: 'note-1' }] });
  const { result, unmount } = renderHook(() => useNotes('senior-1'), { wrapper });
  await waitFor(() => expect(result.current.isSuccess).toBe(true));
  expect(mockRequest).toHaveBeenCalledWith('/seniors/senior-1/notes');
  expect(result.current.data).toEqual([{ id: 'note-1' }]);
  unmount();
  client.clear();
  client.unmount();
});

it('creates and updates only through their documented routes', async () => {
  const { client, wrapper } = setup();
  mockRequest.mockResolvedValue({ id: 'note-1' });
  const create = renderHook(() => useCreateNote('senior-1'), { wrapper });
  const update = renderHook(() => useUpdateNote('senior-1'), { wrapper });

  await create.result.current.mutateAsync({ content: 'Ate lunch well.' });
  await update.result.current.mutateAsync({ noteId: 'note-1', content: 'Ate all of lunch.' });

  expect(mockRequest.mock.calls[0]).toEqual([
    '/seniors/senior-1/notes',
    { method: 'POST', body: { content: 'Ate lunch well.' } },
  ]);
  expect(mockRequest.mock.calls[1]).toEqual([
    '/notes/note-1',
    { method: 'PATCH', body: { content: 'Ate all of lunch.' } },
  ]);
  create.unmount();
  update.unmount();
  client.clear();
  client.unmount();
});
