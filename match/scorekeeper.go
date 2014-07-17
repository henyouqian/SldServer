package main

import (
	// "./ssdb"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/garyburd/redigo/redis"
	"github.com/golang/glog"
	"github.com/henyouqian/lwutil"
	"runtime"
	"strconv"
	"time"
)

func init() {
	glog.Infoln("scorekeeper init")
}

func checkError(err error) {
	if err != nil {
		_, file, line, _ := runtime.Caller(1)
		e := fmt.Sprintf("[%s:%d]%v", file, line, err)
		panic(e)
	}
}

func checkSsdbError(resp []string, err error) {
	if resp[0] != "ok" {
		err = errors.New(fmt.Sprintf("ssdb error: %s", resp[0]))
	}
	if err != nil {
		_, file, line, _ := runtime.Caller(1)
		e := fmt.Sprintf("[%s:%d]%v", file, line, err)
		panic(e)
	}
}

func handleError() {
	if r := recover(); r != nil {
		_, file, line, _ := runtime.Caller(2)
		glog.Error(r, file, line)

		buf := make([]byte, 2048)
		runtime.Stack(buf, false)
		glog.Errorf("%s", buf)
	}
}

func calcReward(rank int) (reward int64) {
	if rank > 100 {
		return int64(100)
	}
	return int64((101 - rank) * 100)
}

func scoreKeeper() {
	defer handleError()

	//ssdb
	ssdb, err := ssdbPool.Get()
	checkError(err)
	defer ssdb.Close()

	//redis
	rc := redisPool.Get()
	defer rc.Close()

	//zrscan
	resp, err := ssdb.Do("zrscan", Z_EVENT, "", "", "", 20)
	checkSsdbError(resp, err)
	resp = resp[1:]
	if len(resp) == 0 {
		return
	}

	//multi_hget
	keyNum := len(resp) / 2
	cmds := make([]interface{}, keyNum+2)
	cmds[0] = "multi_hget"
	cmds[1] = H_EVENT
	for i := 0; i < keyNum; i++ {
		cmds[2+i] = resp[i*2]
	}
	eventResp, err := ssdb.Do(cmds...)
	checkSsdbError(eventResp, err)
	eventResp = eventResp[1:]

	eventNum := len(eventResp) / 2
	for i := 0; i < eventNum; i++ {
		var event Event
		err = json.Unmarshal([]byte(eventResp[i*2+1]), &event)
		checkError(err)

		now := lwutil.GetRedisTimeUnix()
		if event.HasResult || now < event.EndTime {
			continue
		}

		glog.Infof("event begin: id=%d", event.Id)

		//get ranks
		eventLbLey := makeRedisLeaderboardKey(event.Id)
		rankNum, err := redis.Int(rc.Do("ZCARD", eventLbLey))
		checkError(err)
		numPerBatch := 1000
		currRank := 1
		for iBatch := 0; iBatch < rankNum/numPerBatch+1; iBatch++ {
			offset := iBatch * numPerBatch
			values, err := redis.Values(rc.Do("ZREVRANGE", eventLbLey, offset, offset+numPerBatch-1, "WITHSCORES"))
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

				//set to event player record
				recordKey := fmt.Sprintf("%d/%d", event.Id, userId)
				resp, err := ssdb.Do("hget", H_EVENT_PLAYER_RECORD, recordKey)
				checkSsdbError(resp, err)
				record := EventPlayerRecord{}
				err = json.Unmarshal([]byte(resp[1]), &record)
				checkError(err)

				record.FinalRank = rank
				record.MatchReward = calcReward(rank)

				js, err := json.Marshal(record)
				checkError(err)

				resp, err = ssdb.Do("hset", H_EVENT_PLAYER_RECORD, recordKey, js)
				checkSsdbError(resp, err)

				//save to H_EVENT_RANK
				key := makeHashEventRankKey(event.Id)
				resp, err = ssdb.Do("hset", key, rank, userId)
				checkSsdbError(resp, err)

				//add player reward
				playerKey := makePlayerInfoKey(userId)
				resp, err = ssdb.Do("hincr", playerKey, playerRewardCache, record.MatchReward)
				checkSsdbError(resp, err)
				resp, err = ssdb.Do("hincr", playerKey, playerTotalReward, record.MatchReward)
				checkSsdbError(resp, err)

				//add to Z_EVENT_PLAYER_RECORD
				key = fmt.Sprintf("Z_EVENT_PLAYER_RECORD/%d", userId)
				resp, err = ssdb.Do("zset", key, event.Id, event.Id)
				checkSsdbError(resp, err)
			}
		}

		//bet
		//recalc team score
		scoreMap := recaculateTeamScore(ssdb, rc, event.Id)
		if scoreMap == nil {
			glog.Errorln("scoreMap == nil. eventId=%d", event.Id)
			continue
		}

		//calc winning teams
		maxScore := 0
		winTeams := make([]string, 0, 10)
		for teamName, score := range scoreMap {
			if score > 0 {
				if score > maxScore {
					winTeams = winTeams[:1]
					winTeams[0] = teamName
					score = maxScore
				} else if score == maxScore {
					winTeams = append(winTeams, teamName)
				}
			}
		}

		//calc betting pool reward sum
		key := makeEventBettingPoolKey(event.Id)
		bettingPool := map[string]int64{}
		err = ssdb.HGetMapAll(key, bettingPool)
		checkError(err)

		betMoneySum := int64(0)
		winTeamsMoneySum := int64(0)
		for team, money := range bettingPool {
			betMoneySum += money

			for _, winTeam := range winTeams {
				if team == winTeam {
					winTeamsMoneySum += money
					break
				}
			}
		}

		if winTeamsMoneySum == 0 {
			glog.Errorln("winTeamsMoneySum == 0")
			continue
		}

		winMult := float32(betMoneySum) / float32(winTeamsMoneySum)

		//bet reward
		for _, team := range winTeams {
			key = makeEventTeamPlayerBetKey(event.Id, team)

			subKey := ""
			for true {
				resp, err := ssdb.Do("hscan", key, subKey, "", 100)
				if err != nil || resp[0] != "ok" {
					glog.Errorln(err, resp[0])
					break
				}
				resp = resp[1:]
				num := len(resp) / 2

				if num == 0 {
					break
				}

				for i := 0; i < num; i++ {
					playerIdStr := resp[2*i]
					playerBetStr := resp[2*i+1]
					playerId, err := strconv.ParseInt(playerIdStr, 10, 64)
					checkError(err)
					playerBet, err := strconv.ParseInt(playerBetStr, 10, 64)
					checkError(err)

					//add reward to player
					playerKey := makePlayerInfoKey(playerId)
					addMoney := int64(float32(playerBet) * winMult)
					resp, err = ssdb.Do("hincr", playerKey, playerRewardCache, addMoney)

					key := makeEventPlayerRecordSubkey(event.Id, playerId)
					resp, err := ssdb.Do("hget", H_EVENT_PLAYER_RECORD, key)
					checkSsdbError(resp, err)
					var record EventPlayerRecord
					err = json.Unmarshal([]byte(resp[1]), &record)
					checkError(err)
					record.BetReward = addMoney

					js, err := json.Marshal(record)
					checkError(err)
					resp, err = ssdb.Do("hset", H_EVENT_PLAYER_RECORD, key, js)
					checkSsdbError(resp, err)

					//
					subKey = playerIdStr
				}

			}
		}

		//event finished
		event.HasResult = true
		jsEvent, err := json.Marshal(event)
		checkError(err)
		resp, err = ssdb.Do("hset", H_EVENT, event.Id, jsEvent)
		checkSsdbError(resp, err)

		//del redis leaderboard
		_, err = rc.Do("del", eventLbLey)
		checkError(err)

		glog.Infof("event end")
	}
}

func scoreKeeperMain() {
	glog.Info("scorekeeper start")
	for true {
		go scoreKeeper()

		//sleep to next minute
		now := time.Now()

		t := now.Add(time.Minute + time.Second)
		to := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, time.Local)
		dt := to.Sub(now)
		//dt = time.Second
		time.Sleep(dt)
	}
}
