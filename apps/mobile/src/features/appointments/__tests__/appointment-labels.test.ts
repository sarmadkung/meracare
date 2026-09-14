import type { Appointment, AppointmentStatus } from '@genxcare/contracts';
import {
  APPOINTMENT_STATUSES,
  appointmentDateLabel,
  appointmentKindLabel,
  appointmentPlaceLabel,
  appointmentStatusLabel,
  appointmentStatusTone,
  appointmentTimeLabel,
  appointmentWhenLabel,
  isAppointmentOpen,
  isAppointmentUpcoming,
  SELECTABLE_APPOINTMENT_KINDS,
} from '@genxcare/contracts';

/**
 * The wording an older adult reads. Nothing internal may reach a screen, and
 * status must be legible without colour (plans/phase6.md §§7, 31).
 */

function appointment(overrides: Partial<Appointment> = {}): Appointment {
  return {
    id: 'appointment-1',
    seniorId: 'senior-1',
    title: 'Cardiology review',
    kind: 'doctor_visit',
    providerName: 'Dr Ahmed',
    location: 'City Hospital',
    notes: null,
    assignedUserId: null,
    // 09:30 in Karachi, which is 04:30 UTC.
    scheduledAt: '2026-08-20T04:30:00Z',
    endsAt: null,
    status: 'scheduled',
    completedAt: null,
    completedBy: null,
    cancelledAt: null,
    cancelledBy: null,
    createdBy: 'user-1',
    createdAt: '2026-08-17T09:00:00Z',
    updatedAt: '2026-08-17T09:00:00Z',
    ...overrides,
  };
}

describe('status wording', () => {
  it('says what a status means in words, never a database value', () => {
    expect(appointmentStatusLabel('scheduled')).toBe('Booked');
    expect(appointmentStatusLabel('completed')).toBe('Went');
    expect(appointmentStatusLabel('cancelled')).toBe('Cancelled');
  });

  it('has a label and a tone for every status the API can send', () => {
    for (const status of APPOINTMENT_STATUSES) {
      expect(appointmentStatusLabel(status as AppointmentStatus)).toBeTruthy();
      expect(appointmentStatusTone(status as AppointmentStatus)).toBeTruthy();
    }
  });

  // A settled appointment is distinguished by its words as well as its tone, so
  // the two carry independent information.
  it('gives settled appointments a tone that differs from a booked one', () => {
    expect(appointmentStatusTone('scheduled')).toBe('neutral');
    expect(appointmentStatusTone('completed')).toBe('positive');
    expect(appointmentStatusTone('cancelled')).toBe('muted');
  });
});

describe('kind wording', () => {
  it('reads as something a person would say', () => {
    expect(appointmentKindLabel('doctor_visit')).toBe("Doctor's visit");
    expect(appointmentKindLabel('laboratory')).toBe('Blood test or scan');
  });

  it('offers every kind the API accepts', () => {
    expect(SELECTABLE_APPOINTMENT_KINDS).toHaveLength(6);
    for (const option of SELECTABLE_APPOINTMENT_KINDS) {
      expect(option.label).not.toContain('_');
    }
  });
});

describe('time wording', () => {
  // The point of the whole timezone apparatus: a daughter in London reading her
  // mother's calendar must see her mother's hour.
  it('reads an instant in the senior‘s timezone, not the device‘s', () => {
    expect(appointmentTimeLabel(appointment(), 'Asia/Karachi')).toBe('09:30');
    expect(appointmentTimeLabel(appointment(), 'Europe/London')).toBe('05:30');
  });

  it('shows the date in the senior‘s timezone', () => {
    expect(appointmentDateLabel(appointment(), 'Asia/Karachi')).toContain('20 August');
  });

  it('shows an end time only when somebody recorded one', () => {
    expect(appointmentWhenLabel(appointment(), 'Asia/Karachi')).toBe('09:30');
    expect(
      appointmentWhenLabel(appointment({ endsAt: '2026-08-20T05:15:00Z' }), 'Asia/Karachi'),
    ).toBe('09:30 to 10:15');
  });

  // A visit that runs past midnight is two instants, so it needs no special
  // case — but the wording has to survive it.
  it('handles an appointment that runs past midnight', () => {
    const overnight = appointment({
      scheduledAt: '2026-08-20T18:30:00Z', // 23:30 in Karachi
      endsAt: '2026-08-20T19:30:00Z', // 00:30 the next day
    });
    expect(appointmentWhenLabel(overnight, 'Asia/Karachi')).toBe('23:30 to 00:30');
  });
});

describe('place wording', () => {
  it('joins the provider and the place', () => {
    expect(appointmentPlaceLabel(appointment())).toBe('Dr Ahmed · City Hospital');
  });

  it('copes when either half is missing, as it is for a family meeting', () => {
    expect(appointmentPlaceLabel(appointment({ location: null }))).toBe('Dr Ahmed');
    expect(appointmentPlaceLabel(appointment({ providerName: null }))).toBe('City Hospital');
    expect(appointmentPlaceLabel(appointment({ providerName: null, location: null }))).toBe('');
  });
});

describe('reading the clock', () => {
  // Upcoming is not a stored status: the same row is upcoming this morning and
  // past this evening without anything being written.
  it('decides upcoming from the clock', () => {
    const at = appointment();
    expect(isAppointmentUpcoming(at, new Date('2026-08-20T04:00:00Z'))).toBe(true);
    expect(isAppointmentUpcoming(at, new Date('2026-08-20T05:00:00Z'))).toBe(false);
  });

  it('reports only a booked appointment as still changeable', () => {
    expect(isAppointmentOpen(appointment())).toBe(true);
    expect(isAppointmentOpen(appointment({ status: 'completed' }))).toBe(false);
    expect(isAppointmentOpen(appointment({ status: 'cancelled' }))).toBe(false);
  });
});
