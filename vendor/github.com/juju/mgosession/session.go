// Copyright 2016 Canonical Ltd.

// Package mgosession provides multiplexing for MongoDB sessions. It is designed
// so that many concurrent operations can be performed without
// using one MongoDB socket connection for each operation.
package mgosession

import (
	"fmt"
	"sync"
	"time"

	"github.com/juju/utils/clock"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/tomb.v2"
)

const pingInterval = 1 * time.Second

var Clock clock.Clock = clock.WallClock

// Pool represents a pool of mgo sessions.
type Pool struct {
	tomb tomb.Tomb

	// mu guards the fields below it.
	mu sync.Mutex

	// sessions holds all the sessions currently available for use.
	sessions []*mgo.Session

	// sessionIndex holds the index of the next session that will
	// be returned from Pool.Session.
	sessionIndex int

	// session holds the base session from which all sessions
	// returned from Pool.Session will be copied.
	session *mgo.Session

	// closed holds whether the Pool has been closed.
	closed bool
}

// Logger is used by mgosession to log information about sessions.
type Logger interface {
	// Debug logs a message at debug level.
	Debug(message string)
	// Info logs a message at info level.
	Info(message string)
}

// NewPool returns a session pool that maintains a maximum
// of maxSessions sessions available for reuse.
//
// All the sessions will be coped (with Session.Copy) from s.
//
// The logger is used to log informational messages about the pool
// and may be nil if no logging is required.
func NewPool(logger Logger, s *mgo.Session, maxSessions int) *Pool {
	if logger == nil {
		logger = nullLogger{}
	}
	p := &Pool{
		sessions: make([]*mgo.Session, maxSessions),
		session:  s.Copy(),
	}
	p.tomb.Go(func() error {
		return p.pinger(logger)
	})
	return p
}

// pinger occasionally pings the sessions in the pool
// to make sure that they are OK, and resets the pool
// if it gets an error. This means that even if nothing
// external notices an error and calls Reset, our maximum
// window for an outage is pingInterval.
//
// If there was an IsDead method on mgo.Session,
// this would be unnecessary (as would Reset).
// See https://github.com/go-mgo/mgo/issues/124.
func (p *Pool) pinger(logger Logger) error {
	for {
		select {
		case <-p.tomb.Dying():
			return nil
		case <-Clock.After(pingInterval):
		}
		session := p.Session(logger)
		if session.Ping() != nil {
			p.Reset()
		}
		session.Close()
	}
}

// Session returns a new session from the pool. It may
// reuse an existing session that has not been marked
// with DoNotReuse.
//
// The logger is used to log requests associated with
// the session request and may be nil if no logging is required.
//
// Session may be called concurrently.
func (p *Pool) Session(logger Logger) *mgo.Session {
	if logger == nil {
		logger = nullLogger{}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		panic("Session called on closed Pool")
	}
	s := p.sessions[p.sessionIndex]
	if s == nil {
		logger.Info(fmt.Sprintf("creating new session; index %d", p.sessionIndex))
		s = p.session.Copy()
		// Ping the session so that we're sure that the returned session
		// is attached to a mongodb socket otherwise we won't
		// be limiting the number of sessions at all.
		// Ignore the error because we'll do the same whether there's
		// an error or not.
		s.Ping()
		p.sessions[p.sessionIndex] = s
	} else {
		logger.Debug(fmt.Sprintf("reusing session; index %d", p.sessionIndex))
	}
	p.sessionIndex = (p.sessionIndex + 1) % len(p.sessions)
	return s.Clone()
}

// Close closes the pool. It may be called concurrently with other
// Pool methods, but once called, a call to Session will panic.
func (p *Pool) Close() {
	// First stop the pinger so that it won't
	// ask for any sessions after we're closed.
	p.tomb.Kill(nil)
	p.tomb.Wait()

	// Then close everything down.
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.closed = true
	p.closeSessions()
	p.session.Close()
}

// Reset resets the session pool so that no existing
// sessions will be reused. This should be called
// when an unexpected error has been encountered using
// a session.
func (p *Pool) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeSessions()
}

func (p *Pool) closeSessions() {
	for i, session := range p.sessions {
		if session != nil {
			session.Close()
			p.sessions[i] = nil
		}
	}
}

// nullLogger implements Logger by doing nothing.
type nullLogger struct{}

func (nullLogger) Debug(string) {}

func (nullLogger) Info(string) {}
