package models

import "time"

type Message struct {
	ID         int
	InboxID    int
	Sender     string
	Subject    string
	TextBody   string
	HtmlBody   string
	ReceivedAt time.Time
}
