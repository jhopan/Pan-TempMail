package models

import "time"

type Inbox struct {
	ID        int
	Email     string
	Type      string
	ExpiresAt time.Time
}
