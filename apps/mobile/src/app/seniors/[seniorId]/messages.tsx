import type { CareMessage } from '@genxcare/contracts';
import { Stack, useLocalSearchParams } from 'expo-router';
import { useEffect, useMemo, useRef, useState } from 'react';
import { ActivityIndicator, View } from 'react-native';

import { Button, Card, Illustration, Screen, Text, TextField } from '@/components/ui';
import { useMarkMessagesRead, useMessages, useSendMessage } from '@/features/messages/use-messages';
import { ApiError } from '@/lib/api-error';
import { useTheme } from '@/theme';

export default function MessagesScreen() {
  const { seniorId = '' } = useLocalSearchParams<{ seniorId: string }>();
  const theme = useTheme();
  const messages = useMessages(seniorId || null);
  const send = useSendMessage(seniorId);
  const markRead = useMarkMessagesRead(seniorId);
  const lastMarkedRead = useRef<string | null>(null);
  const [content, setContent] = useState('');
  const items = useMemo(
    () => messages.data?.pages.flatMap((page) => page.items) ?? [],
    [messages.data],
  );

  useEffect(() => {
    const newest = items[0];
    if (
      newest &&
      (messages.data?.pages[0]?.unreadCount ?? 0) > 0 &&
      !markRead.isPending &&
      lastMarkedRead.current !== newest.id
    ) {
      lastMarkedRead.current = newest.id;
      markRead.mutate(
        { throughMessageId: newest.id },
        {
          onError: () => {
            lastMarkedRead.current = null;
          },
        },
      );
    }
  }, [items, markRead, messages.data?.pages]);

  const submit = async () => {
    const trimmed = content.trim();
    if (!trimmed) return;
    await send.mutateAsync({ content: trimmed });
    setContent('');
  };

  return (
    <Screen scrollable>
      <Stack.Screen options={{ headerShown: true, title: 'Messages' }} />
      <Text variant="pageHeading">Care-circle messages</Text>
      <Text variant="body" color="secondary">
        Coordinate around this senior with everyone who has chat access.
      </Text>

      <Card>
        <TextField
          label="Message"
          value={content}
          onChangeText={setContent}
          multiline
          maxLength={2000}
          style={{ minHeight: 88, paddingTop: theme.spacing.md, textAlignVertical: 'top' }}
        />
        {send.error ? (
          <Text variant="secondary" color="danger">
            {send.error instanceof ApiError ? send.error.message : 'The message could not be sent.'}
          </Text>
        ) : null}
        <Button
          label="Send"
          loading={send.isPending}
          disabled={!content.trim()}
          onPress={() => void submit()}
        />
      </Card>

      {messages.isPending ? (
        <ActivityIndicator color={theme.colors.primary} />
      ) : messages.isError ? (
        <Card>
          <Text variant="sectionHeading">We could not load these messages</Text>
          <Button variant="secondary" label="Try again" onPress={() => void messages.refetch()} />
        </Card>
      ) : items.length === 0 ? (
        <Card>
          <Illustration name="communication" height={140} />
          <Text variant="sectionHeading">No messages yet</Text>
          <Text variant="body" color="secondary">
            Start the conversation when there is something the circle should know.
          </Text>
        </Card>
      ) : (
        items.map((message: CareMessage) => (
          <Card key={message.id}>
            <Text variant="bodyStrong">{message.sentByMe ? 'You' : message.senderName}</Text>
            <Text variant="body">{message.content}</Text>
            <Text variant="secondary" color="secondary">
              {new Date(message.createdAt).toLocaleString()}
            </Text>
          </Card>
        ))
      )}

      {messages.hasNextPage ? (
        <Button
          variant="secondary"
          label="Show earlier messages"
          loading={messages.isFetchingNextPage}
          onPress={() => void messages.fetchNextPage()}
        />
      ) : null}
      <View style={{ height: theme.spacing.lg }} />
    </Screen>
  );
}
