package handlers

import (
	"regexp"
)

// regexpCompile compiles a regex pattern safely.
func regexpCompile(pattern string) *regexp.Regexp {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}
	return re
}
