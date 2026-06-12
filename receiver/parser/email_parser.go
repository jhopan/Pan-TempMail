package parser

type ParsedEmail struct {
	Subject     string
	TextBody    string
	HtmlBody    string
	Attachments []ParsedAttachment
}

type ParsedAttachment struct {
	FileName    string
	ContentType string
	Data        []byte
}
