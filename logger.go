package client

import (
	"io/ioutil"
	"log"
	"os"
)

// Logger interfaces defines minimalistic interface to log messages
type Logger interface {
	Logf(format string, args ...interface{})
}

// NewDefaultLogger returns a Logger which will write log messages to stdout
func NewDefaultLogger() Logger {
	return &defaultLogger{
		logger: log.New(os.Stdout, "", log.LstdFlags),
	}
}

// NewNullLogger returns a Logger which prevents logging of unnecessary messages
func NewNullLogger() Logger {
	return &defaultLogger{
		logger: log.New(ioutil.Discard, "", 0),
	}
}

type defaultLogger struct {
	logger *log.Logger
}

// Log logs the parameters to the stdlib logger. See log.Printf.
func (l defaultLogger) Logf(format string, args ...interface{}) {
	l.logger.Printf(format, args...)
}

// LoggerFunc provides a convenient way to wrap any function to Logger interface
type LoggerFunc func(format string, args ...interface{})

// Logf calls the wrapped function with the arguments provided
func (f LoggerFunc) Logf(format string, args ...interface{}) {
	f(format, args...)
}
