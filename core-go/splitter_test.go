package main

import (
	"reflect"
	"testing"
)

func TestCalculateRetryChunkSeconds(t *testing.T) {
	tests := []struct {
		initial int
		want    []int
	}{
		{300, []int{300, 150, 75, 60}},
		{120, []int{120, 60}},
		{60, []int{60}},
		{30, []int{30}}, // below min, single value
		{10, []int{10}},
		{240, []int{240, 120, 60}},
	}
	for _, tt := range tests {
		got := CalculateRetryChunkSeconds(tt.initial)
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("CalculateRetryChunkSeconds(%d) = %v, want %v", tt.initial, got, tt.want)
		}
	}
}

func TestChunkAudioFormat(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"mp3", "mp3"},
		{"wav", "wav"},
		{"m4a", "wav"},
	}
	for _, tt := range tests {
		got := chunkAudioFormat(tt.input)
		if got != tt.want {
			t.Errorf("chunkAudioFormat(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
