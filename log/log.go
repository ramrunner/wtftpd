package log

import (
	"os"

	l "github.com/sirupsen/logrus"
)

const (
	logSubDisk   = "disk"
	logSubPacket = "packet"
	logSubEng    = "engine"
)

var (
	fileLogger *l.Logger
)

// InitLogs starts the logging subsystem
func InitLogs(a string, lf string) {
	l.SetFormatter(&l.JSONFormatter{})
	l.SetOutput(os.Stdout)
	f, err := os.OpenFile(lf, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		l.Fatalf("can't open log file:%s %s", lf, err)
	}
	fileLogger = l.New()
	fileLogger.SetOutput(f)
	fileLogger.SetFormatter(&l.JSONFormatter{})
	switch a {
	case "debug":
		l.SetLevel(l.DebugLevel)
	case "trace":
		l.SetLevel(l.TraceLevel)
	default:
		l.SetLevel(l.ErrorLevel)
	}
}

// EngineFileLogf logs on a file , engine messages at all levels.
func EngineFileLogf(format string, args ...interface{}) {
	fileLogger.WithFields(l.Fields{
		"system": logSubEng,
	}).Printf(format, args...)
}

// DiskInfof logs in the  info leve, disk subsystem messages
func DiskInfof(format string, args ...interface{}) {
	l.WithFields(l.Fields{
		"system": logSubDisk,
	}).Infof(format, args...)
}

// DiskTracef logs in the trace level, disk subsystem messages
func DiskTracef(format string, args ...interface{}) {
	l.WithFields(l.Fields{
		"system": logSubDisk,
	}).Tracef(format, args...)
}

// DiskDebugf logs in the debug level, disk subsystem messages
func DiskDebugf(format string, args ...interface{}) {
	l.WithFields(l.Fields{
		"system": logSubDisk,
	}).Debugf(format, args...)
}

// DiskFatalf logs in the fatal level, disk subsystem messages
func DiskFatalf(format string, args ...interface{}) {
	l.WithFields(l.Fields{
		"system": logSubDisk,
	}).Fatalf(format, args...)
}

// PacketTracef logs in the trace leve, packet subsystem messages
func PacketTracef(format string, args ...interface{}) {
	l.WithFields(l.Fields{
		"system": logSubPacket,
	}).Tracef(format, args...)
}

// EngineDebugf logs in the debug leven, engine subsystem messages
func EngineDebugf(format string, args ...interface{}) {
	l.WithFields(l.Fields{
		"system": logSubEng,
	}).Debugf(format, args...)
}

// EngineFatalf logs in the fatal level, engine subsystem messages
func EngineFatalf(format string, args ...interface{}) {
	l.WithFields(l.Fields{
		"system": logSubEng,
	}).Fatalf(format, args...)
}

// EngineFatal logs an error in the fatal level, engine subsystem messages
func EngineFatal(e error) {
	EngineFatalf("%s", e)
}

// EngineErrorf logs in the error level, engine subsystem messages
func EngineErrorf(format string, args ...interface{}) {
	l.WithFields(l.Fields{
		"system": logSubEng,
	}).Errorf(format, args...)
}

// EngineError logs an error in the error level, engine subsystem messages
func EngineError(e error) {
	EngineErrorf("%s", e)
}

// EngineTracef logs in the trace leve, engine subsystem messages
func EngineTracef(format string, args ...interface{}) {
	l.WithFields(l.Fields{
		"system": logSubPacket,
	}).Tracef(format, args...)
}
