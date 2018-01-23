package triggermanager

import (
	"time"

	"transaction-service/queries/utils"
)

const pollInterval = 5

func Manage() {
	for {
		time.Sleep(pollInterval)
		dbutils.QueryAndExecuteCurrentTriggers()
	}
}
