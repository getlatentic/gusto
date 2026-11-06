package main

import (
	"os"
	"strconv"
	"strings"
)

func setting(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

// values reads a setting written as one comma-separated line, which is how a
// systemd unit carries a list. Blank members are dropped, so a trailing comma
// does not become one.
func values(name string) []string {
	var members []string
	for _, each := range strings.Split(os.Getenv(name), ",") {
		if trimmed := strings.TrimSpace(each); trimmed != "" {
			members = append(members, trimmed)
		}
	}
	return members
}

func number(name string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err != nil || value < 1 {
		return fallback
	}
	return value
}
