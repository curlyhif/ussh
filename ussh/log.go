package ussh

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

type Loger struct {
	*log.Logger
}

var Slog Loger

const (
	LOG_IO_STDOUT  = 0x1
	LOG_IO_LOGFILE = 0x2
)

func InitLog(logIo uint8) *Loger {
	var ioList []io.Writer
	if logIo&LOG_IO_STDOUT == LOG_IO_STDOUT {
		ioList = append(ioList, os.Stdout)
	}

	if logIo&LOG_IO_LOGFILE == LOG_IO_LOGFILE {
		writer, err := os.OpenFile("Public/ussh/log.txt", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0755)
		if err != nil {
			log.Fatalf("create file log.txt failed: %v", err)
		}
		ioList = append(ioList, writer)
	}

	logger := log.New(io.MultiWriter(ioList...), "[ussh]:", log.Lshortfile|log.LstdFlags|log.Lmicroseconds)
	Slog.Logger = logger
	return &Slog
}

func (l *Loger) Info(v ...interface{}) {
	l.Output(2, "[INFO]:"+fmt.Sprintln(v...))
}

func (l *Loger) Debug(v ...interface{}) {
	l.Output(2, "[DEBUG]:"+fmt.Sprintln(v...))
}

func (l *Loger) Warn(v ...interface{}) {
	l.Output(2, "[WARN]:"+fmt.Sprintln(v...))
}

func (l *Loger) Error(v ...interface{}) {
	l.Output(2, "[ERROR]:"+fmt.Sprintln(v...))
}

func (l *Loger) DebugOut(v string) {
	str := strings.Split(v, "\n")
	for _, i := range str {
		if strings.Contains(i, "sudo") {
			continue
		}
		l.Output(2, "[DEBUGOUT]:"+fmt.Sprintln(i))
	}
}

func (l *Loger) ErrorOut(v string) {
	str := strings.Split(v, "\n")
	for _, i := range str {
		if strings.Contains(i, "sudo") {
			continue
		}
		l.Output(2, "[ERROROUT]:"+fmt.Sprintln(i))
	}
}
