import type { CursorPage } from './pagination';

/** One message in the senior's care-circle stream. */
export interface CareMessage {
  id: string;
  seniorId: string;
  senderUserId: string;
  senderName: string;
  content: string;
  createdAt: string;
  sentByMe: boolean;
}

export interface MessagePage extends CursorPage<CareMessage> {
  unreadCount: number;
}

export interface CreateMessageRequest {
  content: string;
}

export interface MarkMessagesReadRequest {
  throughMessageId: string;
}
