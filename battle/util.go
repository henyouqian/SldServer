package main

import (
	"encoding/base64"
	"fmt"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/henyouqian/ssdbgo"
	"github.com/nu7hatch/gouuid"
)

const (
	H_PLAYER_INFO = "H_PLAYER_INFO" //key:H_PLAYER_INFO/<userId> subkey:property

	PLAYER_GOLD_COIN              = "GoldCoin"
	PLAYER_BATTLE_POINT           = "BattlePoint"
	PLAYER_BATTLE_WIN_STREAK      = "BattleWinStreak"
	PLAYER_BATTLE_WIN_STREAK_MAX  = "BattleWinStreakMax"
	PLAYER_BATTLE_HEART_ZERO_TIME = "BattleHeartZeroTime"

	BATTLE_HEART_TOTAL   = 10
	BATTLE_HEART_ADD_SEC = 60 * 5

	SSDB_AUTH_PORT  = 9875
	SSDB_MATCH_PORT = 9876

	REDIS_HOST = "localhost:6379"
)

var (
	redisPool     *redis.Pool
	ssdbMatchPool *ssdbgo.Pool
	ssdbAuthPool  *ssdbgo.Pool
)

func initRedisAndSsdb() {
	redisPool = &redis.Pool{
		MaxIdle:     20,
		MaxActive:   0,
		IdleTimeout: 240 * time.Second,
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", REDIS_HOST)
			if err != nil {
				return nil, err
			}
			return c, err
		},
	}
	ssdbAuthPool = ssdbgo.NewPool("localhost", SSDB_AUTH_PORT, 10, 60)
	ssdbMatchPool = ssdbgo.NewPool("localhost", SSDB_MATCH_PORT, 10, 60)
}

func genUUID() string {
	uuid, _ := uuid.NewV4()
	return base64.URLEncoding.EncodeToString(uuid[:])
}

func makePlayerInfoKey(userId int64) string {
	return fmt.Sprintf("%s/%d", H_PLAYER_INFO, userId)
}

func getBattleHeartNum(playerInfo *PlayerInfo) int {
	dt := time.Now().Unix() - playerInfo.BattleHeartZeroTime
	heartNum := int(dt) / BATTLE_HEART_ADD_SEC
	if heartNum > BATTLE_HEART_TOTAL {
		heartNum = BATTLE_HEART_TOTAL
	}
	return heartNum
}
