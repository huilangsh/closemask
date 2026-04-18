package proxy

import (
	"fmt"
	"log"
	"os"
)

// LogLevel 日志级别
type LogLevel int

const (
	LogLevelQuiet LogLevel = iota // 仅错误
	LogLevelInfo                  // 关键操作 + 错误（默认）
	LogLevelDebug                 // 全部
)

// Logger 日志记录器
type Logger struct {
	level LogLevel
	inner *log.Logger
}

// stdLogger 全局日志记录器
var stdLogger = &Logger{
	level: LogLevelInfo,
	inner: log.New(os.Stderr, "", log.LstdFlags),
}

// InitLogger 初始化全局日志记录器
func InitLogger(level string) {
	var l LogLevel
	switch level {
	case "quiet":
		l = LogLevelQuiet
	case "debug":
		l = LogLevelDebug
	default:
		l = LogLevelInfo
	}
	stdLogger.level = l
}

// Infof 输出 Info 级别日志
func (l *Logger) Infof(format string, args ...interface{}) {
	if l.level >= LogLevelInfo {
		l.inner.Output(2, fmt.Sprintf(format, args...))
	}
}

// Debugf 输出 Debug 级别日志
func (l *Logger) Debugf(format string, args ...interface{}) {
	if l.level >= LogLevelDebug {
		l.inner.Output(2, fmt.Sprintf(format, args...))
	}
}

// Errorf 输出 Error 级别日志（所有级别都输出）
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.inner.Output(2, fmt.Sprintf(format, args...))
}

// ============ 便捷函数 ============

// LogInfof 输出 Info 级别日志
func LogInfof(format string, args ...interface{}) {
	stdLogger.Infof(format, args...)
}

// LogDebugf 输出 Debug 级别日志
func LogDebugf(format string, args ...interface{}) {
	stdLogger.Debugf(format, args...)
}

// LogErrorf 输出 Error 级别日志
func LogErrorf(format string, args ...interface{}) {
	stdLogger.Errorf(format, args...)
}
