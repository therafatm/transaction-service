package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	logger "transaction_service/logger"
	"transaction_service/queries/actions"
	"transaction_service/queries/utils"
	"transaction_service/utils"

	"github.com/gorilla/mux"
	// "github.com/phayes/freeport"
	_ "github.com/lib/pq"
)

var db *sql.DB

func connectToDB() *sql.DB {
	var (
		host     = os.Getenv("POSTGRES_HOST")
		user     = os.Getenv("POSTGRES_USER")
		password = os.Getenv("POSTGRES_PASSWORD")
		dbname   = os.Getenv("POSTGRES_DB")
	)

	port, err := strconv.Atoi(os.Getenv("DB_PORT"))

	config := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	db, err := sql.Open("postgres", config)
	utils.CheckErr(err)

	return db
}

func getQuoute(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	body, err := dbutils.QueryQuote(vars["username"], vars["stock"])

	if err != nil {
		w.Write([]byte("Error getting quote.\n"))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s := string(body[:])
	response := strings.Split(s, ",")

	price := response[0]
	symbol := response[1]
	userid := response[2]
	timestamp := response[3]
	cryptokey := response[4]

	logger.LogQuoteServ(timestamp, userid, symbol, price, cryptokey)
	w.Write([]byte(body))
}

func addUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	username := vars["username"]
	addMoney := vars["money"]

	_, balance, err := dbutils.QueryUser(username)

	if err != nil {
		if err == sql.ErrNoRows {
			err := dbactions.InsertUser(username, addMoney)
			if err != nil {
				w.Write([]byte("Failed to add user " + username + ".\n"))
				logger.LogErrorEvent("add", username, "", addMoney, "Failed to add user to DB")

				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			} else {
				w.Write([]byte("Successfully added user " + username))
				return
			}
		}

		logger.LogErrorEvent("add", username, "", addMoney, "Failed to add user to DB")

		w.Write([]byte("Failed to add user " + username + ".\n"))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	//add money to existing user
	addMoneyFloat, err := strconv.ParseFloat(addMoney, 64)
	balance += addMoneyFloat
	balanceString := fmt.Sprintf("%f", balance)

	logger.LogCommand("add", username, addMoney)

	err = dbactions.UpdateUser(username, balanceString)

	if err != nil {
		w.Write([]byte("Failed to update user " + username + ".\n"))
		logger.LogErrorEvent("add", username, "", addMoney, "Failed to update user to DB")

		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write([]byte("Successfully added user " + username))
	return
}

func buyOrder(w http.ResponseWriter, r *http.Request) {
	var balance float64
	const orderType string = "buy"

	vars := mux.Vars(r)
	username := vars["username"]
	stock := vars["stock"]
	buyAmount, _ := strconv.ParseFloat(vars["amount"], 64)

	_, balance, err := dbutils.QueryUser(username)

	money := strconv.FormatFloat(buyAmount, 'f', -1, 64)

	logger.LogSystemEvnt("buy", username, stock, money)

	// check that user exists and has enough money
	if err != nil {
		if err == sql.ErrNoRows {
			w.Write([]byte("Invalid user.\n"))
			http.Error(w, err.Error(), http.StatusForbidden)
			logger.LogErrorEvent("buy", username, "", money, "Invalid user")
			return
		}
		utils.LogErr(err)
		w.Write([]byte("Error getting user data.\n"))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		logger.LogErrorEvent("buy", username, "", money, "Error getting user data")

		return
	}

	if balance < buyAmount {
		w.Write([]byte("Insufficent balance."))
		logger.LogErrorEvent("buy", username, "", money, "Not enough money to buy stocks")

		return
	}

	// cancel existing reservation for the same stock, if exists
	err = dbactions.RemoveReservation(nil, username, stock, "buy", nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// addReservation()
	body, err := dbutils.QueryQuote(username, stock)
	if err != nil {
		utils.LogErr(err)
		if body != nil {
			w.Write([]byte("Error getting stock quote.\n"))
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Write([]byte("Error converting quote to string.\n"))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	quote, _ := strconv.ParseFloat(strings.Split(string(body), ",")[0], 64)
	buyUnits := int(buyAmount / quote)

	err = dbactions.BuyOrderTx(username, stock, "buy", buyUnits, buyAmount)

	if err != nil {
		utils.LogErr(err)
		w.Write([]byte("Error setting buy amount."))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write([]byte("Buy order placed. You have 60 seconds to confirm your order; otherwise, it will be dropped."))

	// remove reservation if not bought within 60 seconds
	go dbactions.RemoveOrder(username, stock, orderType, 60)
}

func commitBuy(w http.ResponseWriter, r *http.Request) {
	const orderType = "buy"
	var requestParams = mux.Vars(r)
	err := dbactions.CommitBuySellTransaction(requestParams["username"], orderType)

	logger.LogCommand("commit buy", requestParams["username"], "")

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		logger.LogErrorEvent("commit buy", requestParams["username"], "", "", "Error getting user data")

		return
	}

	w.Write([]byte("Sucessfully committed transaction."))
	return
}

func cancelBuy(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	username := vars["username"]
	err := dbactions.RemoveLastOrderTypeReservation(username, "buy")

	logger.LogCommand("cancel buy", username, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		logger.LogErrorEvent("cancel buy", username, "", "", "Cancel buy failed")

		return
	}

	res := []byte("Successfully cancelled most recent BUY command\n")
	w.Write(res)
}

func cancelSell(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	username := vars["username"]
	err := dbactions.RemoveLastOrderTypeReservation(username, "sell")

	logger.LogCommand("cancel sell", username, "")

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		logger.LogErrorEvent("cancel buy", username, "", "", "Cancel sell failed")

		return
	}

	res := []byte("Successfully cancelled most recent SELL command\n")
	w.Write(res)
}

func sellOrder(w http.ResponseWriter, r *http.Request) {
	const orderType = "sell"
	var userShares int

	vars := mux.Vars(r)
	username := vars["username"]
	stock := vars["stock"]
	sellAmount, _ := strconv.ParseFloat(vars["amount"], 64)

	logger.LogCommand("sell order", username, "")

	// confirm that user has enough valued stock
	// to complete sell

	_, userShares, err := dbutils.QueryUserStock(username, stock)

	if err != nil {
		if err == sql.ErrNoRows {
			w.Write([]byte("User has no shares of this stock."))
			logger.LogErrorEvent("Sell Order", username, "", "", "User Has no Shares")

			return
		}
		utils.LogErr(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	body, err := dbutils.QueryQuote(username, stock)
	if err != nil {
		utils.LogErr(err)
		w.Write([]byte("Error getting stock quote.\n"))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	quote, _ := strconv.ParseFloat(strings.Split(string(body), ",")[0], 64)
	balance := quote * float64(userShares)

	if balance < sellAmount {
		logger.LogErrorEvent("Sell Order", username, "", "", "Insufficient Balance")

		w.Write([]byte("Insufficent balance to sell stock."))
		return
	}

	// cancel existing sell reservation for this stock
	err = dbactions.RemoveReservation(nil, username, stock, "sell", nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// add new reservation
	sellUnits := int(sellAmount / quote)

	err = dbactions.AddReservation(nil, username, stock, orderType, sellUnits, sellAmount, nil)
	if err != nil {
		utils.LogErr(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write([]byte("Sell order placed. You have 60 seconds to confirm your order; otherwise, it will be dropped."))
	go dbactions.RemoveOrder(username, stock, orderType, 60)
}

func commitSell(w http.ResponseWriter, r *http.Request) {
	const orderType = "sell"
	var requestParams = mux.Vars(r)
	err := dbactions.CommitBuySellTransaction(requestParams["username"], orderType)

	logger.LogCommand("Commit Sell", requestParams["username"], "")

	if err != nil {
		logger.LogErrorEvent("Commit Sell", requestParams["username"], "", "", "Commit Sell failed")

		utils.LogErr(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write([]byte("Sucessfully comitted transaction."))
	return
}

func setBuyAmount(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	username := vars["username"]
	stock := vars["stock"]
	buyAmount, _ := strconv.ParseFloat(vars["amount"], 64)
	orderType := "buy"

	logger.LogCommand("Set Buy", username, "")

	_, userBalance, err := dbutils.QueryUser(username)

	// check that user exists and has enough money
	if err != nil {
		if err == sql.ErrNoRows {
			w.Write([]byte("Invalid user."))
			logger.LogErrorEvent("Set Buy", username, "", "", "No User Found")

			return
		}
		utils.LogErr(err)
		w.Write([]byte("Error getting user data."))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if userBalance < buyAmount {
		logger.LogErrorEvent("Set Buy", username, "", "", "Insufficient Balance")

		//log.Printf("User balance: %f\nBuy amount: %f", buyAmount, userBalance)
		w.Write([]byte("Insufficent balance."))
		return
	}

	_, _, totalValue, triggerPriceDB, err := dbutils.QueryUserStockTrigger(username, stock, orderType)
	if totalValue > 0 || triggerPriceDB > 0 {
		logger.LogErrorEvent("Set Buy", username, "", "", "SET BUY AMOUNT already exists for this stock and user combination.\nCancel current SET BUY and try again.\n")
		w.Write([]byte("SET BUY AMOUNT already exists for this stock and user combination.\nCancel current SET BUY and try again.\n"))
		return
	}

	err = dbactions.ExecuteSetBuyAmount(username, stock, orderType, buyAmount)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	m := string("Sucessfully comitted SET BUY AMOUNT transaction.")
	w.Write([]byte(m))
	return
}

func setBuyTrigger(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	username := vars["username"]
	stock := vars["stock"]
	triggerPrice := vars["triggerPrice"]
	orderType := "buy"

	logger.LogCommand("Set Buy trigger", username, "")

	// invalid trigger price
	if p, err := strconv.ParseFloat(triggerPrice, 64); p <= 0 || err != nil {
		logger.LogErrorEvent("Set Buy Trigger", username, "", "", "Invalid trigger price. Trigger price must be greater than 0.")

		w.Write([]byte("Invalid trigger price. Trigger price must be greater than 0.\n"))
		return
	}

	// check if user has SET BUY AMOUNT record in trigger DB
	_, _, totalValue, triggerPriceDB, err := dbutils.QueryUserStockTrigger(username, stock, orderType)

	if err != nil {
		if err == sql.ErrNoRows {
			logger.LogErrorEvent("Set Buy Trigger", username, "", "", "SET BUY AMOUNT doesn't exist for this stock and user combination.")

			w.Write([]byte("SET BUY AMOUNT doesn't exist for this stock and user combination.\nCannot process trigger.\n"))
			return
		}
		utils.LogErr(err)
		w.Write([]byte("Error getting user data."))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// trigger already exists, return error
	if totalValue > 0 && triggerPriceDB > 0 {
		w.Write([]byte("SET BUY TRIGGER already exists for this stock and user combination.\nCancel current SET BUY and try again.\n"))
		logger.LogErrorEvent("Set Buy Trigger", username, "", "", "SET BUY TRIGGER already exists for this stock and user combination.")

		return
	}

	err = dbactions.SetBuyTrigger(username, stock, orderType, triggerPrice)
	if err != nil {
		w.Write([]byte("Failed to SET BUY trigger.\n"))
		logger.LogErrorEvent("Set Buy Trigger", username, "", "", "Failed to SET BUY trigger.")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write([]byte("Successfully SET BUY trigger."))
	return
}

func setSellAmount(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	username := vars["username"]
	symbol := vars["stock"]
	sellAmount, _ := strconv.ParseFloat(vars["amount"], 64)
	orderType := "sell"

	logger.LogCommand("Set Sell Amount", username, "")

	// confirm that user has enough valued stock
	// to complete sell

	_, userShares, err := dbutils.QueryUserStock(username, symbol)
	if err != nil {
		if err == sql.ErrNoRows {
			w.Write([]byte("User has no shares of this stock."))
			logger.LogErrorEvent("Set Sell Amount", username, "", "", "User has no shares of stock")
			return
		}
		utils.LogErr(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	_, _, totalValue, triggerPriceDB, err := dbutils.QueryUserStockTrigger(username, symbol, orderType)
	if totalValue > 0 || triggerPriceDB > 0 {
		logger.LogErrorEvent("Set Sell Amount", username, "", "", "SET SELL AMOUNT already exists for this stock and user combination.")

		w.Write([]byte("SET SELL AMOUNT already exists for this stock and user combination.\nCancel current SET SELL and try again.\n"))
		return
	}

	body, err := dbutils.QueryQuote(username, symbol)
	if err != nil {
		utils.LogErr(err)
		w.Write([]byte("Error getting stock quote.\n"))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	quote, _ := strconv.ParseFloat(strings.Split(string(body), ",")[0], 64)
	balance := quote * float64(userShares)

	if balance < sellAmount {
		logger.LogErrorEvent("Set Sell Amount", username, "", "", "Insufficent balance to sell stock.")

		w.Write([]byte("Insufficent balance to sell stock."))
		return
	}

	err = dbactions.SetUserOrderTypeAmount(nil, username, symbol, orderType, sellAmount, nil)

	if err != nil {
		logger.LogErrorEvent("Set Sell Amount", username, "", "", "Error setting SET SELL amount.")

		w.Write([]byte("Error setting SET SELL amount."))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write([]byte("Successfully SET SELL amount"))
	return
}

func setSellTrigger(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	username := vars["username"]
	stock := vars["stock"]
	triggerPrice := vars["triggerPrice"]
	orderType := "sell"

	logger.LogCommand("Set Sell Trigger", username, "")

	_, _, totalValue, triggerPriceDB, err := dbutils.QueryUserStockTrigger(username, stock, orderType)
	if totalValue > 0 && triggerPriceDB > 0 {
		logger.LogErrorEvent("Set Sell Trigger", username, "", "", "SET SELL AMOUNT already exists for this stock and user combination.")

		w.Write([]byte("SET SELL AMOUNT already exists for this stock and user combination.\nCancel current SET SELL and try again.\n"))
		return
	}

	if err != nil {
		if err == sql.ErrNoRows {
			logger.LogErrorEvent("Set Sell Trigger", username, "", "", "SET SELL AMOUNT doesn't exist for this stock and user combination.")
			w.Write([]byte("SET SELL AMOUNT doesn't exist for this stock and user combination.\nCannot process trigger.\n"))
			return
		}
		utils.LogErr(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	triggerPriceFloat, _ := strconv.ParseFloat(triggerPrice, 64)

	err = dbactions.SetSellTrigger(username, stock, totalValue, triggerPriceFloat)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write([]byte("Successfully SET SELL trigger."))
	return
}

func executeTrigger(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	username := vars["username"]
	symbol := vars["stock"]
	shares := vars["shares"]
	triggerValue, _ := strconv.ParseFloat(vars["triggerValue"], 64)
	totalValue, _ := strconv.ParseFloat(vars["totalValue"], 64)
	orderType := vars["orderType"]

	logger.LogSystemEvnt("Sell Trigger executed", username, symbol, shares)

	err := dbactions.ExecuteTrigger(username, symbol, shares, totalValue, triggerValue, orderType)

	if err != nil {
		logger.LogErrorEvent(" Sell Trigger", username, "", "", "Trigger failed")

		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	res := []byte("Sucessfully executed SET " + orderType + " trigger.")
	w.Write(res)
	return
}

func cancelSetBuy(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	username := vars["username"]
	symbol := vars["stock"]
	err := dbactions.CancelSetTrigger(username, symbol, "buy")

	logger.LogCommand("Cancel Set Buy", username, "")

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		logger.LogErrorEvent("Cancel Set Buy", username, "", "", "Cancel Set Buy failed")

		return
	}

	res := []byte("Successfully cancelled SET BUY\n")
	w.Write(res)
}

func cancelSetSell(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	username := vars["username"]
	symbol := vars["stock"]

	logger.LogCommand("Cancel Set Sell", username, "")

	err := dbactions.CancelSetTrigger(username, symbol, "sell")

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		logger.LogErrorEvent("Cancel Set Sell", username, "", "", "Cancel Set Buy failed")

		return
	}

	res := []byte("Successfully cancelled SET SELL\n")
	w.Write(res)
}

func validateURLParams(r *http.Request) (err error) {
	vars := mux.Vars(r)

	username, ok := vars["username"]
	if ok != false {
		if len(username) <= 0 {
			return errors.New("Invalid username\n")
		}
	}

	stock, ok := vars["stock"]
	if ok != false {
		// v, err := strconv.Atoi(stock)
		if len(stock) <= 0 {
			return errors.New("Invalid stock\n")
		}
		// allows stocks to be numbers
		// could add check for stocks not being number values
	}

	amount, ok := vars["amount"]
	if ok != false {
		floatAmount, err := strconv.ParseFloat(amount, 64)
		if floatAmount <= 0 || err != nil {
			return errors.New("Invalid amount\n")
		}
	}

	money, ok := vars["money"]
	if ok != false {
		floatMoney, err := strconv.ParseFloat(money, 64)
		if floatMoney <= 0 || err != nil {
			return errors.New("Invalid money\n")
		}
	}

	triggerPrice, ok := vars["triggerPrice"]
	if ok != false {
		floatTriggerPrice, err := strconv.ParseFloat(triggerPrice, 64)
		if floatTriggerPrice <= 0 || err != nil {
			return errors.New("Invalid trigger price\n")
		}
	}

	shares, ok := vars["shares"]
	if ok != false {
		intShares, err := strconv.Atoi(shares)
		if intShares <= 0 || err != nil {
			return errors.New("Invalid number of shares\n")
		}
	}

	orderType, ok := vars["orderType"]
	if ok != false {
		if len(orderType) <= 0 {
			return errors.New("Invalid order type\n")
		}
	}

	totalValue, ok := vars["totalValue"]
	if ok != false {
		floatTotalValue, err := strconv.ParseFloat(totalValue, 64)
		if floatTotalValue <= 0 || err != nil {
			return errors.New("Invalid totalValue\n")
		}
	}

	err = nil
	return err
}

func logHandler(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := fmt.Sprintf("%s - %s%s", r.Method, r.Host, r.URL)
		err := validateURLParams(r)
		if err != nil {
			utils.LogErr(err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		log.Println(l)
		fn(w, r)
	}
}

func main() {

	logger.InitLogger()

	//logger.LogCommand("add", "cameron", "100.00")
	//logger.LogTransaction("buy", "sfsdf", "220.00")
	//logger.LogSystemEvnt("sell", "username", "stocksymbol", "123.23")
	//logger.LogQuoteServ("command", "username", "servertime", "stocksymbol", "price", "key")
	//logger.LogErrorEvent("sell", "username", "stocksymbol", "123.23")

	db = connectToDB()
	defer db.Close()

	dbactions.SetActionsDB(db)
	dbutils.SetUtilsDB(db)

	router := mux.NewRouter()
	// port, _ := freeport.GetFreePort()
	port := 8888

	log.Println("Running transaction server on port: " + strconv.Itoa(port))

	router.HandleFunc("/api/addUser/{username}/{money}", logHandler(addUser))
	router.HandleFunc("/api/getQuote/{username}/{stock}", logHandler(getQuoute))

	router.HandleFunc("/api/buy/{username}/{stock}/{amount}", logHandler(buyOrder))
	router.HandleFunc("/api/commitBuy/{username}", logHandler(commitBuy))
	router.HandleFunc("/api/cancelBuy/{username}", logHandler(cancelBuy))

	router.HandleFunc("/api/sell/{username}/{stock}/{amount}", logHandler(sellOrder))
	router.HandleFunc("/api/commitSell/{username}", logHandler(commitSell))
	router.HandleFunc("/api/cancelSell/{username}", logHandler(cancelSell))

	router.HandleFunc("/api/setBuyAmount/{username}/{stock}/{amount}", logHandler(setBuyAmount))
	router.HandleFunc("/api/cancelSetBuy/{username}/{stock}", logHandler(cancelSetBuy))
	router.HandleFunc("/api/setBuyTrigger/{username}/{stock}/{triggerPrice}", logHandler(setBuyTrigger))

	router.HandleFunc("/api/setSellAmount/{username}/{stock}/{amount}", logHandler(setSellAmount))
	router.HandleFunc("/api/cancelSetSell/{username}/{stock}", logHandler(cancelSetSell))
	router.HandleFunc("/api/setSellTrigger/{username}/{stock}/{triggerPrice}", logHandler(setSellTrigger))

	router.HandleFunc("/api/executeTrigger/{username}/{stock}/{shares}/{totalValue}/{triggerValue}/{orderType}", logHandler(executeTrigger))

	http.Handle("/", router)

	// go triggermanager.Manage()

	if err := http.ListenAndServe(":"+strconv.Itoa(port), nil); err != nil {
		log.Fatal(err)
		panic(err)
	}
}
