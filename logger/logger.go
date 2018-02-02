package logger

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"time"

	"github.com/lestrrat/go-libxml2"
	"github.com/lestrrat/go-libxml2/xsd"
	"github.com/streadway/amqp"
	"transaction_service/utils"
)

type Command string

const (
	ADD              = Command("ADD")
	QUOTE            = Command("QUOTE")
	BUY              = Command("BUY")
	COMMIT_BUY       = Command("COMMIT_BUY")
	CANCEL_BUY       = Command("CANCEL_BUY")
	SELL             = Command("SELL")
	COMMIT_SELL      = Command("COMMIT_SELL")
	CANCEL_SELL      = Command("CANCEL_SELL")
	SET_BUY_AMOUNT   = Command("SET_BUY_AMOUNT")
	CANCEL_SET_BUY   = Command("CANCEL_SET_BUY")
	SET_BUY_TRIGGER  = Command("SET_BUY_TRIGGER")
	SET_SELL_AMOUNT  = Command("SET_SELL_AMOUNT")
	SET_SELL_TRIGGER = Command("SET_SELL_TRIGGER")
	CANCEL_SET_SELL  = Command("CANCEL_SET_SELL")
	DUMPLOG          = Command("DUMPLOG")
	DISPLAY_SUMMARY  = Command("DISPLAY_SUMMARY")
)

var validCommands = map[Command]bool{
	ADD:              true,
	QUOTE:            true,
	BUY:              true,
	COMMIT_BUY:       true,
	CANCEL_BUY:       true,
	SELL:             true,
	COMMIT_SELL:      true,
	CANCEL_SELL:      true,
	SET_BUY_AMOUNT:   true,
	CANCEL_SET_BUY:   true,
	SET_BUY_TRIGGER:  true,
	SET_SELL_AMOUNT:  true,
	SET_SELL_TRIGGER: true,
	CANCEL_SET_SELL:  true,
	DUMPLOG:          true,
	DISPLAY_SUMMARY:  true}

type LogType struct {
	XMLName            string                  `xml:"log"`
	UserCommand        *UserCommandType        `xml:"userCommand,omitempty"`
	AccountTransaction *AccountTransactionType `xml:"accountTransaction,omitempty"`
	SystemEvent        *SystemEventType        `xml:"systemEvent,omitempty"`
	QuoteServer        *QuoteServerType        `xml:"quoteServer,omitempty"`
	ErrorEvent         *ErrorEventType         `xml:"errorEvent,omitempty"`
}

type UserCommandType struct {
	XMLName           string  `xml:"userCommand"`
	Timestamp         string  `xml:"timestamp"`
	Server            string  `xml:"server"`
	TransactionNumber string  `xml:"transactionNum"`
	Command           Command `xml:"command"`
	Username          string  `xml:"username,omitempty"`
	Symbol            string  `xml:"stockSymbol,omitempty"`
	Filename          string  `xml:"filename,omitempty"`
	Funds             string  `xml:"funds,omitempty"`
}

type AccountTransactionType struct {
	XMLName           string `xml:"accountTransaction"`
	Timestamp         string `xml:"timestamp"`
	Server            string `xml:"server"`
	TransactionNumber string `xml:"transactionNum"`
	Action            string `xml:"action"`
	Username          string `xml:"username"`
	Funds             string `xml:"funds"`
}

type SystemEventType struct {
	XMLName           string `xml:"systemEvent"`
	Timestamp         string `xml:"timestamp"`
	Server            string `xml:"server"`
	TransactionNumber string `xml:"transactionNum"`
	Command           string `xml:"command"`
	Username          string `xml:"username"`
	Symbol            string `xml:"stockSymbol"`
	Funds             string `xml:"funds"`
}

type QuoteServerType struct {
	XMLName           string `xml:"quoteServer"`
	Timestamp         string `xml:"timestamp"`
	Server            string `xml:"server"`
	TransactionNumber string `xml:"transactionNum"`
	QuoteServerTime   string `xml:"quoteServerTime"`
	Username          string `xml:"username"`
	Symbol            string `xml:"stockSymbol"`
	Price             string `xml:"price"`
	CryptoKey         string `xml:"cryptokey"`
}

type ErrorEventType struct {
	XMLName           string  `xml:"errorEvent"`
	Timestamp         string  `xml:"timestamp"`
	Server            string  `xml:"server"`
	TransactionNumber string  `xml:"transactionNum"`
	Command           Command `xml:"command"`
	Username          string  `xml:"username,omitempty"`
	Symbol            string  `xml:"stockSymbol,omitempty"`
	Funds             string  `xml:"funds,omitempty"`
	ErrorMessage      string  `xml:"errorMessage,omitempty"`
}

const server = "transaction"
const logfile = "log.xml"
const schemaFile = "logger/schema.xsd"
const prefix = ""
const indent = "\t"

var globalLog Logger

type Logger struct {
	Conn    *amqp.Connection
	Channel *amqp.Channel
	Queue   amqp.Queue
}

func failOnError(err error, msg string) {
	if err != nil {
		utils.LogErr(err, msg)
		panic(err)
	}
}

func InitLogger() (err error) {
	rabbitUser := os.Getenv("RABBITMQ_DEFAULT_USER")
	rabbitPass := os.Getenv("RABBITMQ_DEFAULT_PASS")
	rabbitHost := os.Getenv("RABBITMQ_HOST")
	rabbitPort := os.Getenv("RABBITMQ_PORT")
	url := fmt.Sprintf("amqp://%s:%s@%s:%s/", rabbitUser, rabbitPass, rabbitHost, rabbitPort)

	globalLog = Logger{}

	globalLog.Conn, err = amqp.Dial(url)
	failOnError(err, fmt.Sprintf("Failed to connect to Rabbit %s", url))

	globalLog.Channel, err = globalLog.Conn.Channel()
	failOnError(err, "Failed to open a channel")

	globalLog.Queue, err = globalLog.Channel.QueueDeclare(
		"log", // name
		false, // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	failOnError(err, "Failed to declare a queue")

	return
}

func Close() {
	if globalLog.Conn != nil {
		globalLog.Conn.Close()
	}
	if globalLog.Channel != nil {
		globalLog.Channel.Close()
	}
}

func publishXMLEntry(entry []byte) {
	err := globalLog.Channel.Publish(
		"",                   // exchange
		globalLog.Queue.Name, // routing key
		false,                // mandatory
		false,                // immediate
		amqp.Publishing{
			ContentType: "text/xml",
			Body:        entry,
		})
	if err != nil {
		utils.LogErr(err, "Failed to publish log message.")
	}
}

func formatStrAmount(amount string) (str string, err error) {
	b, err := strconv.Atoi(amount)
	if err != nil {
		return "", err
	}
	str = fmt.Sprintf("%d.%d", b/100, b%100)
	return
}

func formatAmount(amount int) string {
	return fmt.Sprintf("%d.%d", amount/100, amount%100)
}

func getUnixTimestamp() string {
	return fmt.Sprintf("%d", time.Now().UnixNano()/int64(time.Millisecond))
}

func validateSchema(ele []byte) {
	schema, err := os.Open(schemaFile)
	if err != nil {
		utils.LogErr(err, "failed to open file")
		return
	}
	defer schema.Close()

	schemabuf, err := ioutil.ReadAll(schema)
	if err != nil {
		utils.LogErr(err, "failed to read file")
		return
	}

	s, err := xsd.Parse(schemabuf)
	if err != nil {
		utils.LogErr(err, "failed to parse XSD")
		return
	}
	defer s.Free()

	wrapper := []byte(fmt.Sprintf("<log>%s</log>", ele))

	d, err := libxml2.Parse(wrapper)
	if err != nil {
		utils.LogErr(err, "failed to parse XML")
		return
	}

	if err := s.Validate(d); err != nil {
		for _, err := range err.(xsd.SchemaValidationError).Errors() {
			if err != nil {
				utils.LogErr(err, "failed to validate XML.")
				return
			}
		}
	}
	if err != nil {
		utils.LogErr(err, "failed to validate XML.")
	}
}

func LogCommand(command Command, vars map[string]string) {
	if _, exist := validCommands[command]; exist {
		timestamp := getUnixTimestamp()
		v := UserCommandType{Timestamp: timestamp, Server: server, Command: command}

		if val, exist := vars["trans"]; exist {
			v.TransactionNumber = val
		}
		if val, exist := vars["username"]; exist {
			v.Username = val
		}
		if val, exist := vars["symbol"]; exist {
			v.Symbol = val
		}
		if val, exist := vars["filename"]; exist {
			v.Filename = val
		}
		if val, exist := vars["amount"]; exist {
			var err error
			v.Funds, err = formatStrAmount(val)
			if err != nil {
				utils.LogErr(err, "Failed to format amount")
				return
			}
		}

		output, err := xml.MarshalIndent(v, prefix, indent)
		if err != nil {
			utils.LogErr(err, "failed to marshal log command.")
			return
		}
		publishXMLEntry(output)
		validateSchema(output)
	}
}

func LogQuoteServ(username string, price string, stocksymbol string, quoteTimestamp string, cryptokey string, trans string) {
	timestamp := getUnixTimestamp()

	v := QuoteServerType{Timestamp: timestamp,
		Server:            server,
		QuoteServerTime:   quoteTimestamp,
		Username:          username,
		Symbol:            stocksymbol,
		Price:             price,
		CryptoKey:         cryptokey,
		TransactionNumber: trans}

	output, err := xml.MarshalIndent(v, prefix, indent)
	if err != nil {
		utils.LogErr(err, "failed to marshal quote server request.")
		return
	}

	publishXMLEntry(output)
	validateSchema(output)
}

func LogTransaction(action string, username string, amount int, trans string) {
	file, err := os.OpenFile(logfile, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		utils.LogErr(err, "failed to log transaction.")
		return
	}
	defer file.Close()

	timestamp := getUnixTimestamp()
	v := AccountTransactionType{
		Timestamp:         timestamp,
		Server:            server,
		TransactionNumber: trans,
		Username:          username,
		Action:            action,
		Funds:             formatAmount(amount),
	}

	output, err := xml.MarshalIndent(v, prefix, indent)
	if err != nil {
		utils.LogErr(err, "failed to marshal transaction.")
		return
	}
	publishXMLEntry(output)
	validateSchema(output)
}

// func LogSystemEvnt(command string, username string, stocksymbol string, funds string) {

// 	file, err := os.OpenFile("log.xsd", os.O_APPEND|os.O_WRONLY, 0600)
// 	if err != nil {
// 		panic(err)
// 	}

// 	v := &SystemEvent{Timestamp: strconv.FormatInt(time.Now().UTC().UnixNano(), 10), Server: 1, Command: command, Username: username, StockSymbol: stocksymbol, Funds: funds}

// 	output, err := xml.MarshalIndent(v, "  ", "    ")

// 	if err != nil {

// 		fmt.Printf("error: %v\n", err)

// 	}

// 	file.Write(output)

// }

func LogErrorEvent(command Command, vars map[string]string, emessage string) {
	timestamp := getUnixTimestamp()
	v := ErrorEventType{
		Timestamp:    timestamp,
		Server:       server,
		Command:      command,
		ErrorMessage: emessage}

	if val, exist := vars["trans"]; exist {
		v.TransactionNumber = val
	}
	if val, exist := vars["username"]; exist {
		v.Username = val
	}
	if val, exist := vars["symbol"]; exist {
		v.Symbol = val
	}
	if val, exist := vars["amount"]; exist {
		var err error
		v.Funds, err = formatStrAmount(val)
		if err != nil {
			utils.LogErr(err, "Failed to format amount")
			return
		}
	}

	output, err := xml.MarshalIndent(v, prefix, indent)
	if err != nil {
		utils.LogErr(err, "failed to marshal error event.")
		return
	}

	publishXMLEntry(output)
	validateSchema(output)
}
