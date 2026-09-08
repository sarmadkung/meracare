package notes

import "time"

type Response struct {
	ID           string `json:"id"`
	SeniorID     string `json:"seniorId"`
	AuthorUserID string `json:"authorUserId"`
	AuthorName   string `json:"authorName"`
	Content      string `json:"content"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
	CanEdit      bool   `json:"canEdit"`
}

func ToResponse(note Note, callerID string) Response {
	return Response{
		ID: note.ID.String(), SeniorID: note.SeniorID.String(),
		AuthorUserID: note.AuthorUserID.String(), AuthorName: note.AuthorName,
		Content: note.Content, CreatedAt: note.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt: note.UpdatedAt.UTC().Format(time.RFC3339Nano),
		CanEdit:   note.AuthorUserID.String() == callerID,
	}
}
