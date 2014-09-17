package main

import (
	"encoding/json"
	"strconv"

	"github.com/garyburd/redigo/redis"
	"github.com/golang/glog"
	"github.com/henyouqian/lwutil"
)

func _matchCronGlog() {
	glog.Info("")
}

func initMatchCron() {
	_cron.AddFunc("0 * * * * *", matchCron)
	matchCron()
}

func calcMatchReward(matchId int64, rank int) int {

	return 0
}

func matchCron() {
	defer handleError()

	//ssdb
	ssdbc, err := ssdbPool.Get()
	checkError(err)
	defer ssdbc.Close()

	//redis
	rc := redisPool.Get()
	defer rc.Close()

	//
	matchId := int64(0)
	endTime := int64(0)
	limit := 100

	looping := true
	for looping {
		resp, err := ssdbc.Do("zscan", Z_OPEN_MATCH, matchId, endTime, "", limit)

		checkSsdbError(resp, err)
		if len(resp) == 1 {
			break
		}
		resp = resp[1:]
		glog.Info(resp)

		num := len(resp) / 2
		for i := 0; i < num; i++ {
			endTime, err = strconv.ParseInt(resp[i*2+1], 10, 64)
			checkError(err)
			now := lwutil.GetRedisTimeUnix()
			if now >= endTime {
				matchId, err = strconv.ParseInt(resp[i*2], 10, 64)
				checkError(err)
				glog.Info(matchId)

				//get ranks
				lbKey := makeMatchLeaderboardRdsKey(matchId)
				rankNum, err := redis.Int(rc.Do("ZCARD", lbKey))
				checkError(err)
				numPerBatch := 1000
				currRank := 1
				for iBatch := 0; iBatch < rankNum/numPerBatch+1; iBatch++ {
					offset := iBatch * numPerBatch
					values, err := redis.Values(rc.Do("ZREVRANGE", lbKey, offset, offset+numPerBatch-1, "WITHSCORES"))
					checkError(err)

					num := len(values) / 2
					if num == 0 {
						continue
					}

					for i := 0; i < num; i++ {
						rank := currRank
						currRank++
						userId, err := redis.Int64(values[i*2], nil)
						checkError(err)
						// score, err := redisInt32(values[i*2+1], nil)
						// checkError(err)

						//set to matchPlay
						play := getMatchPlay(ssdbc, matchId, userId)
						play.FinalRank = rank
						play.Reward = calcMatchReward(matchId, rank)

						js, err := json.Marshal(play)
						checkError(err)

						playSubKey := makeMatchPlaySubkey(matchId, userId)
						r, err := ssdbc.Do("hset", H_MATCH_PLAY, playSubKey, js)
						checkSsdbError(r, err)

						//save to H_MATCH_RANK
						matchRankKey := makeHMatchRankKey(matchId)
						r, err = ssdbc.Do("hset", matchRankKey, rank, userId)
						checkSsdbError(r, err)

						//add player reward
						playerKey := makePlayerInfoKey(userId)
						r, err = ssdbc.Do("hincr", playerKey, PLAYER_REWARD_CACHE, play.Reward)
						checkSsdbError(r, err)

						//fixme: remove from Z_OPEN_MATCH and Z_HOT_MATCH

					}
				}
			} else {
				glog.Info("xxxxx", matchId)
				looping = false
				break
			}
		}
	}

}
