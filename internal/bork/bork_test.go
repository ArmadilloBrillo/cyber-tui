package bork_test

import (
	"testing"

	"github.com/ragnar/cyber-tui/internal/bork"
)

const trailer = " Bork Bork Bork!"

func TestBork_Rules(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"the", "the", "zee" + trailer},
		{"the capitalised", "The", "Zee" + trailer},
		{"the shouted", "THE", "ZEE" + trailer},
		{"tion suffix", "nation", "nashun" + trailer},
		{"w and o", "hello world", "hellu vurld" + trailer},
		{"silent e", "have", "hafe-a" + trailer},
		{"en suffix", "seven", "sefee" + trailer},
		{"an", "Man", "Mun" + trailer},
		{"leading e", "every", "ifery" + trailer},
		{"u", "you", "yuoo" + trailer},
		{"ow", "know", "knoo" + trailer},
		{"f", "fix", "ffix" + trailer},
		{"shouted word", "NATION", "NASHUN" + trailer},
		{"punctuation kept", "(the) Nation!", "(zee) Nashun!" + trailer},
		{"multibyte first letter", "Über", "Über" + trailer},
		{"spacing kept", "the  the", "zee  zee" + trailer},
		{"no letters", "123 :)", "123 :)" + trailer},
		{"mention", "@bob the", "@bob zee" + trailer},
		{"url", "go https://x.y/the now", "gu https://x.y/the noo" + trailer},
		{"inline code", "run `the cat` the", "roon `the cat` zee" + trailer},
		{"trailer on last prose line", "the\nnation", "zee\nnashun" + trailer},
		{"fence untouched", "the\n```\nthe\n```", "zee" + trailer + "\n```\nthe\n```"},
		{"only a fence", "```\nthe\n```", "```\nthe\n```"},
		{"empty", "", ""},
		{"blank", "   ", "   "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := bork.Bork(tt.in); got != tt.want {
				t.Errorf("Bork(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
