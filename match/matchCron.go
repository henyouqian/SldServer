package main

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/golang/glog"
	"github.com/henyouqian/lwutil"
)

func _matchCronGlog() {
	glog.Info("")
}

func runMatchCron() {
	// _cron.AddFunc("0 * * * * *", matchCron)

	go func() {
		for true {
			matchCron()

			now := lwutil.GetRedisTime()
			s := 60 - now.Second() + 1
			if s < 10 {
				s += 60
			}
			time.Sleep(time.Duration(s) * time.Second)

		}
	}()

}

func calcRankPrize(match *Match, prizeSum int, rank int) int {
	rankIdx := rank - 1
	fixPrizeNum := len(match.RankPrizeProportions)
	if rankIdx < fixPrizeNum {
		return int(match.RankPrizeProportions[rankIdx] * float32(prizeSum))
	}

	minPrizeNum := int(float32(prizeSum) * match.MinPrizeProportion / float32(MIN_PRIZE))
	propNum := len(match.RankPrizeProportions)
	lastPrize := float32(prizeSum) * match.RankPrizeProportions[propNum-1]
	if (lastPrize > MIN_PRIZE) && (rankIdx < (fixPrizeNum + minPrizeNum)) {
		return MIN_PRIZE
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
		checkError(err)
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
				extraPrizeSubkey := makeHMatchExtraSubkey(matchId, MATCH_EXTRA_PRIZE)
				respPrize, err := ssdbc.Do("hget", H_MATCH_EXTRA, extraPrizeSubkey)
				checkError(err)
				extraPrize := 0
				if len(respPrize) == 2 {
					extraPrize, err = strconv.Atoi(respPrize[1])
					checkError(err)
				}
				prizeSum := match.Prize + extraPrize

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
						if play == nil {
							glog.Error("no play")
							continue
						}
						play.FinalRank = rank

						///get reward sum
						play.Prize = calcRankPrize(match, prizeSum, rank)

						js, err := json.Marshal(play)
						checkError(err)

						playSubKey := makeMatchPlaySubkey(matchId, userId)
						r, err := ssdbc.Do("hset", H_MATCH_PLAY, playSubKey, js)
						checkSsdbError(r, err)

						//save to H_MATCH_RANK
						matchRankKey := makeHMatchRankKey(matchId)
						r, err = ssdbc.Do("hset", matchRankKey, rank, userId)
						checkSsdbError(r, err)

						//add player prize
						addPrizeToCache(ssdbc, userId, matchId, match.Thumb, play.Prize, PRIZE_REASON_RANK, rank)

						//
						addEcoRecord(ssdbc, userId, play.Prize, ECO_FORWHAT_MATCHPRIZE)
					}
				}

				//owner prize
				ownerPrize := int(match.OwnerPrizeProportion * float32(prizeSum))
				if ownerPrize > 0 {
					addPrizeToCache(ssdbc, match.OwnerId, matchId, match.Thumb, ownerPrize, PRIZE_REASON_OWNER, 0)
					addEcoRecord(ssdbc, match.OwnerId, ownerPrize, ECO_FORWHAT_PUBLISHPRIZE)
				}

				//add to del array
				delMatchIds = append(delMatchIds, matchId)

				//del leaderboard redis
				_, err = rc.Do("DEL", lbKey)
				checkError(err)

				//save match
				match.HasResult = true
				js, err := json.Marshal(match)
				lwutil.CheckError(err, "")
				resp, err := ssdbc.Do("hset", H_MATCH, matchId, js)
				lwutil.CheckSsdbError(resp, err)
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
