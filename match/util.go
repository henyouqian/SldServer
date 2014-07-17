package main

import (
	"unicode/utf8"
)

func stringLimit(str *string, limit uint) {
	if uint(len(*str)) > limit {
		*str = (*str)[:limit]
		for len(*str) > 0 {
			if utf8.ValidString(*str) {
				return
			}
			*str = (*str)[:len(*str)-1]
		}
	}
}
