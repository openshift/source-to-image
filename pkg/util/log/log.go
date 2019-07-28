package log

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"k8s.io/klog"
)

// Logger is a simple interface that is roughly equivalent to klog.
type Logger interface {
	Is(level int32) bool
	V(level int32) VerboseLogger
	Infof(format string, args ...interface{})
	Info(args ...interface{})
	Warningf(format string, args ...interface{})
	Warning(args ...interface{})
	Errorf(format string, args ...interface{})
	Error(args ...interface{})
	Fatalf(format string, args ...interface{})
	Fatal(args ...interface{})
}

// VerboseLogger is roughly equivalent to klog's Verbose.
type VerboseLogger interface {
	Infof(format string, args ...interface{})
	Info(args ...interface{})
}

// ToFile creates a logger that will log any items at level or below to file, and defer
// any other output to klog (no matter what the level is).
func ToFile(x io.Writer, level int32) Logger {
	return &FileLogger{
		&sync.Mutex{},
		bufio.NewWriter(x),
		level,
	}
}

var (
	// None implements the Logger interface but does nothing with the log output.
	None Logger = discard{}
	// StderrLog implements the Logger interface for stderr.
	StderrLog = ToFile(os.Stderr, 2)
)

// discard is a Logger that outputs nothing.
type discard struct{}

// Is returns whether the current logging level is greater than or equal to the parameter.
func (discard) Is(level int32) bool {
	return false
}

// V will returns a logger which will discard output if the specified level is greater than the current logging level.
func (discarding discard) V(level int32) VerboseLogger {
	return discarding
}

// Infof records an info log entry.
func (discard) Infof(string, ...interface{}) {
}

// Info records an info log entry.
func (discard) Info(...interface{}) {
}

// Errorf records an error log entry.
func (discard) Errorf(string, ...interface{}) {
}

// Error records an error log entry.
func (discard) Error(...interface{}) {
}

// Warningf records an warning log entry.
func (discard) Warningf(string, ...interface{}) {
}

// Warning records an warning log entry.
func (discard) Warning(...interface{}) {
}

// Fatalf records a fatal log entry.
func (discard) Fatalf(string, ...interface{}) {
}

// Fatal records a fatal log entry.
func (discard) Fatal(...interface{}) {
}

// FileLogger logs the provided messages at level or below to the writer, or delegates
// to klog.
type FileLogger struct {
	mutex *sync.Mutex
	w     *bufio.Writer
	level int32
}

// Is returns whether the current logging level is greater than or equal to the parameter.
func (f *FileLogger) Is(level int32) bool {
	return level <= f.level
}

// V will returns a logger which will discard output if the specified level is greater than the current logging level.
func (f *FileLogger) V(level int32) VerboseLogger {
	// Is the loglevel set verbose enough to accept the forthcoming log statement
	if klog.V(klog.Level(level)) {
		return f
	}
	// Otherwise discard
	return None
}

type severity int32

const (
	infoLog severity = iota
	warningLog
	errorLog
	fatalLog
)

// Elevated logger methods output a detailed prefix for each logging statement.
// At present, we delegate to klog to accomplish this.
type elevated func(int, ...interface{})

type severityDetail struct {
	prefix     string
	delegateFn elevated
}

var severities = []severityDetail{
	infoLog:    {"", klog.InfoDepth},
	warningLog: {"WARNING: ", klog.WarningDepth},
	errorLog:   {"ERROR: ", klog.ErrorDepth},
	fatalLog:   {"FATAL: ", klog.FatalDepth},
}

func (f *FileLogger) writeln(sev severity, line string) {
	severity := severities[sev]

	// If the loglevel has been elevated above this file logger's verbosity (generally set to 2)
	// then delegate ALL messages to elevated logger in order to leverage its file/line/timestamp
	// prefix information.
	if klog.V(klog.Level(f.level + 1)) {
		severity.delegateFn(3, line)
	} else {
		// buf.io is not threadsafe, so serialize access to the stream
		f.mutex.Lock()
		defer f.mutex.Unlock()
		f.w.WriteString(severity.prefix)
		f.w.WriteString(line)
		if !strings.HasSuffix(line, "\n") {
			f.w.WriteByte('\n')
		}
		f.w.Flush()
	}
}

func (f *FileLogger) outputf(sev severity, format string, args ...interface{}) {
	f.writeln(sev, fmt.Sprintf(format, args...))
}

func (f *FileLogger) output(sev severity, args ...interface{}) {
	f.writeln(sev, fmt.Sprint(args...))
}

// Infof records an info log entry.
func (f *FileLogger) Infof(format string, args ...interface{}) {
	f.outputf(infoLog, format, args...)
}

// Info records an info log entry.
func (f *FileLogger) Info(args ...interface{}) {
	f.output(infoLog, args...)
}

// Warningf records an warning log entry.
func (f *FileLogger) Warningf(format string, args ...interface{}) {
	f.outputf(warningLog, format, args...)
}

// Warning records an warning log entry.
func (f *FileLogger) Warning(args ...interface{}) {
	f.output(warningLog, args...)
}

// Errorf records an error log entry.
func (f *FileLogger) Errorf(format string, args ...interface{}) {
	f.outputf(errorLog, format, args...)
}

// Error records an error log entry.
func (f *FileLogger) Error(args ...interface{}) {
	f.output(errorLog, args...)
}

// Fatalf records a fatal log entry and terminates the program.
func (f *FileLogger) Fatalf(format string, args ...interface{}) {
	defer os.Exit(1)
	f.outputf(fatalLog, format, args...)
}

// Fatal records a fatal log entry and terminates the program.
func (f *FileLogger) Fatal(args ...interface{}) {
	defer os.Exit(1)
	f.output(fatalLog, args...)
}
