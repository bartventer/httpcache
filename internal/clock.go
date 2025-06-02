package internal

import "time"

type Clock interface {
	Now() time.Time
	Since(t time.Time) time.Duration
}

type clock struct{}

func NewClock() *clock { return &clock{} }

func (c clock) Since(t time.Time) time.Duration { return time.Since(t) }
func (c clock) Now() time.Time                  { return time.Now() }
