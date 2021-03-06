package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type logRecord struct {
	http.ResponseWriter

	time                time.Time
	ip, method, rawpath string
	responseBytes       int64
	responseStatus      int
	responseWritten		bool
	userAgent, referer  string
	proto               string // "HTTP/1.1"
}

type logHandler struct {
	ch      chan *logRecord
	handler http.Handler

	dir    string // or "" to not log
	stdout bool
}

func NewLoggingHandler(handler http.Handler, dir string, writeStdout bool) http.Handler {
	h := &logHandler{
		ch:      make(chan *logRecord, 1000),
		dir:     dir,
		handler: handler,
		stdout:  writeStdout,
	}
	go h.logFromChannel()
	return h
}

func (h *logHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	// Strip port number from address
	addr := r.RemoteAddr
	if colon := strings.LastIndex(addr, ":"); colon != -1 {
		addr = addr[:colon]
	}

	lr := &logRecord{
		time:           time.Now().UTC(),
		ip:             addr,
		method:         r.Method,
		rawpath:        r.URL.RequestURI(),
		userAgent:      r.UserAgent(),
		referer:        r.Referer(),
		responseStatus: http.StatusOK,
		responseWritten	false,
		proto:          r.Proto,
		ResponseWriter: rw,
	}
	h.handler.ServeHTTP(lr, r)
	h.ch <- lr
}

var monthAbbr = [12]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun",
	"Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}

func (h *logHandler) logFromChannel() {
	lastFileName := ""
	var logFile *os.File
	for {
		lr := <-h.ch

		// [10/Oct/2000:13:55:36 -0700]
		dateString := fmt.Sprintf("%02d/%s/%04d:%02d:%02d:%02d -0000",
			lr.time.Day(),
			monthAbbr[lr.time.Month()-1],
			lr.time.Year(),
			lr.time.Hour(), lr.time.Minute(), lr.time.Second())

		if h.dir != "" {
			fileName := fmt.Sprintf("%s/%04d-%02d-%02d%s%02d.log", h.dir,
				lr.time.Year(), lr.time.Month(), lr.time.Day(), "h", lr.time.Hour())
			if fileName > lastFileName {
				if logFile != nil {
					logFile.Close()
				}
				var err error
				logFile, err = os.OpenFile(fileName, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
				if err != nil {
					log.Printf("Error opening %q: %v", fileName, err)
					continue
				}
				lastFileName = fileName
			}
		}

		// Combined Log Format
		// http://httpd.apache.org/docs/1.3/logs.html#combined
		logLine := fmt.Sprintf("%s - - [%s] %q %d %d %q %q\n",
			lr.ip,
			dateString,
			lr.method+" "+lr.rawpath+" "+lr.proto,
			lr.responseStatus,
			lr.responseBytes,
			lr.referer,
			lr.userAgent,
		)
		if h.stdout {
			os.Stdout.WriteString(logLine)
		}
		if logFile != nil {
			logFile.WriteString(logLine)
		}
	}
}

func (lr *logRecord) Write(p []byte) (int, error) {
	if !lr.responseWritten {
		lr.responseWritten = true
		lr.responseStatus = http.StatusOK
	}
	written, err := lr.ResponseWriter.Write(p)
	lr.responseBytes += int64(written)
	return written, err
}

func (lr *logRecord) WriteHeader(status int) {
	if lr.responseWritten {
		return
	}
	lr.responseStatus = status
	lr.responseWritten = true
	lr.ResponseWriter.WriteHeader(status)
}
