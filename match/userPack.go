package main

import (
	"./ssdb"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/henyouqian/lwutil"
)

const (
	H_USER_PACK            = "H_USER_PACK"        //key:H_USER_PACK subkey:userPackId value:userPack
	H_USER_PACK_EXTRA      = "H_USER_PACK_EXTRA"  //key:H_USER_PACK_EXTRA/userPackId, subkey:fieldKey, value:fieldValue
	Z_USER_PACK            = "Z_USER_PACK"        //key:Z_USER_PACK/userId subkey:userPackId score:userPackId
	Z_USER_PACK_LATEST     = "Z_USER_PACK_LATEST" //subkey:userPackId
	USER_PACK_SERIAL       = "USER_PACK_SERIAL"
	Z_USER_PACK_RANKS      = "Z_USER_PACK_RANKS"      //key:Z_USER_PACK_RANKS/userPackId subkey:userId
	H_USER_PACK_PLAY_TIMES = "H_USER_PACK_PLAY_TIMES" //subkey:userPackId
	H_USER_PACK_PLAY       = "H_USER_PACK_PLAY"       //key:H_USER_PACK_PLAY subKey:userPackId/userId

	//userPackFields
	FLD_USER_PACK_EXTRA_COUPON_REWARD = "extraCouponReward"
)

type UserPackRank struct {
	UserId   int64
	UserName string
	Score    int
}

type UserPack struct {
	Id           int64
	PackId       int64
	SliderNum    int
	Thumb        string
	CouponReward int
	BeginTime    int64
	BeginTimeStr string
	EndTime      int64
	Finished     bool
}

type UserPackPlay struct {
	UserPackId   int64
	UserId       int64
	HighScore    int
	FinalRank    int
	GameCoinNum  int
	Trys         int
	Team         string
	Secret       string
	SecretExpire int64
}

func makeZUserPackKey(userId int64) string {
	return fmt.Sprintf("%s/%d", Z_USER_PACK, userId)
}

func makeZUserPackRanksKey(userPackId int64) string {
	return fmt.Sprintf("%s/%d", Z_USER_PACK_RANKS, userPackId)
}

func makeHUserPackExtraKey(userPackId int64) string {
	return fmt.Sprintf("%s/%d", H_USER_PACK_EXTRA, userPackId)
}

func makeUserPackPlaySubKey(userPackId int64, userId int64) string {
	return fmt.Sprintf("%d/%d", userPackId, userId)
}

func getUserPackPlay(ssdbc *ssdb.Client, userPackPlaySubKey string) *UserPackPlay {
	resp, err := ssdbc.Do("hget", H_USER_PACK_PLAY, userPackPlaySubKey)
	var play UserPackPlay
	if len(resp) < 2 {
		play = UserPackPlay{
			UserPackId:  0,
			UserId:      0,
			HighScore:   0,
			FinalRank:   0,
			GameCoinNum: 3,
			Trys:        0,
			Team:        "",
		}
	} else {
		err = json.Unmarshal([]byte(resp[1]), &play)
		lwutil.CheckError(err, "")
	}
	return &play
}

func apiUserPackNew(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//ssdb
	ssdbc, err := ssdbPool.Get()
	checkError(err)
	defer ssdbc.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")
	userId := session.Userid

	//in
	var in struct {
		Pack
		SliderNum    int
		CouponReward int
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if in.SliderNum < 3 {
		in.SliderNum = 3
	} else if in.SliderNum > 9 {
		in.SliderNum = 9
	}

	stringLimit(&in.Title, 100)
	stringLimit(&in.Text, 2000)

	if in.CouponReward%100 != 0 {
		lwutil.SendError("err_coupon_reward", "in.CouponReward % 100 != 0")
	}

	//check crystal
	playerKey := makePlayerInfoKey(session.Userid)
	admin := isAdmin(session.Username)
	if !admin {
		crystalNum := getPlayerCrystal(ssdbc, playerKey)
		if crystalNum < in.CouponReward {
			lwutil.SendError("err_crystal", "crystalNum < in.CouponReward")
		}
	}

	//new pack
	newPack(ssdbc, &in.Pack, session.Userid)

	//new user pack
	userPackId := GenSerial(ssdbc, USER_PACK_SERIAL)
	now := lwutil.GetRedisTime()
	nowUnix := now.Unix()
	nowStr := now.Format("2006-01-02T15:04:05")
	endTime := now.Add(24 * time.Hour).Unix()
	userPack := UserPack{
		Id:           userPackId,
		PackId:       in.Pack.Id,
		SliderNum:    in.SliderNum,
		Thumb:        in.Thumb,
		CouponReward: in.CouponReward,
		BeginTime:    nowUnix,
		BeginTimeStr: nowStr,
		EndTime:      endTime,
		Finished:     false,
	}

	//json
	js, err := json.Marshal(userPack)
	lwutil.CheckError(err, "")

	//add to hash
	resp, err := ssdbc.Do("hset", H_USER_PACK, userPackId, js)
	lwutil.CheckSsdbError(resp, err)

	//add to zset
	zkey := makeZUserPackKey(userId)
	resp, err = ssdbc.Do("zset", zkey, userPackId, userPackId)
	lwutil.CheckSsdbError(resp, err)

	//add to Z_USER_PACK_LATEST
	resp, err = ssdbc.Do("zset", Z_USER_PACK_LATEST, userPackId, userPackId)
	lwutil.CheckSsdbError(resp, err)

	//decrease crystal
	if !admin {
		addPlayerCrystal(ssdbc, playerKey, -in.CouponReward)
	}

	//out
	lwutil.WriteResponse(w, userPack)
}

func apiUserPackDel(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//ssdb
	ssdbc, err := ssdbPool.Get()
	checkError(err)
	defer ssdbc.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var in struct {
		UserPackId int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//check owner
	zkey := makeZUserPackKey(session.Userid)
	if !isAdmin(session.Username) {
		resp, err := ssdbc.Do("zexists", zkey, in.UserPackId)
		lwutil.CheckError(err, "")
		if resp[0] == "0" {
			lwutil.SendError("err_owner", "not the pack's owner")
		}
	}

	//del
	resp, err := ssdbc.Do("zdel", zkey, in.UserPackId)
	lwutil.CheckSsdbError(resp, err)

	resp, err = ssdbc.Do("zdel", Z_USER_PACK_LATEST, in.UserPackId)
	lwutil.CheckSsdbError(resp, err)

	resp, err = ssdbc.Do("hdel", H_USER_PACK, in.UserPackId)
	lwutil.CheckSsdbError(resp, err)

	ranksKey := makeZUserPackRanksKey(in.UserPackId)
	resp, err = ssdbc.Do("zclear", ranksKey)
	lwutil.CheckSsdbError(resp, err)

	resp, err = ssdbc.Do("hdel", H_USER_PACK_PLAY_TIMES, in.UserPackId)
	lwutil.CheckSsdbError(resp, err)
}

func apiUserPackListMine(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//ssdb
	ssdbc, err := ssdbPool.Get()
	checkError(err)
	defer ssdbc.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var in struct {
		UserId  int64
		StartId int64
		Limit   int
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if in.Limit <= 0 {
		in.Limit = 20
	} else if in.Limit > 50 {
		in.Limit = 50
	}

	if in.StartId == 0 {
		in.StartId = math.MaxInt64
	}

	//get keys
	userId := session.Userid
	zkey := makeZUserPackKey(userId)
	resp, err := ssdbc.Do("zrscan", zkey, in.StartId, in.StartId, "", in.Limit)
	lwutil.CheckSsdbError(resp, err)

	if len(resp) == 1 {
		w.Write([]byte("[]"))
		return
	}
	resp = resp[1:]

	//get user packs
	num := len(resp) / 2
	args := make([]interface{}, num+2)
	args[0] = "multi_hget"
	args[1] = H_USER_PACK

	for i := 0; i < num; i++ {
		args = append(args, resp[i*2])
	}
	resp, err = ssdbc.Do(args...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	type UserPackOut struct {
		UserPack
		PlayTimes int
	}

	userPacks := make([]UserPackOut, len(resp)/2)
	m := make(map[int64]int) //key:userPackId, value:index
	for i, _ := range userPacks {
		packjs := resp[i*2+1]
		err = json.Unmarshal([]byte(packjs), &userPacks[i])
		lwutil.CheckError(err, "")
		m[userPacks[i].Id] = i
	}

	if len(userPacks) > 0 {
		args = make([]interface{}, len(userPacks)+2)
		args[0] = "multi_hget"
		args[1] = H_USER_PACK_PLAY_TIMES
		for _, v := range userPacks {
			args = append(args, v.Id)
		}
		resp, err = ssdbc.Do(args...)
		lwutil.CheckSsdbError(resp, err)
		resp = resp[1:]

		num := len(resp) / 2
		for i := 0; i < num; i++ {
			userPackId, err := strconv.ParseInt(resp[i*2], 10, 64)
			lwutil.CheckError(err, "")
			idx := m[userPackId]
			playTimes, err := strconv.ParseInt(resp[i*2+1], 10, 64)
			lwutil.CheckError(err, "")
			userPacks[idx].PlayTimes = int(playTimes)
		}
	}

	//out
	lwutil.WriteResponse(w, userPacks)
}

func apiUserPackListLatest(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//ssdb
	ssdbc, err := ssdbPool.Get()
	checkError(err)
	defer ssdbc.Close()

	//session
	_, err = findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var in struct {
		StartId int64
		Limit   int
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if in.Limit <= 0 {
		in.Limit = 20
	} else if in.Limit > 50 {
		in.Limit = 50
	}

	if in.StartId == 0 {
		in.StartId = math.MaxInt64
	}

	//get keys
	resp, err := ssdbc.Do("zrscan", Z_USER_PACK_LATEST, in.StartId, in.StartId, "", in.Limit)
	lwutil.CheckSsdbError(resp, err)

	if len(resp) == 1 {
		w.Write([]byte("[]"))
		return
	}
	resp = resp[1:]

	//get user packs
	num := len(resp) / 2
	args := make([]interface{}, num+2)
	args[0] = "multi_hget"
	args[1] = H_USER_PACK
	for i := 0; i < num; i++ {
		args = append(args, resp[i*2+1])
	}
	resp, err = ssdbc.Do(args...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	type UserPackOut struct {
		UserPack
		PlayTimes int
	}

	userPacks := make([]UserPackOut, len(resp)/2)
	m := make(map[int64]int) //key:userPackId, value:index
	for i, _ := range userPacks {
		packjs := resp[i*2+1]
		err = json.Unmarshal([]byte(packjs), &userPacks[i])
		lwutil.CheckError(err, "")
		m[userPacks[i].Id] = i
	}

	if len(userPacks) > 0 {
		args = make([]interface{}, len(userPacks)+2)
		args[0] = "multi_hget"
		args[1] = H_USER_PACK_PLAY_TIMES
		for _, v := range userPacks {
			args = append(args, v.Id)
		}
		resp, err = ssdbc.Do(args...)
		lwutil.CheckSsdbError(resp, err)
		resp = resp[1:]

		num := len(resp) / 2
		for i := 0; i < num; i++ {
			userPackId, err := strconv.ParseInt(resp[i*2], 10, 64)
			lwutil.CheckError(err, "")
			idx := m[userPackId]
			playTimes, err := strconv.ParseInt(resp[i*2+1], 10, 64)
			lwutil.CheckError(err, "")
			userPacks[idx].PlayTimes = int(playTimes)
		}
	}

	//out
	lwutil.WriteResponse(w, userPacks)
}

func apiUserPackGet(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//ssdb
	ssdbc, err := ssdbPool.Get()
	checkError(err)
	defer ssdbc.Close()

	//session
	_, err = findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var in struct {
		UserPackId int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	resp, err := ssdbc.Do("hget", H_USER_PACK, in.UserPackId)
	lwutil.CheckSsdbError(resp, err)

	userPack := UserPack{}
	err = json.Unmarshal([]byte(resp[1]), &userPack)
	lwutil.CheckError(err, "")

	//out
	lwutil.WriteResponse(w, userPack)
}

func apiUserPackPlayBegin(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var in struct {
		UserPackId int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//get userPack
	resp, err := ssdbc.Do("hget", H_USER_PACK, in.UserPackId)
	lwutil.CheckSsdbError(resp, err)
	var userPack UserPack
	err = json.Unmarshal([]byte(resp[1]), &userPack)
	lwutil.CheckError(err, "")
	now := lwutil.GetRedisTimeUnix()

	if now < userPack.BeginTime || now >= userPack.EndTime || userPack.Finished {
		lwutil.SendError("err_time", "userPack out of time")
	}

	//get userPackPlay
	userPackPlaySubKey := makeUserPackPlaySubKey(in.UserPackId, session.Userid)
	play := getUserPackPlay(ssdbc, userPackPlaySubKey)

	if play.UserPackId == 0 {
		play.UserPackId = in.UserPackId
	}
	if play.UserId == 0 {
		play.UserId = session.Userid
	}
	if play.Team == "" {
		playerInfo, err := getPlayerInfo(ssdbc, session.Userid)
		lwutil.CheckError(err, "")
		play.Team = playerInfo.TeamName
	}

	//game coin
	if play.GameCoinNum <= 0 {
		lwutil.SendError("err_game_coin", "")
	}
	play.GameCoinNum--

	//gen secret
	play.Secret = lwutil.GenUUID()
	play.SecretExpire = lwutil.GetRedisTimeUnix() + TRY_EXPIRE_SECONDS

	//update play
	js, err := json.Marshal(play)
	lwutil.CheckError(err, "")
	resp, err = ssdbc.Do("hset", H_USER_PACK_PLAY, userPackPlaySubKey, js)
	lwutil.CheckSsdbError(resp, err)

	//out
	lwutil.WriteResponse(w, play)
}

func apiUserPackPlayEnd(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var in struct {
		EventId  int64
		Secret   string
		Score    int
		Checksum string
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//checksum
	checksum := fmt.Sprintf("%s+%d9d7a", in.Secret, in.Score+8703)
	hasher := sha1.New()
	hasher.Write([]byte(checksum))
	checksum = hex.EncodeToString(hasher.Sum(nil))
	if in.Checksum != checksum {
		lwutil.SendError("err_checksum", "")
	}

	//check event record
	now := lwutil.GetRedisTimeUnix()
	recordKey := makeEventPlayerRecordSubkey(in.EventId, session.Userid)
	resp, err := ssdb.Do("hget", H_EVENT_PLAYER_RECORD, recordKey)
	lwutil.CheckError(err, "")
	if resp[0] != "ok" {
		lwutil.SendError("err_not_found", "event record not found")
	}

	record := EventPlayerRecord{}
	err = json.Unmarshal([]byte(resp[1]), &record)
	lwutil.CheckError(err, "")
	if record.Secret != in.Secret {
		lwutil.SendError("err_not_match", "Secret not match")
	}
	if now > record.SecretExpire {
		lwutil.SendError("err_expired", "secret expired")
	}

	//clear secret
	record.SecretExpire = 0

	//update score
	scoreUpdate := false
	if record.Trys == 1 || record.HighScore == 0 {
		record.HighScore = in.Score
		record.HighScoreTime = now
		scoreUpdate = true
	} else {
		if in.Score > record.HighScore {
			record.HighScore = in.Score
			scoreUpdate = true
		}
	}

	//save record
	jsRecord, err := json.Marshal(record)
	resp, err = ssdb.Do("hset", H_EVENT_PLAYER_RECORD, recordKey, jsRecord)
	lwutil.CheckSsdbError(resp, err)

	//redis
	rc := redisPool.Get()
	defer rc.Close()

	//event leaderboard
	eventLbLey := makeRedisLeaderboardKey(in.EventId)
	if scoreUpdate {
		_, err = rc.Do("ZADD", eventLbLey, record.HighScore, session.Userid)
		lwutil.CheckError(err, "")
	}

	//get rank
	rc.Send("ZREVRANK", eventLbLey, session.Userid)
	rc.Send("ZCARD", eventLbLey)
	err = rc.Flush()
	lwutil.CheckError(err, "")
	rank, err := redis.Int(rc.Receive())
	lwutil.CheckError(err, "")
	rankNum, err := redis.Int(rc.Receive())
	lwutil.CheckError(err, "")

	//recaculate team score
	if scoreUpdate && rank <= TEAM_SCORE_RANK_MAX {
		recaculateTeamScore(ssdb, rc, in.EventId)
	}

	//out
	out := struct {
		Rank    uint32
		RankNum uint32
	}{
		uint32(rank + 1),
		uint32(rankNum),
	}

	//out
	lwutil.WriteResponse(w, out)
}

func regUserPack() {
	http.Handle("/userPack/new", lwutil.ReqHandler(apiUserPackNew))
	http.Handle("/userPack/del", lwutil.ReqHandler(apiUserPackDel))
	http.Handle("/userPack/listMine", lwutil.ReqHandler(apiUserPackListMine))
	http.Handle("/userPack/listLatest", lwutil.ReqHandler(apiUserPackListLatest))
	http.Handle("/userPack/get", lwutil.ReqHandler(apiUserPackGet))

	http.Handle("/userPack/playBegin", lwutil.ReqHandler(apiUserPackPlayBegin))
	http.Handle("/userPack/playEnd", lwutil.ReqHandler(apiUserPackPlayEnd))
}
