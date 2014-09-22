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

func calcRankReward(match *Match, rewardSum int, rank int) int {
	fSum := float32(rewardSum)

	// //owner reward
	// ownerReward := int(match.OwnerRewardProportion * fSum)
	// playerKey := makePlayerInfoKey(userId)

	rankIdx := rank - 1
	fixRewardNum := len(match.RankRewardProportions)
	if rankIdx < fixRewardNum {
		return int(match.RankRewardProportions[rankIdx] * fSum)
	}

	oneCoinNum := int(fSum * match.OneCoinRewardProportion)
	if rankIdx < (fixRewardNum + oneCoinNum) {
		return 1
	}

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

	delMatchIds := make([]int64, 0, 16)
	looping := true
	for looping {
		resp, err := ssdbc.Do("zscan", Z_OPEN_MATCH, matchId, endTime, "", limit)

		checkSsdbError(resp, err)
		if len(resp) == 1 {
			break
		}
		resp = resp[1:]

		num := len(resp) / 2

		//for each match
		for i := 0; i < num; i++ {
			endTime, err = strconv.ParseInt(resp[i*2+1], 10, 64)
			checkError(err)
			now := lwutil.GetRedisTimeUnix()
			if now >= endTime {
				matchId, err = strconv.ParseInt(resp[i*2], 10, 64)
				checkError(err)

				//reward sum
				match := getMatch(ssdbc, matchId)
				extraRewardSubkey := makeHMatchExtraSubkey(matchId, MATCH_EXTRA_COUPON_REWARD)
				respCoupon, err := ssdbc.Do("hget", H_MATCH_EXTRA, extraRewardSubkey)
				checkError(err)
				extraReward := 0
				if len(respCoupon) == 2 {
					extraReward, err = strconv.Atoi(respCoupon[1])
					checkError(err)
				}
				rewardSum := match.CouponReward + extraReward

				//get ranks
				lbKey := makeMatchLeaderboardRdsKey(matchId)
				rankNum, err := redis.Int(rc.Do("ZCARD", lbKey))
				checkError(err)
				numPerBatch := 1000
				currRank := 1

				//for each rank batch
				for iBatch := 0; iBatch < rankNum/numPerBatch+1; iBatch++ {
					offset := iBatch * numPerBatch
					values, err := redis.Values(rc.Do("ZREVRANGE", lbKey, offset, offset+numPerBatch-1, "WITHSCORES"))
					checkError(err)

					num := len(values) / 2
					if num == 0 {
						// continue
						break
					}

					//for each rank
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

						///get reward sum
						play.Reward = calcRankReward(match, rewardSum, rank)

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
						addGcRewardToCache(ssdbc, userId, matchId, play.Reward, REWARD_REASON_RANK)

						//add to del array
						delMatchIds = append(delMatchIds, matchId)
					}
				}

				//owner reward
				ownerReward := int(match.OwnerRewardProportion * float32(rewardSum))
				if ownerReward > 0 {
					addGcRewardToCache(ssdbc, match.OwnerId, matchId, ownerReward, REWARD_REASON_OWNER)
				}
			} else {
				looping = false
				break
			}
		}
	}

	//del from Z_OPEN_MATCH and Z_HOT_MATCH
	if len(delMatchIds) > 0 {
		cmds := make([]interface{}, 0, 10)
		cmds = append(cmds, "multi_zdel")
		cmds = append(cmds, Z_OPEN_MATCH)
		for _, v := range delMatchIds {
			cmds = append(cmds, v)
		}
		resp, err := ssdbc.Do(cmds...)
		lwutil.CheckSsdbError(resp, err)

		cmds = make([]interface{}, 0, 10)
		cmds = append(cmds, "multi_zdel")
		cmds = append(cmds, Z_HOT_MATCH)
		for _, v := range delMatchIds {
			cmds = append(cmds, v)
		}
		resp, err = ssdbc.Do(cmds...)
		lwutil.CheckSsdbError(resp, err)
	}

}