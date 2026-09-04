import type { Appointment, CareTask, MedicationDose } from '@genxcare/contracts';
import * as SQLite from 'expo-sqlite';

import type { SyncEntityType, SyncOperation, SyncStatus, SyncStore } from './sync-queue';

/**
 * The durable local database.
 *
 * docs/07 is explicit that this mirrors only what the app needs offline, not
 * every server table: today's tasks, and the mutations waiting to be sent.
 * Everything else is fetched when there is a connection.
 */

const DATABASE_NAME = 'genxcare.db';

let handle: SQLite.SQLiteDatabase | null = null;

/** Opens the database, creating the schema on first use. */
export async function openDatabase(): Promise<SQLite.SQLiteDatabase> {
  if (handle !== null) return handle;

  const database = await SQLite.openDatabaseAsync(DATABASE_NAME);

  await database.execAsync(`
    PRAGMA journal_mode = WAL;

    CREATE TABLE IF NOT EXISTS cached_tasks (
      id            TEXT PRIMARY KEY NOT NULL,
      senior_id     TEXT NOT NULL,
      scheduled_for TEXT NOT NULL,
      payload       TEXT NOT NULL,
      cached_at     TEXT NOT NULL
    );

    CREATE INDEX IF NOT EXISTS cached_tasks_senior_idx
      ON cached_tasks (senior_id, scheduled_for);

    CREATE TABLE IF NOT EXISTS cached_medication_doses (
      id            TEXT PRIMARY KEY NOT NULL,
      senior_id     TEXT NOT NULL,
      scheduled_for TEXT NOT NULL,
      payload       TEXT NOT NULL,
      cached_at     TEXT NOT NULL
    );

    CREATE INDEX IF NOT EXISTS cached_medication_doses_senior_idx
      ON cached_medication_doses (senior_id, scheduled_for);

    CREATE TABLE IF NOT EXISTS cached_appointments (
      id           TEXT PRIMARY KEY NOT NULL,
      senior_id    TEXT NOT NULL,
      scheduled_at TEXT NOT NULL,
      payload      TEXT NOT NULL,
      cached_at    TEXT NOT NULL
    );

    CREATE INDEX IF NOT EXISTS cached_appointments_senior_idx
      ON cached_appointments (senior_id, scheduled_at);

    CREATE TABLE IF NOT EXISTS sync_queue (
      operation_id   TEXT PRIMARY KEY NOT NULL,
      entity_type    TEXT NOT NULL,
      entity_id      TEXT NOT NULL,
      operation_type TEXT NOT NULL,
      payload        TEXT,
      created_at     TEXT NOT NULL,
      retry_count    INTEGER NOT NULL DEFAULT 0,
      last_error     TEXT,
      status         TEXT NOT NULL DEFAULT 'pending'
    );

    CREATE INDEX IF NOT EXISTS sync_queue_pending_idx
      ON sync_queue (status, created_at);
  `);

  handle = database;
  return database;
}

/** Closes the database. Used by tests and on sign-out. */
export async function closeDatabase(): Promise<void> {
  if (handle === null) return;
  await handle.closeAsync();
  handle = null;
}

/** The shape rows are stored in. */
interface QueueRow {
  operation_id: string;
  entity_type: string;
  entity_id: string;
  operation_type: string;
  payload: string | null;
  created_at: string;
  retry_count: number;
  last_error: string | null;
  status: string;
}

function toOperation(row: QueueRow): SyncOperation {
  return {
    operationId: row.operation_id,
    entityType: row.entity_type as SyncEntityType,
    entityId: row.entity_id,
    operationType: row.operation_type as SyncOperation['operationType'],
    payload: row.payload,
    createdAt: row.created_at,
    retryCount: row.retry_count,
    lastError: row.last_error,
    status: row.status as SyncStatus,
  };
}

/** The queue backed by SQLite. */
export const sqliteSyncStore: SyncStore = {
  async enqueue(operation) {
    const database = await openDatabase();

    // Replaces an operation with the same id, so a retry of the same user
    // action never queues twice.
    await database.runAsync(
      `INSERT OR REPLACE INTO sync_queue (
         operation_id, entity_type, entity_id, operation_type,
         payload, created_at, retry_count, last_error, status
       ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
      operation.operationId,
      operation.entityType,
      operation.entityId,
      operation.operationType,
      operation.payload,
      operation.createdAt,
      operation.retryCount,
      operation.lastError,
      operation.status,
    );
  },

  async pending() {
    const database = await openDatabase();
    const rows = await database.getAllAsync<QueueRow>(
      `SELECT * FROM sync_queue WHERE status = 'pending' ORDER BY created_at, rowid`,
    );
    return rows.map(toOperation);
  },

  async remove(operationId) {
    const database = await openDatabase();
    await database.runAsync(`DELETE FROM sync_queue WHERE operation_id = ?`, operationId);
  },

  async markFailed(operationId, error, permanent) {
    const database = await openDatabase();
    await database.runAsync(
      `UPDATE sync_queue
       SET retry_count = retry_count + 1,
           last_error  = ?,
           status      = ?
       WHERE operation_id = ?`,
      error,
      permanent ? 'failed' : 'pending',
      operationId,
    );
  },

  async forEntity(entityType, entityId) {
    const database = await openDatabase();
    const rows = await database.getAllAsync<QueueRow>(
      `SELECT * FROM sync_queue
       WHERE entity_type = ? AND entity_id = ?
       ORDER BY created_at, rowid`,
      entityType,
      entityId,
    );
    return rows.map(toOperation);
  },
};

/**
 * Caches the tasks a senior has in a window, so the list is readable with no
 * connection.
 *
 * The window is replaced rather than merged: the server is authoritative, and
 * a task that has disappeared from its answer should disappear here too.
 */
export async function cacheTasks(seniorId: string, tasks: CareTask[]): Promise<void> {
  const database = await openDatabase();
  const cachedAt = new Date().toISOString();

  await database.withTransactionAsync(async () => {
    await database.runAsync(`DELETE FROM cached_tasks WHERE senior_id = ?`, seniorId);

    for (const task of tasks) {
      await database.runAsync(
        `INSERT OR REPLACE INTO cached_tasks (id, senior_id, scheduled_for, payload, cached_at)
         VALUES (?, ?, ?, ?, ?)`,
        task.id,
        seniorId,
        task.scheduledFor,
        JSON.stringify(task),
        cachedAt,
      );
    }
  });
}

/** Reads the cached tasks for a senior, oldest first. */
export async function cachedTasks(seniorId: string): Promise<CareTask[]> {
  const database = await openDatabase();
  const rows = await database.getAllAsync<{ payload: string }>(
    `SELECT payload FROM cached_tasks WHERE senior_id = ? ORDER BY scheduled_for`,
    seniorId,
  );

  return rows.map((row) => JSON.parse(row.payload) as CareTask);
}

/**
 * Caches today's medication for a senior, so the list is readable with no
 * connection.
 *
 * docs/07 names "current medication schedules/instances" among the few things
 * worth keeping locally, and it is the list somebody needs while standing in
 * front of the person they care for (plans/phase5.md §20).
 */
export async function cacheDoses(seniorId: string, doses: MedicationDose[]): Promise<void> {
  const database = await openDatabase();
  const cachedAt = new Date().toISOString();

  await database.withTransactionAsync(async () => {
    await database.runAsync(`DELETE FROM cached_medication_doses WHERE senior_id = ?`, seniorId);

    for (const dose of doses) {
      await database.runAsync(
        `INSERT OR REPLACE INTO cached_medication_doses
           (id, senior_id, scheduled_for, payload, cached_at)
         VALUES (?, ?, ?, ?, ?)`,
        dose.id,
        seniorId,
        dose.scheduledFor,
        JSON.stringify(dose),
        cachedAt,
      );
    }
  });
}

/** Reads the cached doses for a senior, earliest first. */
export async function cachedDoses(seniorId: string): Promise<MedicationDose[]> {
  const database = await openDatabase();
  const rows = await database.getAllAsync<{ payload: string }>(
    `SELECT payload FROM cached_medication_doses WHERE senior_id = ? ORDER BY scheduled_for`,
    seniorId,
  );

  return rows.map((row) => JSON.parse(row.payload) as MedicationDose);
}

/**
 * Caches a senior's upcoming appointments, so the list is readable with no
 * connection.
 *
 * The one appointment view worth keeping locally: somebody in a car on the way
 * to a hospital needs the address, and that is exactly where the signal goes.
 * Nothing here is ever written back — appointments are read offline and only
 * changed online (plans/phase6.md §23).
 */
export async function cacheAppointments(
  seniorId: string,
  appointments: Appointment[],
): Promise<void> {
  const database = await openDatabase();
  const cachedAt = new Date().toISOString();

  await database.withTransactionAsync(async () => {
    await database.runAsync(`DELETE FROM cached_appointments WHERE senior_id = ?`, seniorId);

    for (const appointment of appointments) {
      await database.runAsync(
        `INSERT OR REPLACE INTO cached_appointments
           (id, senior_id, scheduled_at, payload, cached_at)
         VALUES (?, ?, ?, ?, ?)`,
        appointment.id,
        seniorId,
        appointment.scheduledAt,
        JSON.stringify(appointment),
        cachedAt,
      );
    }
  });
}

/** Reads the cached appointments for a senior, soonest first. */
export async function cachedAppointments(seniorId: string): Promise<Appointment[]> {
  const database = await openDatabase();
  const rows = await database.getAllAsync<{ payload: string }>(
    `SELECT payload FROM cached_appointments WHERE senior_id = ? ORDER BY scheduled_at`,
    seniorId,
  );

  return rows.map((row) => JSON.parse(row.payload) as Appointment);
}

/** Clears everything local. Used on sign-out, so care data does not outlive the session. */
export async function clearOfflineData(): Promise<void> {
  const database = await openDatabase();
  await database.execAsync(
    `DELETE FROM cached_tasks;
     DELETE FROM cached_medication_doses;
     DELETE FROM cached_appointments;
     DELETE FROM sync_queue;`,
  );
}
