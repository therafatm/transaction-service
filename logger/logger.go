// Copyright 2012 The Go Authors. All rights reserved.

// Use of this source code is governed by a BSD-style

// license that can be found in the LICENSE file.

package xml_test

import (
	"encoding/xml"
	"strconv"
	"time"

	"fmt"

	"os"
)

func LogCommand(command string, username string, funds string) {

	// Log a command sent to the system

	file, err := os.OpenFile("log.xsd", os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}

	type UserCommand struct {
		Timestamp string `xml:"timestamp"`

		Server int `xml:"server"`

		TransactionNumber int `xml:"transactionnumber"`

		Command string `xml:"action"`

		Username string `xml:"username"`

		Funds string `xml:"funds"`
	}

	v := &UserCommand{Timestamp: strconv.FormatInt(time.Now().UTC().UnixNano(), 10), Server: 1, Command: command, Username: username, Funds: funds}

	output, err := xml.MarshalIndent(v, "  ", "    ")

	if err != nil {

		fmt.Printf("error: %v\n", err)

	}

	file.Write(output)

}

func LogTransaction(command string, username string, funds string) {

	file, err := os.OpenFile("log.xsd", os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}

	type AccountTransaction struct {
		Timestamp string `xml:"timestamp"`

		Server int `xml:"server"`

		TransactionNumber int `xml:"transactionnumber"`

		Command string `xml:"action"`

		Username string `xml:"username"`

		Funds string `xml:"funds"`
	}

	v := &AccountTransaction{Timestamp: strconv.FormatInt(time.Now().UTC().UnixNano(), 10), Server: 1, Command: command, Username: username, Funds: funds}

	output, err := xml.MarshalIndent(v, "  ", "    ")

	if err != nil {

		fmt.Printf("error: %v\n", err)

	}

	file.Write(output)

}

func LogSystemEvnt(command string, username string, stocksymbol string, funds string) {

	file, err := os.OpenFile("log.xsd", os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}

	type SystemEvent struct {
		Timestamp string `xml:"timestamp"`

		Server int `xml:"server"`

		TransactionNumber int `xml:"transactionnumber"`

		Command string `xml:"action"`

		Username string `xml:"username"`

		StockSymbol string `xml:"stocksymbol"`

		Funds string `xml:"funds"`
	}

	v := &SystemEvent{Timestamp: strconv.FormatInt(time.Now().UTC().UnixNano(), 10), Server: 1, Command: command, Username: username, StockSymbol: stocksymbol, Funds: funds}

	output, err := xml.MarshalIndent(v, "  ", "    ")

	if err != nil {

		fmt.Printf("error: %v\n", err)

	}

	file.Write(output)

}

func LogQuoteServ(quoteservtime string, username string, stocksymbol string, price string, cryptokey string) {

	file, err := os.OpenFile("log.xsd", os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}

	type QuoteServer struct {
		Timestamp string `xml:"timestamp"`

		Server int `xml:"server"`

		TransactionNumber int `xml:"transactionnumber"`

		QuoteServerTime string `xml:"quoteservertime"`

		Username string `xml:"username"`

		StockSymbol string `xml:"stocksymbol"`

		Price string `xml:"price"`

		CryptoKey string `xml:"cryptokey"`
	}

	v := &QuoteServer{Timestamp: strconv.FormatInt(time.Now().UTC().UnixNano(), 10), Server: 1, QuoteServerTime: quoteservtime, Username: username, StockSymbol: stocksymbol, Price: price, CryptoKey: cryptokey}

	output, err := xml.MarshalIndent(v, "  ", "    ")

	if err != nil {

		fmt.Printf("error: %v\n", err)

	}

	file.Write(output)
}

func LogErrorEvent(command string, username string, stocksymbol string, funds string, emessage string) {

	file, err := os.OpenFile("log.xsd", os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}

	type ErrorEvent struct {
		Timestamp string `xml:"timestamp"`

		Server int `xml:"server"`

		TransactionNumber int `xml:"transactionnumber"`

		Command string `xml:"action"`

		Username string `xml:"username"`

		StockSymbol string `xml:"stocksymbol"`

		Funds string `xml:"funds"`

		ErrorMessage string `xml:"error"`
	}

	v := &ErrorEvent{Timestamp: strconv.FormatInt(time.Now().UTC().UnixNano(), 10), Server: 1, Command: command, Username: username, StockSymbol: stocksymbol, Funds: funds, ErrorMessage: emessage}

	output, err := xml.MarshalIndent(v, "  ", "    ")

	if err != nil {

		fmt.Printf("error: %v\n", err)

	}

	file.Write(output)
}
func InitLogger() {

	file, err := os.Create("log.xsd")

	if err != nil {
		return
	}

	file.WriteString("logfile initialized")

}
