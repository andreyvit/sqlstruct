package sqlstruct

import (
	"strings"
)

func publishedName(n string) string {
	if n[:1] == strings.ToUpper(n[:1]) {
		return n
	}
	if lowercaseIdents[n] {
		return strings.ToUpper(n)
	} else {
		return strings.ToUpper(n[:1]) + n[1:]
	}
}

func unpublishedName(n string) string {
	if n[:1] == strings.ToLower(n[:1]) {
		return n
	}
	if n == strings.ToUpper(n) {
		return strings.ToLower(n)
	} else {
		return strings.ToLower(n[:1]) + n[1:]
	}
}

var lowercaseIdents = map[string]bool{
	"id": true,
	"pk": true,
}
