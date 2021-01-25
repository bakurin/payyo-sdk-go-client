package client

import (
	"io/ioutil"
	"log"
	"os"
)

// LogLevel specifies the logger log level
type LogLevel uint32

const (
	// ErrorLevel level. Used for errors that should definitely be noted.
	ErrorLevel LogLevel = iota
	// WarningLevel level. Non-critical entries.
	WarningLevel
	// InfoLevel level
	InfoLevel
	// DebugLevel level. Very verbose logging.
	DebugLevel
)

func (l LogLevel) String() string {
	switch {
	case l == ErrorLevel:
		return "error"
	case l == WarningLevel:
		return "warning"
	case l == InfoLevel:
		return "info"
	default:
		return "debug"
	}
}

// Logger interfaces defines minimalistic interface to log messages
type Logger interface {
	Logf(level LogLevel, format string, args ...interface{})
}

// NewDefaultLogger returns a Logger which will write log messages to stdout
func NewDefaultLogger(level LogLevel) Logger {
	return &defaultLogger{
		level:  level,
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
	level  LogLevel
	logger *log.Logger
}

// Log logs the parameters to the stdlib logger. See log.Printf.
func (l defaultLogger) Logf(level LogLevel, format string, args ...interface{}) {
	if l.level >= level {
		l.logger.Printf(format, args...)
	}
}

// LoggerFunc provides a convenient way to wrap any function to Logger interface
type LoggerFunc func(level LogLevel, format string, args ...interface{})

// Logf calls the wrapped function with the arguments provided
func (f LoggerFunc) Logf(level LogLevel, format string, args ...interface{}) {
	f(level, format, args...)
}
