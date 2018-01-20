package main

import (
    "net/http"
    "log"
    "fmt"
    "os"
    "runtime"
    "database/sql"
    "strconv"
    "io/ioutil"
    "strings"
    "time"

    "github.com/gorilla/mux"
    // "github.com/phayes/freeport"
    _ "github.com/lib/pq"
)

const QUOTE_SERVER_PORT = 8000
var db *sql.DB

func checkErr(err error) {
    if err != nil {
        logErr(err)
        log.Fatal(err)
    }
}

func logErr(err error) {
    _, fn, line, _ := runtime.Caller(1)
    log.Printf("[error] %s:%d %v", fn, line, err)
}

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
    checkErr(err)

    return db
}

func getQuoteServerURL() string {
    if os.Getenv("GO_ENV") == "dev" {
        port := strconv.Itoa(QUOTE_SERVER_PORT)
        return string("http://localhost:" + port)
    }

    return string("http:quoteserve.seng:4444")
}

func queryQuote(username string, stock string) (body []byte, err error){
    URL := getQuoteServerURL()
    res, err := http.Get(URL + "/api/getQuote/" + username + "/" + stock )
    
    if err != nil {
        logErr(err)
    } else {
        body, err = ioutil.ReadAll(res.Body)
    }

    return
}

func queryUser(username string) (uid string, balance float64, err error) {
    query := "SELECT uid, money FROM users WHERE username = $1"
    err = db.QueryRow(query, username).Scan(&uid, &balance)
    return
}

func updateUserMoney(tx *sql.Tx, username string, money float64, orderType string) (err error) {
    _, balance, err := queryUser(username)
    
    if err != nil {
        logErr(err)
        return
    }

    if strings.Compare(orderType, "buy") == 0 {
        balance -= money
    } else {
        balance += money
    }

    if err != nil {
        logErr(err)
        return  
    }
    
    query := "UPDATE users SET money=$1 WHERE username=$2"
    _, err = tx.Exec(query, balance, username)

    return
}

func queryUserStock(username string, symbol string) (string, int, error) {
    var sid string
    var shares int
    query := "SELECT sid, shares FROM stocks WHERE username = $1 AND symbol = $2"
    err := db.QueryRow(query, username, symbol).Scan(&sid, &shares)
    return sid, shares, err
}


func updateUserStock(tx *sql.Tx, username string, symbol string, shares int, orderType string) (err error) {
    var query string
    _, currentShares, err := queryUserStock(username, symbol)

    if err != nil {
        if err == sql.ErrNoRows {
            query := "INSERT INTO stocks(username,symbol,shares) VALUES($1,$2,$3)"
            _, err = tx.Exec(query, username, symbol, shares)
            return
        } else {
            logErr(err)
            return
        }
    }

    if strings.Compare(orderType, "buy") == 0 {
        currentShares += shares
    } else {
        currentShares -= shares
    }

    if currentShares > 0 {
        query = "UPDATE stocks SET shares=$1 WHERE username=$2 AND symbol=$3"
        _, err = tx.Exec(query, currentShares, username, symbol)
    } else {
        query = "DELETE FROM stocks WHERE username=$1 AND symbol=$2"
        _, err = tx.Exec(query, username, symbol)
    }
    
    return
}

func getLastReservation(username string, orderType string)(string, int, float64, error){
    var symbol string
    var shares int
    var face_value float64

    query := "SELECT symbol, shares, face_value FROM reservations WHERE username=$1 and type=$2 ORDER BY (time) DESC LIMIT 1"
    err := db.QueryRow(query, username, orderType).Scan(&symbol, &shares, &face_value)
    return symbol, shares, face_value, err
}

func addReservation(username string, stock string, orderType string, shares int, face_value float64) (res sql.Result, err error){
    // time in seconds
    time := time.Now().Unix()
    query := "INSERT INTO reservations(username, symbol, type, shares, face_value, time) VALUES($1,$2,$3,$4,$5,$6)"
    res, err = db.Exec(query, username, stock, orderType, shares, face_value, time)
    return
}

func removeReservation(tx *sql.Tx, username string, stock string, reservationType string, shares int, face_value float64) (err error){
    query := "DELETE FROM reservations WHERE username=$1 AND symbol=$2 AND shares=$3 AND face_value=$4 AND type=$5"
    if tx == nil {
        _, err = db.Exec(query, username, stock, shares, face_value, reservationType)
    } else {
        _, err = tx.Exec(query, username, stock, shares, face_value, reservationType)
    }
    return
}

func removeOrder(username string, stock string, reservationType string, shares int, face_value float64){
    time.Sleep(60 * time.Second)
    err := removeReservation(nil, username, stock, reservationType, shares, face_value)
    if err != nil {
        log.Println("Error removing reservation due to timeout.")
        logErr(err)
    }
}

func getQuoute(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    body, err := queryQuote(vars["username"], vars["stock"])

    if err != nil {
         w.Write([]byte("Error getting quote."))
    }

    w.Write([]byte(body))
}

func addUser(w http.ResponseWriter, r *http.Request) {
    var uid string
    var balance float64
    vars := mux.Vars(r)
    username := vars["username"]
    addMoney := vars["money"]

    uid, balance, err := queryUser(username)

    if err == sql.ErrNoRows {
        //add new user
        query := "INSERT INTO users(username, money) VALUES($1,$2)"
        _, err := db.Exec(query, username, addMoney)
        if err != nil {
            log.Fatal(err)
        } else {
            w.Write([]byte("Successfully added user " + username))
        }
    } else {
        //add money to existing user
        fmt.Println(balance)
        addMoney, err := strconv.ParseFloat(addMoney, 64)

        balance += addMoney
        fmt.Println(balance)
        query := "UPDATE users SET money = $1 WHERE uid = $2"
        _, err = db.Exec(query, balance, uid)
        if err != nil {
            logErr(err)
            return
        } else {
            w.Write([]byte("Successfully updated balance for " + username))
        }       
    }
}

func buyOrder(w http.ResponseWriter, r *http.Request) {
    var balance float64
    const orderType string = "buy"

    vars := mux.Vars(r)
    username := vars["username"]
    stock := vars["stock"]
    buyAmount, _ := strconv.ParseFloat(vars["amount"], 64)

    _, balance, err := queryUser(username)

    if err != nil {
        if err == sql.ErrNoRows {
            w.Write([]byte("Invalid user."))
            return
        }
        logErr(err)
        w.Write([]byte("Error getting user data."))
    }

    if balance < buyAmount {
        w.Write([]byte("Insufficent balance."))
        return
    }

    body, err := queryQuote(username, stock)
    if err != nil {
        logErr(err)
        if body != nil {
            w.Write([]byte("Error getting stock quote."))
        } 
        w.Write([]byte("Error converting quote to string."))
    } 

    quote, _ := strconv.ParseFloat(strings.Split(string(body), ",")[0],64)

    buyUnits := int(buyAmount/quote)
   
    _, err = addReservation(username, stock, "buy", buyUnits, quote)

    if err != nil {
        logErr(err)
        w.Write([]byte("Error reserving stock."))
        return
    }

    w.Write([]byte("Buy order placed. You have 60 seconds to confirm your order; otherwise, it will be dropped."))
    go removeOrder(username, stock, orderType, buyUnits, quote)
}

func commitBuy(w http.ResponseWriter, r *http.Request){
    const orderType = "buy"
    commitTransaction(w, r, orderType)
}

func sellOrder(w http.ResponseWriter, r *http.Request) {
    const orderType = "sell"
    var userShares int

    vars := mux.Vars(r)
    username := vars["username"]
    stock := vars["stock"]
    sellAmount, _ := strconv.ParseFloat(vars["amount"], 64)

    _, userShares, err := queryUserStock(username, stock)

    if err != nil {
        if err == sql.ErrNoRows {
            w.Write([]byte("User has no shares of this stock."))
            return
        }
        logErr(err)
        w.Write([]byte("Error getting user stock data."))
    }

    body, err := queryQuote(username, stock)
    if err != nil {
        logErr(err)
        if body != nil {
            w.Write([]byte("Error getting stock quote."))
        } 
        w.Write([]byte("Error converting quote to string."))
    } 

    quote, _ := strconv.ParseFloat(strings.Split(string(body), ",")[0],64)
    balance := quote * float64(userShares)

    if balance < sellAmount {
        w.Write([]byte("Insufficent balance to sell stock."))
        return
    }
   
    _, err = addReservation(username, stock, orderType, userShares, quote)

    if err != nil {
        logErr(err)
        w.Write([]byte("Error reserving stock."))
        return
    }

    w.Write([]byte("Sell order placed. You have 60 seconds to confirm your order; otherwise, it will be dropped."))
    go removeOrder(username, stock, orderType, userShares, quote)
}

func commitSell(w http.ResponseWriter, r *http.Request){
    const orderType = "sell"
    commitTransaction(w, r, orderType)
}

func commitTransaction(w http.ResponseWriter, r *http.Request, orderType string) {
    var symbol string
    var shares int
    var face_value float64

    vars := mux.Vars(r)
    username := vars["username"]
    symbol, shares, face_value, err := getLastReservation(username, orderType)
    
    if err != nil {
        logErr(err)
        w.Write([]byte("Error retrieving reservation."))
        return        
    }

    amount := float64(shares) * face_value

    tx, err := db.Begin()
    err = updateUserMoney(tx, username, amount, orderType)
    if err != nil {
        logErr(err)
        tx.Rollback()
        w.Write([]byte("Error updating user."))
        return         
    }

    
    err = updateUserStock(tx, username, symbol, shares, orderType)
    if err != nil {
        logErr(err)
        tx.Rollback()
        w.Write([]byte("Error updating user stock."))
        return         
    }

    err = removeReservation(tx, username, symbol, orderType, shares, face_value)
    if err != nil {
        logErr(err)
        tx.Rollback()
        w.Write([]byte("Error updating reservation."))
        return         
    }

    err = tx.Commit()
    if err != nil {
        logErr(err)
        tx.Rollback()
        w.Write([]byte("Error committing transaction."))
        return  
    }

    w.Write([]byte("Sucessfully comitted transaction."))
}

func main() {
    db = connectToDB()
    defer db.Close()

    router :=  mux.NewRouter()
    // port, _ := freeport.GetFreePort()
    port := 8888

    log.Println("Running transaction server on port: " + strconv.Itoa(port))

    router.HandleFunc("/api/getQuote/{username}/{stock}", getQuoute)
    router.HandleFunc("/api/addUser/{username}/{money}", addUser)
    router.HandleFunc("/api/buyOrder/{username}/{stock}/{amount}", buyOrder)
    router.HandleFunc("/api/commitBuy/{username}", commitBuy)
    router.HandleFunc("/api/commitSell/{username}", commitSell)
    router.HandleFunc("/api/sellOrder/{username}/{stock}/{amount}", sellOrder)

    // router.HandleFunc("/articles/{category}/{id:[0-9]+}", ArticleHandler)
    http.Handle("/", router)

    if err := http.ListenAndServe(":" + strconv.Itoa(port), nil); err != nil {
        log.Fatal(err)
        panic(err)
    }
}
