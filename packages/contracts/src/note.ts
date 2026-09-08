/** A senior-scoped care note. */
export interface CareNote {
  id: string;
  seniorId: string;
  authorUserId: string;
  authorName: string;
  content: string;
  createdAt: string;
  updatedAt: string;
  canEdit: boolean;
}

export interface CareNoteListResponse {
  items: CareNote[];
}

export interface CreateCareNoteRequest {
  content: string;
}

export interface UpdateCareNoteRequest {
  content: string;
}
