package messages

import "time"

type Response struct {
	ID           string `json:"id"`
	SeniorID     string `json:"seniorId"`
	SenderUserID string `json:"senderUserId"`
	SenderName   string `json:"senderName"`
	Content      string `json:"content"`
	CreatedAt    string `json:"createdAt"`
	SentByMe     bool   `json:"sentByMe"`
}

func ToResponse(message Message, callerID string) Response {
	return Response{
		ID: message.ID.String(), SeniorID: message.SeniorID.String(),
		SenderUserID: message.SenderUserID.String(), SenderName: message.SenderName,
		Content: message.Content, CreatedAt: message.CreatedAt.UTC().Format(time.RFC3339Nano),
		SentByMe: message.SenderUserID.String() == callerID,
	}
}
