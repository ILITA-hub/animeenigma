package service

import "time"

// Clock yields the current UTC date string; injectable for tests.
type Clock interface {
	Today() string // "2006-01-02" in UTC
}

type realClock struct{}

func (realClock) Today() string { return time.Now().UTC().Format("2006-01-02") }

// fixedClock is used by tests.
type fixedClock struct{ date string }

func (f fixedClock) Today() string { return f.date }
