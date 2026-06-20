package main

import (
	"strings"
	"time"
)

func parseFrontMatter(fm string) Post {
	post := Post{}
	for line := range strings.SplitSeq(fm, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		kv := strings.SplitN(line, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		value := strings.Trim(strings.TrimSpace(kv[1]), `"`)
		switch key {
		case "title":
			post.Title = value
		case "subtitle":
			post.Subtitle = value
		case "is_draft":
			post.IsDraft = value == "true"
		case "date":
			post.Date, _ = time.Parse("02-01-2006 15:04", value)
		}
	}
	return post
}
