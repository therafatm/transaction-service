package triggermanager

import (
	"time"

	"transaction_service/queries/utils"
)

const pollInterval = 5

func Manage() {
	for {
		time.Sleep(pollInterval)
		dbutils.QueryAndExecuteCurrentTriggers()
	}
}
