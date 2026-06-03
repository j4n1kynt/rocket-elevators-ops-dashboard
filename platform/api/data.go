package main

import (
	"strconv"
	"strings"
)

func nullableString(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func isNumeric(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}
