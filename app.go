package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
	"strings"
	"encoding/json"

	"transaction_service/queries/actions"
	"transaction_service/queries/utils"
	"transaction_service/queries/models"
	"transaction_service/triggers/triggermanager"
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
		dbname = os.Getenv("POSTGRES_DB")
	)

	port, err := strconv.Atoi(os.Getenv("DB_PORT"))

	config := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	db, err := sql.Open("postgres", config)
	utils.CheckErr(err)

	return db
}

func respondWithError(w http.ResponseWriter, code int, err error, message string) {
	utils.LogErr(err)
    respondWithJSON(w, code, map[string]string{"error": err.Error(), "message": message})
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
    response, _ := json.Marshal(payload)

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    w.Write(response)
}

//TODO: refactor  + test
func getQuoute(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	body, err := dbutils.QueryQuote(vars["username"], vars["stock"])

	if err != nil {
		w.Write([]byte("Error getting quote.\n"))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write([]byte(body))
}

func clearUsers(w http.ResponseWriter, r *http.Request) {
	err := dbactions.ClearUsers()
	if err != nil {
		errMsg := "Failed to clear users"
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}
	w.WriteHeader(http.StatusOK)
    w.Write([]byte("Cleared users succesfully."))
}


func addUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	username := vars["username"]
	moneyStr := vars["money"]
	errMsg := fmt.Sprintf("Failed to add user %s", username)

	money, err := strconv.Atoi(moneyStr)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	user, err := dbutils.QueryUser(username)

	if err != nil && err == sql.ErrNoRows {
		//user no exist
		newUser := models.User{Username: username, Money: money}
		err := dbactions.InsertUser(newUser)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, err, errMsg)
			return
		}

	} else if err != nil {
		// error
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return 

	}else{
		// user exists
		user.Money += money
		err = dbactions.UpdateUser(user)

		if err != nil {
			respondWithError(w, http.StatusInternalServerError, err, errMsg)
			return
		}
	}

	user, err = dbutils.QueryUser(username)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}
	respondWithJSON(w, http.StatusOK, user)
}

func availableBalance(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	username := vars["username"]

	_, err := dbutils.QueryUser(username)
	if err != nil && err == sql.ErrNoRows {
		errMsg := fmt.Sprintf("No such user %s exists.", username)
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}else if err != nil {
		errMsg := fmt.Sprintf("Error retrieving user %s.", username)
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	balance, err := dbutils.QueryUserAvailableBalance(username)
	if err != nil {
		errMsg := fmt.Sprintf("Error getting user available balance for %s.", username)
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}
	var m map[string]int
	m = make(map[string]int)
	m["balance"] = balance

	respondWithJSON(w, http.StatusOK, m)
}

func buyOrder(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	username := vars["username"]
	symbol := vars["symbol"]
	
	buyAmount, err := strconv.Atoi(vars["amount"])
	if err != nil {
		errMsg := fmt.Sprintf("Invalid amount %s.", vars["amount"])
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	balance, err := dbutils.QueryUserAvailableBalance(username)

	// check that user exists and has enough money
	if err != nil {
		if err == sql.ErrNoRows {
			errMsg := fmt.Sprintf("Failed to find user %s.", username)
			respondWithError(w, http.StatusInternalServerError, err, errMsg)
			return
		}

		errMsg := fmt.Sprintf("Error getting user data for %s.", username)
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	if balance < buyAmount {
		errMsg := fmt.Sprintf("User does not have enough money to complete order.")
		err = errors.New("Error not enough money.")
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}


	// cancel existing reservation for the same stock, if exists
	err = dbactions.RemoveReservation(nil, username, symbol, models.BUY)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to cancel previous buy order for %s.", username)
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	// addReservation()
	body, err := dbutils.QueryQuote(username, symbol)
	if err != nil {
		if body != nil {
			errMsg := "Error getting stock quote for user."
			respondWithError(w, http.StatusInternalServerError, err, errMsg)
			return
		}
		errMsg := "Error converting quote to string."
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	priceStr :=  strings.Replace(strings.Split(string(body), ",")[0], ".", "", 1)
	quote, err := strconv.Atoi(priceStr)
	if err != nil {
		errMsg := "Error reading stock quote."
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	reservation := models.Reservation{ Username: username, Symbol: symbol, Order: models.BUY }
	reservation.Shares = buyAmount / quote
	reservation.Amount = reservation.Shares * quote
	reservation.Time = time.Now().Unix()

	err = dbactions.BuyOrderTx(reservation)
	if err != nil {
		errMsg := "Error setting buy order."
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	res, err := dbutils.QueryReservation(username, symbol, models.BUY)
	if err != nil {
		errMsg := "Error reservation not found after insert."
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	respondWithJSON(w, http.StatusOK, res)

	// remove reservation if not bought within 60 seconds
	go dbactions.RemoveOrder(reservation.Username, reservation.Symbol, reservation.Order, 60)
}

func commitBuy(w http.ResponseWriter, r *http.Request) {
	var requestParams = mux.Vars(r)
	username := requestParams["username"]

	res, err := dbutils.QueryLastReservation(username, models.BUY)
	if err != nil && err == sql.ErrNoRows {
		errMsg := "No reserved buy order to commit."
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}else if err != nil {
		errMsg := "Error finding last buy reservation."
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	err = dbactions.CommitBuySellTransaction(res)
	if err != nil {
		errMsg := "Error commiting order."
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	stock, err := dbutils.QueryUserStock(res.Username, res.Symbol)
	if err != nil {
		errMsg := "Error could not find updated stock."
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
	}

	respondWithJSON(w, http.StatusOK, stock)
	return
}

func cancelBuy(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	username := vars["username"]

	res, err := dbutils.QueryLastReservation(username, models.BUY)
	if err != nil && err == sql.ErrNoRows {
		errMsg := "No reserved buy order to delete."
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}else if err != nil {
		errMsg := "Error finding last buy reservation."
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	err = dbactions.RemoveLastOrderTypeReservation(username, models.BUY)
	if err != nil {
		errMsg := "Error deleting last buy reservation."
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
	}

	respondWithJSON(w, http.StatusOK, res)
	return
}

// func cancelSell(w http.ResponseWriter, r *http.Request) {
// 	vars := mux.Vars(r)
// 	username := vars["username"]
// 	err := dbactions.RemoveLastOrderTypeReservation(username, "sell")

// 	if err != nil {
// 		http.Error(w, err.Error(), http.StatusInternalServerError)
// 		return
// 	}

// 	res := []byte("Successfully cancelled most recent SELL command\n")
// 	w.Write(res)
// }

// func sellOrder(w http.ResponseWriter, r *http.Request) {
// 	const orderType = "sell"
// 	var userShares int

// 	vars := mux.Vars(r)
// 	username := vars["username"]
// 	stock := vars["stock"]
// 	sellAmount, _ := strconv.ParseFloat(vars["amount"], 64)

// 	// confirm that user has enough valued stock
// 	// to complete sell

// 	_, userShares, err := dbutils.QueryUserStock(username, stock)

// 	if err != nil {
// 		if err == sql.ErrNoRows {
// 			w.Write([]byte("User has no shares of this stock."))
// 			return
// 		}
// 		utils.LogErr(err)
// 		http.Error(w, err.Error(), http.StatusInternalServerError)
// 		return
// 	}

// 	body, err := dbutils.QueryQuote(username, stock)
// 	if err != nil {
// 		utils.LogErr(err)
// 		w.Write([]byte("Error getting stock quote.\n"))
// 		http.Error(w, err.Error(), http.StatusInternalServerError)
// 		return
// 	}

// 	quote, _ := strconv.ParseFloat(strings.Split(string(body), ",")[0], 64)
// 	balance := quote * float64(userShares)

// 	if balance < sellAmount {
// 		w.Write([]byte("Insufficent balance to sell stock."))
// 		return
// 	}

// 	// cancel existing sell reservation for this stock
// 	err = dbactions.RemoveReservation(nil, username, stock, "sell", nil)
// 	if err != nil {
// 		http.Error(w, err.Error(), http.StatusInternalServerError)
// 		return
// 	}

// 	// add new reservation
// 	sellUnits := int(sellAmount / quote)

// 	err = dbactions.AddReservation(nil, username, stock, orderType, sellUnits, sellAmount, nil)
// 	if err != nil {
// 		utils.LogErr(err)
// 		http.Error(w, err.Error(), http.StatusInternalServerError)
// 		return
// 	}

// 	w.Write([]byte("Sell order placed. You have 60 seconds to confirm your order; otherwise, it will be dropped."))
// 	go dbactions.RemoveOrder(username, stock, orderType, 60)
// }

// func commitSell(w http.ResponseWriter, r *http.Request) {
// 	const orderType = "sell"
// 	var requestParams = mux.Vars(r)
// 	err := dbactions.CommitBuySellTransaction(requestParams["username"], orderType)

// 	if err != nil {
// 		utils.LogErr(err)
// 		http.Error(w, err.Error(), http.StatusInternalServerError)
// 		return
// 	}

// 	w.Write([]byte("Sucessfully comitted transaction."))
// 	return
// }

// func setBuyAmount(w http.ResponseWriter, r *http.Request) {
// 	vars := mux.Vars(r)
// 	username := vars["username"]
// 	stock := vars["stock"]
// 	buyAmount, _ := strconv.ParseFloat(vars["amount"], 64)
// 	orderType := "buy"

// 	_, userBalance, err := dbutils.QueryUser(username)

// 	// check that user exists and has enough money
// 	if err != nil {
// 		if err == sql.ErrNoRows {
// 			w.Write([]byte("Invalid user."))
// 			return
// 		}
// 		utils.LogErr(err)
// 		w.Write([]byte("Error getting user data."))
// 		http.Error(w, err.Error(), http.StatusInternalServerError)
// 		return
// 	}

// 	if userBalance < buyAmount {
// 		log.Printf("User balance: %f\nBuy amount: %f", buyAmount, userBalance)
// 		w.Write([]byte("Insufficent balance."))
// 		return
// 	}

// 	_, _, totalValue, triggerPriceDB, err := dbutils.QueryUserStockTrigger(username, stock, orderType)
// 	if totalValue > 0 || triggerPriceDB > 0 {
// 		w.Write([]byte("SET BUY AMOUNT already exists for this stock and user combination.\nCancel current SET BUY and try again.\n"))
// 		return
// 	}

// 	err = dbactions.ExecuteSetBuyAmount(username, stock, orderType, buyAmount)
// 	if err != nil {
// 		http.Error(w, err.Error(), http.StatusInternalServerError)
// 		return
// 	}

// 	m := string("Sucessfully comitted SET BUY AMOUNT transaction.")
// 	w.Write([]byte(m))
// 	return
// }

// func setBuyTrigger(w http.ResponseWriter, r *http.Request) {
// 	vars := mux.Vars(r)
// 	username := vars["username"]
// 	stock := vars["stock"]
// 	triggerPrice := vars["triggerPrice"]
// 	orderType := "buy"

// 	// invalid trigger price
// 	if p, err := strconv.ParseFloat(triggerPrice, 64); p <= 0 || err != nil {
// 		w.Write([]byte("Invalid trigger price. Trigger price must be greater than 0.\n"))
// 		return
// 	}

// 	// check if user has SET BUY AMOUNT record in trigger DB
// 	_, _, totalValue, triggerPriceDB, err := dbutils.QueryUserStockTrigger(username, stock, orderType)

// 	if err != nil {
// 		if err == sql.ErrNoRows {
// 			w.Write([]byte("SET BUY AMOUNT doesn't exist for this stock and user combination.\nCannot process trigger.\n"))
// 			return
// 		}
// 		utils.LogErr(err)
// 		w.Write([]byte("Error getting user data."))
// 		http.Error(w, err.Error(), http.StatusInternalServerError)
// 		return
// 	}

// 	// trigger already exists, return error
// 	if totalValue > 0 && triggerPriceDB > 0 {
// 		w.Write([]byte("SET BUY TRIGGER already exists for this stock and user combination.\nCancel current SET BUY and try again.\n"))
// 		return
// 	}

// 	err = dbactions.SetBuyTrigger(username, stock, orderType, triggerPrice)
// 	if err != nil {
// 		w.Write([]byte("Failed to SET BUY trigger.\n"))
// 		http.Error(w, err.Error(), http.StatusInternalServerError)
// 		return
// 	}

// 	w.Write([]byte("Successfully SET BUY trigger."))
// 	return
// }

// func setSellAmount(w http.ResponseWriter, r *http.Request) {
// 	vars := mux.Vars(r)
// 	username := vars["username"]
// 	symbol := vars["stock"]
// 	sellAmount, _ := strconv.ParseFloat(vars["amount"], 64)
// 	orderType := "sell"

// 	// confirm that user has enough valued stock
// 	// to complete sell

// 	_, userShares, err := dbutils.QueryUserStock(username, symbol)
// 	if err != nil {
// 		if err == sql.ErrNoRows {
// 			w.Write([]byte("User has no shares of this stock."))
// 			return
// 		}
// 		utils.LogErr(err)
// 		http.Error(w, err.Error(), http.StatusInternalServerError)
// 		return
// 	}

// 	_, _, totalValue, triggerPriceDB, err := dbutils.QueryUserStockTrigger(username, symbol, orderType)
// 	if totalValue > 0 || triggerPriceDB > 0 {
// 		w.Write([]byte("SET SELL AMOUNT already exists for this stock and user combination.\nCancel current SET SELL and try again.\n"))
// 		return
// 	}

// 	body, err := dbutils.QueryQuote(username, symbol)
// 	if err != nil {
// 		utils.LogErr(err)
// 		w.Write([]byte("Error getting stock quote.\n"))
// 		http.Error(w, err.Error(), http.StatusInternalServerError)
// 		return
// 	}

// 	quote, _ := strconv.ParseFloat(strings.Split(string(body), ",")[0], 64)
// 	balance := quote * float64(userShares)

// 	if balance < sellAmount {
// 		w.Write([]byte("Insufficent balance to sell stock."))
// 		return
// 	}

// 	err = dbactions.SetUserOrderTypeAmount(nil, username, symbol, orderType, sellAmount, nil)

// 	if err != nil {
// 		w.Write([]byte("Error setting SET SELL amount."))
// 		http.Error(w, err.Error(), http.StatusInternalServerError)
// 		return
// 	}

// 	w.Write([]byte("Successfully SET SELL amount"))
// 	return
// }

// func setSellTrigger(w http.ResponseWriter, r *http.Request) {
// 	vars := mux.Vars(r)
// 	username := vars["username"]
// 	stock := vars["stock"]
// 	triggerPrice := vars["triggerPrice"]
// 	orderType := "sell"

// 	_, _, totalValue, triggerPriceDB, err := dbutils.QueryUserStockTrigger(username, stock, orderType)
// 	if totalValue > 0 && triggerPriceDB > 0 {
// 		w.Write([]byte("SET SELL AMOUNT already exists for this stock and user combination.\nCancel current SET SELL and try again.\n"))
// 		return
// 	}

// 	if err != nil {
// 		if err == sql.ErrNoRows {
// 			w.Write([]byte("SET SELL AMOUNT doesn't exist for this stock and user combination.\nCannot process trigger.\n"))
// 			return
// 		}
// 		utils.LogErr(err)
// 		http.Error(w, err.Error(), http.StatusInternalServerError)
// 		return
// 	}

// 	triggerPriceFloat, _ := strconv.ParseFloat(triggerPrice, 64)

// 	err = dbactions.SetSellTrigger(username, stock, totalValue, triggerPriceFloat)

// 	if err != nil {
// 		http.Error(w, err.Error(), http.StatusInternalServerError)
// 		return
// 	}

// 	w.Write([]byte("Successfully SET SELL trigger."))
// 	return
// }

// func executeTrigger(w http.ResponseWriter, r *http.Request) {
// 	vars := mux.Vars(r)
// 	username := vars["username"]
// 	symbol := vars["stock"]
// 	shares := vars["shares"]
// 	triggerValue, _ := strconv.ParseFloat(vars["triggerValue"], 64)
// 	totalValue, _ := strconv.ParseFloat(vars["totalValue"], 64)
// 	orderType := vars["orderType"]

// 	err := dbactions.ExecuteTrigger(username, symbol, shares, totalValue, triggerValue, orderType)

// 	if err != nil {
// 		http.Error(w, err.Error(), http.StatusInternalServerError)
// 		return
// 	}

// 	res := []byte("Sucessfully executed SET " + orderType + " trigger.")
// 	w.Write(res)
// 	return
// }

// func cancelSetBuy(w http.ResponseWriter, r *http.Request) {
// 	vars := mux.Vars(r)
// 	username := vars["username"]
// 	symbol := vars["stock"]
// 	err := dbactions.CancelSetTrigger(username, symbol, "buy")

// 	if err != nil {
// 		http.Error(w, err.Error(), http.StatusInternalServerError)
// 		return
// 	}

// 	res := []byte("Successfully cancelled SET BUY\n")
// 	w.Write(res)
// }

// func cancelSetSell(w http.ResponseWriter, r *http.Request) {
// 	vars := mux.Vars(r)
// 	username := vars["username"]
// 	symbol := vars["stock"]
// 	err := dbactions.CancelSetTrigger(username, symbol, "sell")

// 	if err != nil {
// 		http.Error(w, err.Error(), http.StatusInternalServerError)
// 		return
// 	}

// 	res := []byte("Successfully cancelled SET SELL\n")
// 	w.Write(res)
// }


func logHandler(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := fmt.Sprintf("%s - %s%s", r.Method, r.Host, r.URL)
		// err := validateURLParams(r)
		// if err != nil {
		// 	utils.LogErr(err)
		// 	http.Error(w, err.Error(), http.StatusBadRequest)
		// 	return
		// }

		log.Println(l)
		fn(w, r)
	}
}

func main() {
	db = connectToDB()
	defer db.Close()

	dbactions.SetActionsDB(db)
	dbutils.SetUtilsDB(db)

	router := mux.NewRouter()
	// port, _ := freeport.GetFreePort()
	port := 8888

	
	router.HandleFunc("/api/clearUsers", logHandler(clearUsers))
	router.HandleFunc("/api/availableBalance/{username}", logHandler(availableBalance))


	router.HandleFunc("/api/add/{username}/{money}", logHandler(addUser))
	router.HandleFunc("/api/getQuote/{username}/{stock}", logHandler(getQuoute))

	router.HandleFunc("/api/buy/{username}/{symbol}/{amount}", logHandler(buyOrder))
	router.HandleFunc("/api/commitBuy/{username}", logHandler(commitBuy))
	router.HandleFunc("/api/cancelBuy/{username}", logHandler(cancelBuy))

	// router.HandleFunc("/api/sell/{username}/{stock}/{amount}", logHandler(sellOrder))
	// router.HandleFunc("/api/commitSell/{username}", logHandler(commitSell))
	// router.HandleFunc("/api/cancelSell/{username}", logHandler(cancelSell))

	// router.HandleFunc("/api/setBuyAmount/{username}/{stock}/{amount}", logHandler(setBuyAmount))
	// router.HandleFunc("/api/cancelSetBuy/{username}/{stock}", logHandler(cancelSetBuy))
	// router.HandleFunc("/api/setBuyTrigger/{username}/{stock}/{triggerPrice}", logHandler(setBuyTrigger))

	// router.HandleFunc("/api/setSellAmount/{username}/{stock}/{amount}", logHandler(setSellAmount))
	// router.HandleFunc("/api/cancelSetSell/{username}/{stock}", logHandler(cancelSetSell))
	// router.HandleFunc("/api/setSellTrigger/{username}/{stock}/{triggerPrice}", logHandler(setSellTrigger))

	// router.HandleFunc("/api/executeTrigger/{username}/{stock}/{shares}/{totalValue}/{triggerValue}/{orderType}", logHandler(executeTrigger))

	http.Handle("/", router)

	go triggermanager.Manage()

	if err := http.ListenAndServe(":"+strconv.Itoa(port), nil); err != nil {
		log.Fatal(err)
		panic(err)
	}

	log.Println("Running transaction server on port: " + strconv.Itoa(port))
}
