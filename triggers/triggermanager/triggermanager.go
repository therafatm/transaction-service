package triggermanager

import (
	"log"
	"time"

	"common/logging"
	"transaction_service/queries/transdb"

	"github.com/go-redis/redis"
)

type Env struct {
	logger     logging.Logger
	tdb        transdb.TransactionDataStore
	quoteCache *redis.Client
}

func (env *Env) manageTriggers() {
	const pollInterval = 200
	for {
		time.Sleep(time.Millisecond * pollInterval)
		env.tdb.QueryAndExecuteCurrentTriggers(env.quoteCache, "1")
	}
}

func main() {
	logger := logging.NewLoggerConnection()
	tdb := transdb.NewTransactionDBConnection()
	quoteCache := transdb.NewQuoteCacheConnection()

	defer tdb.DB.Close()
	defer quoteCache.Close()

	env := &Env{quoteCache: quoteCache, logger: logger, tdb: tdb}

	go env.manageTriggers()

	log.Println("Running trigger manager started.")
}
