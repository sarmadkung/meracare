import type { Appointment, CareTask, MedicationDose, Senior } from '@genxcare/contracts';
import {
  appointmentStatusLabel,
  appointmentWhenLabel,
  can,
  careEventDescription,
  careEventTimeLabel,
  doseStatusLabel,
  doseTimeLabel,
  statusLabel,
  taskTimeLabel,
} from '@genxcare/contracts';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import type { Href } from 'expo-router';
import { useMemo, type ReactNode } from 'react';
import { ActivityIndicator, Pressable, StyleSheet, View } from 'react-native';

import { Button, Card, Screen, Text } from '@/components/ui';
import { useActivity } from '@/features/activity/use-activity';
import { useAppointments } from '@/features/appointments/use-appointments';
import { useCircleMembers } from '@/features/circle/use-circle';
import { useMedicationDoses } from '@/features/medications/use-medications';
import { useRemoveSenior, useSenior } from '@/features/seniors/use-seniors';
import { useSeniorTasks } from '@/features/tasks/use-tasks';
import { ApiError } from '@/lib/api-error';
import { confirmAction, showMessage } from '@/lib/dialogs';
import { useTheme } from '@/theme';

/**
 * Senior dashboard (docs/13-mvp-screen-map.md, screen 11).
 *
 * The one screen a family opens every day, so it answers the question they
 * actually arrive with — "what does my mother need today, and has it happened?"
 * — rather than presenting a menu of domains to go looking through
 * (plans/phase9.md §§8, 9).
 *
 * It summarises and links; it never duplicates a domain screen. Each section is
 * a few lines and a way in. The same screen serves every care mode, and the
 * caller's permissions decide which sections exist at all.
 */
export default function SeniorDashboardScreen() {
  const theme = useTheme();
  const { seniorId } = useLocalSearchParams<{ seniorId: string }>();
  const senior = useSenior(seniorId ?? null);

  if (senior.isPending) {
    return (
      <Screen>
        <Stack.Screen options={{ headerShown: true, title: '' }} />
        <View style={styles.centred}>
          <ActivityIndicator color={theme.colors.primary} />
        </View>
      </Screen>
    );
  }

  if (senior.isError) {
    // A 404 here is what a revoked caregiver sees. The wording says the profile
    // is unavailable rather than that it does not exist, because the two are
    // deliberately indistinguishable to the API and guessing would be worse
    // than saying less (docs/02-permissions-and-authorization.md).
    const notFound = senior.error instanceof ApiError && senior.error.status === 404;

    return (
      <Screen scrollable>
        <Stack.Screen options={{ headerShown: true, title: '' }} />
        <Card>
          <Text variant="sectionHeading">
            {notFound ? 'This profile is not available' : 'We could not load this profile'}
          </Text>
          <Text variant="body" color="secondary">
            {notFound
              ? 'You may no longer be part of this care circle.'
              : 'Something went wrong. Please try again.'}
          </Text>
          <Button
            variant="secondary"
            label={notFound ? 'Back to Today' : 'Try again'}
            onPress={() => (notFound ? router.replace('/home') : void senior.refetch())}
          />
        </Card>
      </Screen>
    );
  }

  return <Dashboard profile={senior.data} />;
}

function Dashboard({ profile }: { profile: Senior }) {
  const theme = useTheme();
  const removeSenior = useRemoveSenior(profile.id);

  function confirmRemoveProfile() {
    confirmAction({
      title: `Remove ${profile.displayName}'s profile?`,
      message:
        'If no care has been recorded, this permanently deletes the mistaken profile. Otherwise it archives the profile and removes it from everyone’s list while preserving care history.',
      confirmLabel: 'Remove profile',
      onConfirm: () =>
        removeSenior.mutate(undefined, {
          onSuccess: (result) => {
            showMessage({
              title: result.disposition === 'deleted' ? 'Profile deleted' : 'Profile archived',
              message:
                result.disposition === 'deleted'
                  ? 'The empty profile was permanently deleted.'
                  : 'The profile was removed from care lists and its history was preserved.',
              onDismiss: () => router.replace('/home'),
            });
          },
        }),
    });
  }

  // Everything on this screen is today's, in the senior's own timezone. The
  // queries are declared unconditionally and enabled by permission, because a
  // hook cannot be called conditionally and a disabled query costs nothing.
  const doses = useMedicationDoses(can(profile, 'medications.view') ? profile.id : null, 'today');
  const tasks = useSeniorTasks(can(profile, 'tasks.view') ? profile.id : null, 'today');
  const appointments = useAppointments(
    can(profile, 'appointments.view') ? profile.id : null,
    'today',
  );
  const activity = useActivity(can(profile, 'activity.view') ? profile.id : null);

  // Defensive on `items`: this is the screen a family opens every day, and a
  // page shape it did not expect must not be the thing that takes it down.
  const recentActivity = activity.data?.pages[0]?.items?.slice(0, 3) ?? [];

  // Activity reads as "Sara marked Morning walk as done", so the timeline needs
  // names for the ids it carries. The care-circle query is shared with the
  // circle and activity screens through TanStack's cache, so asking for it here
  // costs a request once rather than once per screen (plans/phase9.md §33).
  const members = useCircleMembers(
    can(profile, 'activity.view') && can(profile, 'members.view') ? profile.id : null,
  );

  const actorNames = useMemo(() => {
    const byUserId = new Map<string, string>();
    for (const member of members.data ?? []) byUserId.set(member.userId, member.displayName);
    return byUserId;
  }, [members.data]);

  // "Somebody" covers an event with no human actor, one whose actor has since
  // left the circle, and a reader who may not see the member list at all.
  const actorName = (actorUserId: string | null) =>
    (actorUserId === null ? undefined : actorNames.get(actorUserId)) ?? 'Somebody';

  // Whether this circle has anything in it yet decides between guidance and a
  // summary. A new profile with three empty sections reads as a broken app
  // (plans/phase9.md §7).
  //
  // A query the reader has no permission for is disabled, and a disabled query
  // stays pending for ever — so "have we heard back?" has to mean "from the
  // ones we actually asked", or a caregiver who may see only appointments would
  // never be shown anything.
  const answered = (permitted: boolean, query: { isPending: boolean }) =>
    !permitted || !query.isPending;

  const nothingYet =
    answered(can(profile, 'medications.view'), doses) &&
    answered(can(profile, 'tasks.view'), tasks) &&
    answered(can(profile, 'appointments.view'), appointments) &&
    (doses.data ?? []).length === 0 &&
    (tasks.data ?? []).length === 0 &&
    (appointments.data ?? []).length === 0;

  return (
    <Screen scrollable>
      <Stack.Screen options={{ headerShown: true, title: profile.displayName }} />

      <View style={{ gap: theme.spacing.sm }}>
        <Text variant="pageHeading">{profile.displayName}</Text>
        <Text variant="body" color="secondary">
          {profile.isSelf ? 'Your own care' : 'Care circle'}
        </Text>
      </View>

      {nothingYet ? <GettingStarted profile={profile} /> : null}

      {can(profile, 'medications.view') ? (
        <Section
          title="Medication today"
          query={doses}
          emptyTitle="No medication today"
          emptyBody={
            can(profile, 'medications.manage')
              ? 'Add a medicine and its times, and GenXcare will show each dose here and remind you before it is due.'
              : 'Nothing is scheduled for today.'
          }
          action={{
            label: 'View medication',
            href: { pathname: '/seniors/[seniorId]/medications', params: { seniorId: profile.id } },
          }}
          render={(items: MedicationDose[]) =>
            items.slice(0, 3).map((dose) => (
              <SummaryRow
                key={dose.id}
                title={dose.name}
                detail={`${doseTimeLabel(dose, profile.timezone)} · ${doseStatusLabel(dose.status)}`}
                href={{
                  pathname: '/medications/[medicationId]',
                  params: { medicationId: dose.medicationId },
                }}
              />
            ))
          }
        />
      ) : null}

      {can(profile, 'tasks.view') ? (
        <Section
          title="Care tasks today"
          query={tasks}
          emptyTitle="Nothing to do today"
          emptyBody={
            can(profile, 'tasks.manage')
              ? 'Add the everyday things — a walk, a meal, a phone call — and they will appear here each day they are due.'
              : 'Nothing is scheduled for today.'
          }
          action={{
            label: 'View care tasks',
            href: { pathname: '/seniors/[seniorId]/tasks', params: { seniorId: profile.id } },
          }}
          render={(items: CareTask[]) =>
            items
              .slice(0, 3)
              .map((task) => (
                <SummaryRow
                  key={task.id}
                  title={task.title}
                  detail={`${taskTimeLabel(task, profile.timezone)} · ${statusLabel(task.status)}`}
                  href={{ pathname: '/tasks/[taskId]', params: { taskId: task.id } }}
                />
              ))
          }
        />
      ) : null}

      {can(profile, 'appointments.view') ? (
        <Section
          title="Appointments today"
          query={appointments}
          emptyTitle="No appointments today"
          emptyBody={
            can(profile, 'appointments.manage')
              ? 'Add a visit and GenXcare will remind the circle an hour before it starts.'
              : 'Nothing is booked for today.'
          }
          action={{
            label: 'View appointments',
            href: {
              pathname: '/seniors/[seniorId]/appointments',
              params: { seniorId: profile.id },
            },
          }}
          render={(items: Appointment[]) =>
            items.slice(0, 3).map((appointment) => (
              <SummaryRow
                key={appointment.id}
                title={appointment.title}
                detail={`${appointmentWhenLabel(appointment, profile.timezone)} · ${appointmentStatusLabel(
                  appointment.status,
                )}`}
                href={{
                  pathname: '/appointments/[appointmentId]',
                  params: { appointmentId: appointment.id },
                }}
              />
            ))
          }
        />
      ) : null}

      {can(profile, 'activity.view') ? (
        <Card>
          <Text variant="sectionHeading">Recent activity</Text>

          {activity.isPending ? (
            <SectionLoading label="Loading activity…" />
          ) : activity.isError ? (
            <SectionError onRetry={() => void activity.refetch()} />
          ) : recentActivity.length === 0 ? (
            <Text variant="body" color="secondary">
              Nothing has happened yet. As the circle records care, it will appear here.
            </Text>
          ) : (
            <View style={{ gap: theme.spacing.md }}>
              {recentActivity.map((event) => (
                <View key={event.id} style={{ gap: 2 }}>
                  <Text variant="body">
                    {careEventDescription(event, actorName(event.actorUserId))}
                  </Text>
                  <Text variant="secondary" color="secondary">
                    {careEventTimeLabel(event, profile.timezone)}
                  </Text>
                </View>
              ))}
            </View>
          )}

          <Button
            variant="secondary"
            label="View activity"
            onPress={() =>
              router.push({
                pathname: '/seniors/[seniorId]/activity',
                params: { seniorId: profile.id },
              })
            }
          />
        </Card>
      ) : null}

      {can(profile, 'notes.view') ? (
        <Card>
          <Text variant="sectionHeading">Care notes</Text>
          <Text variant="body" color="secondary">
            Observations shared by the people caring for {profile.displayName}.
          </Text>
          <Button
            variant="secondary"
            label="View care notes"
            onPress={() =>
              router.push({
                pathname: '/seniors/[seniorId]/notes',
                params: { seniorId: profile.id },
              })
            }
          />
        </Card>
      ) : null}

      {can(profile, 'messages.participate') ? (
        <Card>
          <Text variant="sectionHeading">Messages</Text>
          <Text variant="body" color="secondary">
            Coordinate privately with this care circle.
          </Text>
          <Button
            variant="secondary"
            label="Open messages"
            onPress={() =>
              router.push({
                pathname: '/seniors/[seniorId]/messages',
                params: { seniorId: profile.id },
              })
            }
          />
        </Card>
      ) : null}

      {can(profile, 'members.view') ? (
        <Card>
          <Text variant="sectionHeading">Care circle</Text>
          <Text variant="body" color="secondary">
            The family and caregivers involved in this person&apos;s care.
          </Text>
          <Button
            variant="secondary"
            label="View care circle"
            onPress={() =>
              router.push({
                pathname: '/seniors/[seniorId]/circle',
                params: { seniorId: profile.id },
              })
            }
          />
        </Card>
      ) : null}

      <Card>
        <Text variant="sectionHeading">Profile</Text>
        <DetailRow label="Date of birth" value={profile.dateOfBirth} />
        <DetailRow label="Phone" value={profile.phone} />
        <DetailRow label="Address" value={profile.address} />
        <DetailRow label="Emergency contact" value={profile.emergencyContact} />

        {/* The edit action appears only when the API would allow it. Hiding it
            is a courtesy, not the control: the server refuses regardless
            (plans/phase9.md §13). */}
        {can(profile, 'senior.edit') ? (
          <Button
            variant="secondary"
            label="Edit profile"
            onPress={() =>
              router.push({
                pathname: '/seniors/[seniorId]/edit',
                params: { seniorId: profile.id },
              })
            }
          />
        ) : null}

        {!profile.isSelf && can(profile, 'members.manage') ? (
          <>
            {removeSenior.isError ? (
              <Text variant="secondary" color="danger">
                {removeSenior.error instanceof ApiError
                  ? removeSenior.error.message
                  : 'This profile could not be removed.'}
              </Text>
            ) : null}
            <Button
              variant="danger"
              label="Remove this profile"
              loading={removeSenior.isPending}
              onPress={confirmRemoveProfile}
            />
          </>
        ) : null}
      </Card>
    </Screen>
  );
}

/** The minimum a section needs from a TanStack query. */
interface SectionQuery<T> {
  isPending: boolean;
  isError: boolean;
  data: T[] | undefined;
  refetch: () => unknown;
}

/**
 * One summary section: heading, then whichever of loading, error, empty, or
 * content applies, then a way in.
 *
 * Shared so that all four states are handled the same way on every section —
 * a blank space where a failed request should be is the bug this shape exists
 * to prevent (plans/phase9.md §§10, 11, 12).
 */
function Section<T>({
  title,
  query,
  emptyTitle,
  emptyBody,
  action,
  render,
}: {
  title: string;
  query: SectionQuery<T>;
  emptyTitle: string;
  emptyBody: string;
  action: { label: string; href: Href };
  render: (items: T[]) => ReactNode;
}) {
  const theme = useTheme();
  const items = query.data ?? [];

  return (
    <Card>
      <Text variant="sectionHeading">{title}</Text>

      {query.isPending ? (
        <SectionLoading label={`Loading ${title.toLowerCase()}…`} />
      ) : query.isError ? (
        <SectionError onRetry={() => void query.refetch()} />
      ) : items.length === 0 ? (
        <View style={{ gap: theme.spacing.xs }}>
          <Text variant="bodyStrong">{emptyTitle}</Text>
          <Text variant="body" color="secondary">
            {emptyBody}
          </Text>
        </View>
      ) : (
        <View style={{ gap: theme.spacing.md }}>{render(items)}</View>
      )}

      <Button variant="secondary" label={action.label} onPress={() => router.push(action.href)} />
    </Card>
  );
}

function SectionLoading({ label }: { label: string }) {
  const theme = useTheme();

  return (
    <View style={[styles.loading, { gap: theme.spacing.md, paddingVertical: theme.spacing.md }]}>
      <ActivityIndicator color={theme.colors.primary} />
      <Text variant="secondary" color="secondary">
        {label}
      </Text>
    </View>
  );
}

/**
 * A section that failed.
 *
 * Never the underlying message: a care app must not put a database error in
 * front of somebody checking on their mother (plans/phase9.md §12).
 */
function SectionError({ onRetry }: { onRetry: () => void }) {
  const theme = useTheme();

  return (
    <View style={{ gap: theme.spacing.sm }}>
      <Text variant="body" color="secondary">
        We could not load this just now.
      </Text>
      <Button variant="secondary" label="Try again" onPress={onRetry} />
    </View>
  );
}

/** One line in a summary: what it is, when, and how it stands. */
function SummaryRow({ title, detail, href }: { title: string; detail: string; href: Href }) {
  const theme = useTheme();

  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={`${title}. ${detail}`}
      onPress={() => router.push(href)}
      style={({ pressed }) => [
        styles.summaryRow,
        { gap: theme.spacing.md, minHeight: theme.minTouchTarget, opacity: pressed ? 0.85 : 1 },
      ]}
    >
      <View style={{ flex: 1, gap: 2 }}>
        <Text variant="body">{title}</Text>
        {/* Status is written out, never carried by colour alone
            (docs/18, plans/phase9.md §35). */}
        <Text variant="secondary" color="secondary">
          {detail}
        </Text>
      </View>
      <Text variant="body" color="secondary">
        ›
      </Text>
    </Pressable>
  );
}

/**
 * What to do with a care circle that is empty.
 *
 * Only the steps this reader is allowed to take, so a caregiver is never
 * invited to add a medicine they cannot add (plans/phase9.md §§7, 13).
 */
function GettingStarted({ profile }: { profile: Senior }) {
  const theme = useTheme();

  const steps: { label: string; href: Href }[] = [];

  if (can(profile, 'medications.manage')) {
    steps.push({
      label: 'Add a medication',
      href: {
        pathname: '/seniors/[seniorId]/medications/new',
        params: { seniorId: profile.id },
      },
    });
  }
  if (can(profile, 'tasks.manage')) {
    steps.push({
      label: 'Add a care task',
      href: { pathname: '/seniors/[seniorId]/tasks/new', params: { seniorId: profile.id } },
    });
  }
  if (can(profile, 'appointments.manage')) {
    steps.push({
      label: 'Add an appointment',
      href: {
        pathname: '/seniors/[seniorId]/appointments/new',
        params: { seniorId: profile.id },
      },
    });
  }
  if (can(profile, 'members.invite')) {
    steps.push({
      label: 'Invite someone to help',
      href: { pathname: '/seniors/[seniorId]/invite', params: { seniorId: profile.id } },
    });
  }

  if (steps.length === 0) return null;

  return (
    <Card>
      <Text variant="sectionHeading">Getting started</Text>
      <Text variant="body" color="secondary">
        {profile.isSelf
          ? 'Nothing is set up yet. Add whatever you want GenXcare to keep track of — you can do the rest later.'
          : `Nothing is set up for ${profile.displayName} yet. Start with whichever of these matters most today.`}
      </Text>

      <View style={{ gap: theme.spacing.sm }}>
        {steps.map((step, index) => (
          <Button
            key={step.label}
            // The first suggestion is the emphasised one; a column of identical
            // primary buttons gives no help at all about where to begin.
            variant={index === 0 ? 'primary' : 'secondary'}
            label={step.label}
            onPress={() => router.push(step.href)}
          />
        ))}
      </View>
    </Card>
  );
}

function DetailRow({ label, value }: { label: string; value: string | null }) {
  const theme = useTheme();

  return (
    <View style={{ gap: theme.spacing.xs }}>
      <Text variant="secondary" color="secondary">
        {label}
      </Text>
      <Text variant="body">{value ?? 'Not added yet'}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  centred: { alignItems: 'center', flex: 1, justifyContent: 'center' },
  loading: { alignItems: 'center', flexDirection: 'row' },
  summaryRow: { alignItems: 'center', flexDirection: 'row' },
});
