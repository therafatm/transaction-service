package triggermanager

import (
	"time"

	"../queries/utils"
)

func Manage() {
	// for {
	time.Sleep(1)
	dbutils.QueryAndExecuteCurrentTriggers()
	// }
}
