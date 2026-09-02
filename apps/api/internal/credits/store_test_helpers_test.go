package credits

import "fmt"

func walletSystemShard(userID int64) string {
	return fmt.Sprintf("wallet:%d", userID)
}
