package logging

import (
	"testing"
)

func TestDefaultLogLevelIsInfo(t *testing.T) {
	level := GetLogLevel()
	if level != Info {
		t.Errorf("expected default log level to be Info, got %v", level)
	}
}

func TestSetGetLogLevelRoundtrip(t *testing.T) {
	levels := []LogLevel{Debug, Info, Warning, Error}
	for _, level := range levels {
		SetLogLevel(level)
		got := GetLogLevel()
		if got != level {
			t.Errorf("SetLogLevel(%v) then GetLogLevel() = %v, want %v", level, got, level)
		}
	}
	SetLogLevel(Info)
}

func TestShouldLog(t *testing.T) {
	tests := []struct {
		current  LogLevel
		message  LogLevel
		expected bool
	}{
		{Debug, Debug, true},
		{Debug, Info, true},
		{Debug, Warning, true},
		{Debug, Error, true},
		{Info, Debug, false},
		{Info, Info, true},
		{Info, Warning, true},
		{Info, Error, true},
		{Warning, Debug, false},
		{Warning, Info, false},
		{Warning, Warning, true},
		{Warning, Error, true},
		{Error, Debug, false},
		{Error, Info, false},
		{Error, Warning, false},
		{Error, Error, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.current)+"_"+string(tt.message), func(t *testing.T) {
			SetLogLevel(tt.current)
			got := ShouldLog(tt.message)
			if got != tt.expected {
				t.Errorf("ShouldLog(%v) with level %v = %v, want %v",
					tt.message, tt.current, got, tt.expected)
			}
		})
	}
	SetLogLevel(Info)
}

func TestLogMessageDoesNotCrash(t *testing.T) {
	levels := []LogLevel{Debug, Info, Warning, Error}
	for _, level := range levels {
		SetLogLevel(level)
		LogMessage(level, "test message for level "+string(level))
	}
	SetLogLevel(Info)
}
