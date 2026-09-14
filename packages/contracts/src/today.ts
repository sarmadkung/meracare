/** Which domain a row on Today came from. */
export type TodayItemKind = 'task' | 'dose' | 'appointment';

/**
 * One thing happening today, in one senior's care.
 *
 * Today spans every circle the reader belongs to, so each row names its senior
 * and carries that senior's timezone: "Metformin at 2pm" means nothing without
 * knowing whose 2pm it is, and a daughter in London must see the time her
 * mother in Karachi will experience.
 */
export interface TodayItem {
  kind: TodayItemKind;
  id: string;
  seniorId: string;
  seniorName: string;
  /** IANA zone of the senior this belongs to, not the reader's device. */
  timezone: string;
  title: string;
  /** Dosage, provider name, or empty. */
  detail: string;
  scheduledFor: string;
  status: string;
  /** True only for work explicitly given to this reader. */
  assignedToMe: boolean;
}

/** Response shape of `GET /v1/today`. */
export interface TodayListResponse {
  items: TodayItem[];
}
