package triggermanager

import (
	"time"

	"../../queries/utils"
)

const pollInterval = 5

func Manage() {
	for {
		time.Sleep(pollInterval)
		dbutils.QueryAndExecuteCurrentTriggers()
	}
}
