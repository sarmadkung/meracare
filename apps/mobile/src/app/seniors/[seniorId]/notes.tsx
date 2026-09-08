import { can, type CareNote } from '@genxcare/contracts';
import { Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { ActivityIndicator, View } from 'react-native';

import { Button, Card, Illustration, Screen, Text, TextField } from '@/components/ui';
import { useCreateNote, useNotes, useUpdateNote } from '@/features/notes/use-notes';
import { useSenior } from '@/features/seniors/use-seniors';
import { ApiError } from '@/lib/api-error';
import { useTheme } from '@/theme';

export default function NotesScreen() {
  const { seniorId = '' } = useLocalSearchParams<{ seniorId: string }>();
  const theme = useTheme();
  const notes = useNotes(seniorId || null);
  const senior = useSenior(seniorId || null);
  const create = useCreateNote(seniorId);
  const update = useUpdateNote(seniorId);
  const [content, setContent] = useState('');
  const [editing, setEditing] = useState<CareNote | null>(null);

  const submit = async () => {
    const trimmed = content.trim();
    if (!trimmed) return;
    const saved = editing
      ? await update.mutateAsync({ noteId: editing.id, content: trimmed })
      : await create.mutateAsync({ content: trimmed });
    if (saved) {
      setContent('');
      setEditing(null);
    }
  };

  const error = create.error ?? update.error;

  return (
    <Screen scrollable>
      <Stack.Screen options={{ headerShown: true, title: 'Care notes' }} />
      <Text variant="pageHeading">Care notes</Text>
      <Text variant="body" color="secondary">
        Share useful observations with this care circle.
      </Text>

      {senior.data && can(senior.data, 'notes.create') ? (
        <Card>
          <TextField
            label={editing ? 'Edit your note' : 'Add a note'}
            value={content}
            onChangeText={setContent}
            multiline
            maxLength={4000}
            style={{ minHeight: 120, paddingTop: theme.spacing.md, textAlignVertical: 'top' }}
          />
          {error ? (
            <Text variant="secondary" color="danger">
              {error instanceof ApiError ? error.message : 'The note could not be saved.'}
            </Text>
          ) : null}
          <View style={{ flexDirection: 'row', gap: theme.spacing.sm }}>
            <Button
              label={editing ? 'Save note' : 'Add note'}
              loading={create.isPending || update.isPending}
              disabled={!content.trim()}
              onPress={() => void submit()}
            />
            {editing ? (
              <Button
                variant="secondary"
                label="Cancel"
                onPress={() => {
                  setEditing(null);
                  setContent('');
                }}
              />
            ) : null}
          </View>
        </Card>
      ) : null}

      {notes.isPending ? (
        <ActivityIndicator color={theme.colors.primary} />
      ) : notes.isError ? (
        <Card>
          <Text variant="sectionHeading">We could not load these notes</Text>
          <Button variant="secondary" label="Try again" onPress={() => void notes.refetch()} />
        </Card>
      ) : notes.data.length === 0 ? (
        <Card>
          <Illustration name="communication" height={140} />
          <Text variant="sectionHeading">No notes yet</Text>
          <Text variant="body" color="secondary">
            The first observation added by the care circle will appear here.
          </Text>
        </Card>
      ) : (
        notes.data.map((note) => (
          <Card key={note.id}>
            <Text variant="body">{note.content}</Text>
            <Text variant="secondary" color="secondary">
              {note.authorName} · {new Date(note.createdAt).toLocaleString()}
            </Text>
            {note.canEdit ? (
              <Button
                variant="secondary"
                label="Edit"
                onPress={() => {
                  setEditing(note);
                  setContent(note.content);
                }}
              />
            ) : null}
          </Card>
        ))
      )}
    </Screen>
  );
}
