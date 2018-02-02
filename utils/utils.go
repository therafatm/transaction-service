package utils

import (
	"log"
	"runtime"
)

func LogErr(err error, msg string) {
	_, fn, line, _ := runtime.Caller(1)
	log.Printf("Error %s:%d %v: %s", fn, line, err, msg)
}

func LogErrSkip(err error, msg string, skip int) {
	_, fn, line, _ := runtime.Caller(skip)
	log.Printf("Error %s:%d %v: %s", fn, line, err, msg)
}
