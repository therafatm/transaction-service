package utils

import (
	"log"
	"runtime"
)

func CheckErr(err error) {
	if err != nil {
		LogErr(err)
		log.Fatal(err)
	}
}

func LogErr(err error) {
	_, fn, line, _ := runtime.Caller(1)
	log.Printf("[error] %s:%d %v", fn, line, err)
}
