package parser

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/quotedprintable"
	"strings"

	"github.com/emersion/go-message"
	"github.com/emersion/go-message/mail"
	"golang.org/x/net/html/charset"
)

func ParseEmail(r io.Reader) (*ParsedEmail, error) {
	entity, err := message.Read(r)
	if err != nil {
		return nil, err
	}
	result := &ParsedEmail{Attachments: []ParsedAttachment{}}
	result.Subject = sanitizeUTF8(decodeHeader(entity.Header.Get("Subject")))
	mediaType, params, _ := entity.Header.ContentType()
	if !strings.HasPrefix(mediaType, "multipart/") {
		body, _ := io.ReadAll(entity.Body)
		decoded := decodeBody(body, entity.Header.Get("Content-Transfer-Encoding"))
		decoded = convertCharset(decoded, params["charset"])
		decoded = sanitizeUTF8(decoded)
		if mediaType == "text/html" {
			result.HtmlBody = decoded
		} else {
			result.TextBody = decoded
		}
		return result, nil
	}
	mr := mail.NewReader(entity)
	parseParts(mr, result)
	return result, nil
}

func parseParts(mr *mail.Reader, result *ParsedEmail) {
	for {
		part, err := mr.NextPart()
		if err != nil {
			break
		}
		switch h := part.Header.(type) {
		case *mail.InlineHeader:
			contentType, params, _ := h.ContentType()
			body, _ := io.ReadAll(part.Body)
			decoded := decodeBody(body, h.Get("Content-Transfer-Encoding"))
			decoded = convertCharset(decoded, params["charset"])
			decoded = sanitizeUTF8(decoded)
			if contentType == "text/plain" && result.TextBody == "" {
				result.TextBody = decoded
			}
			if contentType == "text/html" && result.HtmlBody == "" {
				result.HtmlBody = decoded
			}
		case *mail.AttachmentHeader:
			filename, _ := h.Filename()
			contentType, _, _ := h.ContentType()
			body, _ := io.ReadAll(part.Body)
			decoded := decodeBodyBytes(body, h.Get("Content-Transfer-Encoding"))
			result.Attachments = append(result.Attachments, ParsedAttachment{
				FileName: filename, ContentType: contentType, Data: decoded,
			})
		}
	}
}

func decodeHeader(s string) string {
	dec := new(mime.WordDecoder)
	decoded, err := dec.DecodeHeader(s)
	if err != nil {
		return s
	}
	return decoded
}

func decodeBody(data []byte, encoding string) string {
	return string(decodeBodyBytes(data, encoding))
}

func decodeBodyBytes(data []byte, encoding string) []byte {
	encoding = strings.ToLower(strings.TrimSpace(encoding))
	switch encoding {
	case "base64":
		decoded := make([]byte, base64.StdEncoding.DecodedLen(len(data)))
		n, err := base64.StdEncoding.Decode(decoded, data)
		if err != nil {
			return data
		}
		return decoded[:n]
	case "quoted-printable":
		qpr := quotedprintable.NewReader(bytes.NewReader(data))
		decoded, err := io.ReadAll(qpr)
		if err != nil {
			return data
		}
		return decoded
	default:
		return data
	}
}

func convertCharset(s string, cs string) string {
	if cs == "" || strings.EqualFold(cs, "utf-8") || strings.EqualFold(cs, "utf8") {
		return s
	}
	reader, err := charset.NewReader(strings.NewReader(s), "text/plain; charset="+cs)
	if err != nil {
		return s
	}
	converted, err := io.ReadAll(reader)
	if err != nil {
		return s
	}
	return string(converted)
}

func sanitizeUTF8(s string) string {
	return strings.ToValidUTF8(s, "")
}
