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

func InitLog() *Loger {
	writer2 := os.Stdout
	writer3, err := os.OpenFile("ussh/log.txt", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0755)
	if err != nil {
		log.Fatalf("create file log.txt failed: %v", err)
	}
	logger := log.New(io.MultiWriter(writer2, writer3), "[ussh]:", log.Lshortfile|log.LstdFlags|log.Lmicroseconds)
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
		l.Output(2, "[DEBUG]:"+fmt.Sprintln(i))
	}
}
