package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"./queries/actions"
	"./queries/utils"
	"./utils"

	"github.com/gorilla/mux"
	// "github.com/phayes/freeport"
	_ "github.com/lib/pq"
)

var db *sql.DB

func connectToDB() *sql.DB {
	var (
		host     = "localhost"
		port     = 5432
		user     = os.Getenv("POSTGRES_USER")
		password = os.Getenv("POSTGRES_PASSWORD")
		dbname   = "transactions"
	)

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
		w.Write([]byte("Error getting quote."))
	}

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
				w.Write([]byte("Failed to add user " + username))
			} else {
				w.Write([]byte("Successfully added user " + username))
			}
			return
		}
		w.Write([]byte("Failed to add user " + username))
		return
	}

	//add money to existing user
	addMoneyFloat, err := strconv.ParseFloat(addMoney, 64)
	balance += addMoneyFloat
	balanceString := fmt.Sprintf("%f", balance)

	err = dbactions.UpdateUser(username, balanceString)

	if err != nil {
		w.Write([]byte("Failed to update user " + username))
	} else {
		w.Write([]byte("Successfully added user " + username))
	}

}

func buyOrder(w http.ResponseWriter, r *http.Request) {
	var balance float64
	const orderType string = "buy"

	vars := mux.Vars(r)
	username := vars["username"]
	stock := vars["stock"]
	buyAmount, _ := strconv.ParseFloat(vars["amount"], 64)

	_, balance, err := dbutils.QueryUser(username)

	if err != nil {
		if err == sql.ErrNoRows {
			w.Write([]byte("Invalid user."))
			return
		}
		utils.LogErr(err)
		w.Write([]byte("Error getting user data."))
		return
	}

	if balance < buyAmount {
		w.Write([]byte("Insufficent balance."))
		return
	}

	body, err := dbutils.QueryQuote(username, stock)
	if err != nil {
		utils.LogErr(err)
		if body != nil {
			w.Write([]byte("Error getting stock quote."))
		}
		w.Write([]byte("Error converting quote to string."))
	}

	quote, _ := strconv.ParseFloat(strings.Split(string(body), ",")[0], 64)
	buyUnits := int(buyAmount / quote)

	_, err = dbactions.AddReservation(username, stock, "buy", buyUnits, quote)

	if err != nil {
		utils.LogErr(err)
		w.Write([]byte("Error reserving stock."))
		return
	}

	w.Write([]byte("Buy order placed. You have 60 seconds to confirm your order; otherwise, it will be dropped."))
	go dbactions.RemoveOrder(username, stock, orderType, buyUnits, quote)
}

func commitBuy(w http.ResponseWriter, r *http.Request) {
	const orderType = "buy"
	var requestParams = mux.Vars(r)
	response := dbactions.CommitTransaction(requestParams["username"], orderType)
	w.Write(response)
}

func sellOrder(w http.ResponseWriter, r *http.Request) {
	const orderType = "sell"
	var userShares int

	vars := mux.Vars(r)
	username := vars["username"]
	stock := vars["stock"]
	sellAmount, _ := strconv.ParseFloat(vars["amount"], 64)

	_, userShares, err := dbutils.QueryUserStock(username, stock)

	if err != nil {
		if err == sql.ErrNoRows {
			w.Write([]byte("User has no shares of this stock."))
			return
		}
		utils.LogErr(err)
		w.Write([]byte("Error getting user stock data."))
	}

	body, err := dbutils.QueryQuote(username, stock)
	if err != nil {
		utils.LogErr(err)
		if body != nil {
			w.Write([]byte("Error getting stock quote."))
		}
		w.Write([]byte("Error converting quote to string."))
	}

	quote, _ := strconv.ParseFloat(strings.Split(string(body), ",")[0], 64)
	balance := quote * float64(userShares)

	if balance < sellAmount {
		w.Write([]byte("Insufficent balance to sell stock."))
		return
	}

	_, err = dbactions.AddReservation(username, stock, orderType, userShares, quote)

	if err != nil {
		utils.LogErr(err)
		w.Write([]byte("Error reserving stock."))
		return
	}

	w.Write([]byte("Sell order placed. You have 60 seconds to confirm your order; otherwise, it will be dropped."))
	go dbactions.RemoveOrder(username, stock, orderType, userShares, quote)
}

func commitSell(w http.ResponseWriter, r *http.Request) {
	const orderType = "sell"
	var requestParams = mux.Vars(r)
	response := dbactions.CommitTransaction(requestParams["username"], orderType)
	w.Write(response)
}

func setBuyAmount(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	username := vars["username"]
	stock := vars["stock"]
	buyAmount, _ := strconv.ParseFloat(vars["amount"], 64)
	orderType := "buy"

	_, userBalance, err := dbutils.QueryUser(username)

	if err != nil {
		if err == sql.ErrNoRows {
			w.Write([]byte("Invalid user."))
			return
		}
		utils.LogErr(err)
		w.Write([]byte("Error getting user data."))
		return
	}

	if userBalance < buyAmount {
		log.Printf("User balance: %f\nBuy amount: %f", buyAmount, userBalance)
		w.Write([]byte("Insufficent balance."))
		return
	}

	remainingBalance := userBalance - buyAmount
	res := dbactions.CommitSetBuyAmountTx(username, stock, orderType, remainingBalance, buyAmount)
	w.Write(res)
	return
}

func setBuyTrigger(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	username := vars["username"]
	stock := vars["stock"]
	triggerPrice := vars["triggerPrice"]

	_, _, err := dbutils.QueryUser(username)

	if err != nil {
		if err == sql.ErrNoRows {
			w.Write([]byte("Invalid user."))
			return
		}
		utils.LogErr(err)
		w.Write([]byte("Error getting user data."))
		return
	}

	res := dbactions.SetBuyTrigger(username, stock, triggerPrice)

	w.Write(res)
	return
}

func main() {
	db = connectToDB()
	defer db.Close()

	dbactions.SetActionsDB(db)
	dbutils.SetUtilsDB(db)

	router := mux.NewRouter()
	// port, _ := freeport.GetFreePort()
	port := 8888

	log.Println("Running transaction server on port: " + strconv.Itoa(port))

	router.HandleFunc("/api/getQuote/{username}/{stock}", getQuoute)
	router.HandleFunc("/api/addUser/{username}/{money}", addUser)
	router.HandleFunc("/api/buyOrder/{username}/{stock}/{amount}", buyOrder)
	router.HandleFunc("/api/commitBuy/{username}", commitBuy)
	router.HandleFunc("/api/sellOrder/{username}/{stock}/{amount}", sellOrder)
	router.HandleFunc("/api/commitSell/{username}", commitSell)

	router.HandleFunc("/api/setBuyAmount/{username}/{stock}/{amount}", setBuyAmount)
	router.HandleFunc("/api/setBuyTrigger/{username}/{stock}/{triggerPrice}", setBuyTrigger)

	// router.HandleFunc("/articles/{category}/{id:[0-9]+}", ArticleHandler)
	http.Handle("/", router)

	if err := http.ListenAndServe(":"+strconv.Itoa(port), nil); err != nil {
		log.Fatal(err)
		panic(err)
	}
}
