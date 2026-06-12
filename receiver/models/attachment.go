package models

type Attachment struct {
	ID          int
	MessageID   int
	FileName    string
	ContentType string
	Path        string
}
