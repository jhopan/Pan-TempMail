package cloudflare

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrZoneNotFound is returned when a domain has no zone the panel can manage.
var ErrZoneNotFound = errors.New("cloudflare: zone not found or not active")

// APIError is a failure reported by the Cloudflare REST API, either through a
// non-2xx status or through an envelope with success=false.
type APIError struct {
	Status   int
	Codes    []int
	Messages []string
}

func (e *APIError) Error() string {
	if e == nil {
		return "cloudflare: <nil>"
	}
	message := strings.Join(e.Messages, "; ")
	if message == "" {
		message = "request failed"
	}
	if len(e.Codes) == 0 {
		return fmt.Sprintf("cloudflare %d: %s", e.Status, message)
	}
	codes := make([]string, 0, len(e.Codes))
	for _, code := range e.Codes {
		codes = append(codes, strconv.Itoa(code))
	}
	return fmt.Sprintf("cloudflare %d (%s): %s", e.Status, strings.Join(codes, ","), message)
}

// redact strips the API token from server supplied text so an echoed token can
// never reach logs or an HTTP response through an error message.
func (e *APIError) redact(token string) {
	if token == "" {
		return
	}
	for i, message := range e.Messages {
		e.Messages[i] = strings.ReplaceAll(message, token, "[redacted]")
	}
}

func hasCode(err error, code int) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	for _, candidate := range apiErr.Codes {
		if candidate == code {
			return true
		}
	}
	return false
}

func hasStatus(err error, status int) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.Status == status
}
