package auth

import "strconv"

const TokenRedisKeyPrefix = "token:"

func TokenRedisKey(userID uint64) string {
	return TokenRedisKeyPrefix + strconv.FormatUint(userID, 10)
}
