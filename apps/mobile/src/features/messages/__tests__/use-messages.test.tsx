import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react-native';
import type { ReactNode } from 'react';

import { useMarkMessagesRead, useMessages, useSendMessage } from '../use-messages';

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

it('pages through the senior conversation', async () => {
  const { client, wrapper } = setup();
  mockRequest
    .mockResolvedValueOnce({ items: [{ id: 'new' }], nextCursor: 'older', unreadCount: 1 })
    .mockResolvedValueOnce({ items: [{ id: 'old' }], nextCursor: null, unreadCount: 1 });
  const { result, unmount } = renderHook(() => useMessages('senior-1'), { wrapper });
  await waitFor(() => expect(result.current.isSuccess).toBe(true));
  await result.current.fetchNextPage();
  expect(mockRequest.mock.calls[1]?.[0]).toBe('/seniors/senior-1/messages?cursor=older');
  unmount();
  client.clear();
  client.unmount();
});

it('sends a message and advances read state explicitly', async () => {
  const { client, wrapper } = setup();
  mockRequest.mockResolvedValue({ id: 'message-1' });
  const send = renderHook(() => useSendMessage('senior-1'), { wrapper });
  const read = renderHook(() => useMarkMessagesRead('senior-1'), { wrapper });

  await send.result.current.mutateAsync({ content: 'I will arrive at ten.' });
  await read.result.current.mutateAsync({ throughMessageId: 'message-1' });

  expect(mockRequest.mock.calls[0]).toEqual([
    '/seniors/senior-1/messages',
    { method: 'POST', body: { content: 'I will arrive at ten.' } },
  ]);
  expect(mockRequest.mock.calls[1]).toEqual([
    '/seniors/senior-1/messages/read',
    { method: 'POST', body: { throughMessageId: 'message-1' } },
  ]);
  send.unmount();
  read.unmount();
  client.clear();
  client.unmount();
});
