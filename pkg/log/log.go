package log

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/hadi77ir/go-logging"
)

func init() {
	startTime = time.Now()
}

var logger logging.Logger

func Global() logging.Logger {
	return logger
}

var startTime time.Time

func InitLogger(path string) error {
	var err error
	var file *os.File
	if path == "stderr" || path == "" {
		file = os.Stderr
	} else {
		file, err = os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
	}
	logger, err = NewFileLogger(file)
	if err != nil {
		return err
	}
	return nil
}

type MutexWriter struct {
	mutex  sync.Mutex
	writer io.Writer
}

func (m *MutexWriter) Write(p []byte) (n int, err error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.writer.Write(p)
}

func (m *MutexWriter) WriteMulti(p ...[]byte) (n int, err error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	n = 0
	for _, b := range p {
		i, err := m.writer.Write(b)
		if err != nil {
			return n, err
		}
		n += i
	}
	return n, nil
}

var _ io.Writer = &MutexWriter{}

func logLevelString(level logging.Level) string {
	switch level {
	case logging.TraceLevel:
		return "TRC"
	case logging.DebugLevel:
		return "DBG"
	case logging.InfoLevel:
		return "INF"
	case logging.WarnLevel:
		return "WRN"
	case logging.ErrorLevel:
		return "ERR"
	case logging.FatalLevel:
		return "FTL"
	case logging.PanicLevel:
		return "PNC"
	}
	return "INF"
}

type FileLogger struct {
	writer *MutexWriter
	fields logging.Fields
}

func NewFileLogger(writer io.Writer) (*FileLogger, error) {
	if writer == nil {
		return nil, errors.New("writer must not be nil")
	}
	fileLogger := &FileLogger{
		writer: &MutexWriter{writer: writer},
	}
	return fileLogger, nil
}

func timeElapsed() string {
	timeDuration := time.Since(startTime)
	return fmt.Sprintf("%8d.%.03d", int(math.Floor(timeDuration.Seconds())), timeDuration.Milliseconds()%1000)
}

func (f *FileLogger) Log(level logging.Level, args ...interface{}) {
	sb := &strings.Builder{}
	sb.WriteString("[")
	sb.WriteString(timeElapsed())
	sb.WriteString("][")
	sb.WriteString(logLevelString(level))
	sb.WriteString("][")
	sb.WriteString(formatArgs(args))
	sb.WriteString("]")
	if f.fields != nil && len(f.fields) != 0 {
		sb.WriteString("[")
		sb.WriteString(formatFields(f.fields))
		sb.WriteString("]")
	}
	sb.WriteString("\n")
	if level == logging.PanicLevel {
		sb.WriteString(string(debug.Stack()))
		sb.WriteString("\n")
	}

	_, _ = f.writer.WriteMulti([]byte(sb.String()))

	if level == logging.PanicLevel {
		panic(sb.String())
	}
}

func formatArgs(args []interface{}) string {
	return fmt.Sprint(args...)
}

func (f *FileLogger) WithFields(fields logging.Fields) logging.Logger {
	return &FieldsLogger{logger: f, fields: fields}
}

func (f *FileLogger) Logger() logging.Logger {
	return f
}

type FieldsLogger struct {
	logger logging.Logger
	fields logging.Fields
}

func formatFields(fields logging.Fields) string {
	formatted := ""
	for key, value := range fields {
		formatted += fmt.Sprintf("%s=%v ", key, value)
	}
	return formatted[:len(formatted)-1]
}

func (f *FieldsLogger) Log(level logging.Level, args ...interface{}) {
	var newArgs []interface{}
	newArgs = make([]interface{}, len(args)+1)
	n := copy(newArgs, args)
	newArgs[n] = formatFields(f.fields)
	f.logger.Log(level, newArgs...)
}

// WithFields returns a new logger with fields replaced with new ones. same as what "logrus" does.
func (f *FieldsLogger) WithFields(fields logging.Fields) logging.Logger {
	return &FieldsLogger{logger: f.logger, fields: fields}
}

func (f *FieldsLogger) Logger() logging.Logger {
	return f.logger
}

var _ logging.Logger = &FileLogger{}
