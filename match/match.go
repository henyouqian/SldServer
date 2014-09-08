package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/golang/glog"
	"github.com/henyouqian/lwutil"
)

const (
	MATCH_SERIAL    = "MATCH_SERIAL"
	H_MATCH         = "H_MATCH"         //subkey:matchId value:matchJson
	H_MATCH_EXTRA   = "H_MATCH_EXTRA"   //key:H_MATCH_EXTRA subkey:matchId/fieldKey value:fieldValue
	Z_MATCH         = "Z_MATCH"         //subkey:matchId score:beginTime
	Z_PENDING_MATCH = "Z_PENDING_MATCH" //subkey:matchId score:beginTime
	Z_PLAYER_MATCH  = "Z_PLAYER_MATCH"  //key:Z_PLAYER_MATCH/userId subkey:matchId score:beginTime
)

type Match struct {
	Id                      int64
	PackId                  int64
	OwnerId                 int64
	SliderNum               int
	Thumb                   string
	CouponReward            int
	BeginTime               int64
	BeginTimeStr            string
	EndTime                 int64
	HasResult               bool
	RankRewardProportions   []float32
	LuckyRewardProportion   float32
	OneCoinRewardProportion float32
	OwnerRewardProportion   float32
	ChallengeSeconds        int
}

type MatchExtra struct {
	PlayTimes         int
	ExtraCouponReward int
}

const (
	MATCH_EXTRA_PLAY_TIMES    = "PlayTimes"
	MATCH_EXTRA_COUPON_REWARD = "ExtraCouponReward"
)

const (
	MATCH_LUCKY_REWARD_PROPORTION    = float32(0.15)
	MATCH_ONE_COIN_REWARD_PROPORTION = float32(0.05)
	MATCH_OWNER_REWARD_PROPORTION    = float32(0.1)
)

var (
	MATCH_RANK_REWARD_PROPORTIONS = []float32{
		0.15, 0.09, 0.08, 0.07, 0.06, 0.05, 0.04, 0.03, 0.02, 0.01,
		0.01, 0.01, 0.01, 0.01, 0.01, 0.01, 0.01, 0.01, 0.01, 0.01,
	}
)

func matchGlog() {
	glog.Info("")
}

func makeHMatchExtraSubkey(matchId int64, fieldKey string) string {
	return fmt.Sprintf("%d/%s", matchId, fieldKey)
}

func makeZPlayerMatchKey(userId int64) string {
	return fmt.Sprintf("%s/%d", Z_PLAYER_MATCH, userId)
}

func init() {
	//check reward
	sum := float32(0)
	for _, v := range MATCH_RANK_REWARD_PROPORTIONS {
		sum += v
	}
	sum += MATCH_LUCKY_REWARD_PROPORTION
	sum += MATCH_ONE_COIN_REWARD_PROPORTION
	sum += MATCH_OWNER_REWARD_PROPORTION
	if sum > 1.0 {
		panic("reward sum > 1.0")
	}
}

func apiMatchNew(w http.ResponseWriter, r *http.Request) {
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
		Pack
		BeginTime        string
		SliderNum        int
		CouponReward     int
		ChallengeSeconds int
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

	//check gold coin
	playerKey := makePlayerInfoKey(session.Userid)
	admin := isAdmin(session.Username)
	if !admin {
		goldNum := getPlayerGoldCoin(ssdbc, playerKey)
		if goldNum < in.CouponReward {
			lwutil.SendError("err_gold_coin", "goldNum < in.CouponReward")
		}
	}

	//new pack
	newPack(ssdbc, &in.Pack, session.Userid)

	//new match
	matchId := GenSerial(ssdbc, MATCH_SERIAL)
	beginTimeUnix := int64(0)
	beginTimeStr := in.BeginTime
	endTimeUnix := int64(0)
	isPublishNow := false
	if in.BeginTime == "" {
		beginTime := lwutil.GetRedisTime()
		beginTimeUnix = beginTime.Unix()
		beginTimeStr = beginTime.Format("2006-01-02T15:04:05")
		endTimeUnix = beginTime.Add(24 * time.Hour).Unix()
		isPublishNow = true
	} else {
		beginTime, err := time.Parse("2006-01-02T15:04:05", in.BeginTime)
		lwutil.CheckError(err, "")
		if beginTime.Before(lwutil.GetRedisTime()) {
			lwutil.SendError("err_time", "begin time must later than now")
		}
		beginTimeUnix = beginTime.Unix()
		endTimeUnix = beginTime.Add(24 * time.Hour).Unix()
	}

	match := Match{
		Id:                      matchId,
		PackId:                  in.Pack.Id,
		OwnerId:                 session.Userid,
		SliderNum:               in.SliderNum,
		Thumb:                   in.Pack.Thumb,
		CouponReward:            in.CouponReward,
		BeginTime:               beginTimeUnix,
		BeginTimeStr:            beginTimeStr,
		EndTime:                 endTimeUnix,
		HasResult:               false,
		RankRewardProportions:   MATCH_RANK_REWARD_PROPORTIONS,
		LuckyRewardProportion:   MATCH_LUCKY_REWARD_PROPORTION,
		OneCoinRewardProportion: MATCH_ONE_COIN_REWARD_PROPORTION,
		OwnerRewardProportion:   MATCH_OWNER_REWARD_PROPORTION,
		ChallengeSeconds:        in.ChallengeSeconds,
	}

	//json
	js, err := json.Marshal(match)
	lwutil.CheckError(err, "")

	//add to hash
	resp, err := ssdbc.Do("hset", H_MATCH, matchId, js)
	lwutil.CheckSsdbError(resp, err)

	if isPublishNow {
		//add to Z_MATCH
		resp, err = ssdbc.Do("zset", Z_MATCH, matchId, beginTimeUnix)
		lwutil.CheckSsdbError(resp, err)
	} else {
		//add to Z_PENDING_MATCH
		resp, err = ssdbc.Do("zset", Z_PENDING_MATCH, matchId, beginTimeUnix)
		lwutil.CheckSsdbError(resp, err)
	}

	//add to Z_PLAYER_MATCH
	zPlayerMatchKey := makeZPlayerMatchKey(session.Userid)
	resp, err = ssdbc.Do("zset", zPlayerMatchKey, matchId, beginTimeUnix)
	lwutil.CheckSsdbError(resp, err)

	//decrease gold coin
	if !admin {
		addPlayerGoldCoin(ssdbc, playerKey, -in.CouponReward)
	}

	//out
	lwutil.WriteResponse(w, match)
}

func apiMatchDel(w http.ResponseWriter, r *http.Request) {
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
		MatchId int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//get match
	resp, err := ssdbc.Do("hget", H_MATCH, in.MatchId)
	lwutil.CheckSsdbError(resp, err)
	var match Match
	err = json.Unmarshal([]byte(resp[1]), &match)
	lwutil.CheckError(err, "")

	//check owner
	if match.OwnerId != session.Userid {
		lwutil.SendError("err_owner", "not the pack's owner")
	}

	//check running
	resp, err = ssdbc.Do("zexists", Z_MATCH, in.MatchId)
	if ssdbCheckExists(resp) {
		lwutil.SendError("err_publish", "match already published, del not allowed")
	}

	//del
	resp, err = ssdbc.Do("zdel", Z_MATCH, in.MatchId)
	lwutil.CheckSsdbError(resp, err)

	zPlayerMatchKey := makeZPlayerMatchKey(session.Userid)
	resp, err = ssdbc.Do("zdel", zPlayerMatchKey, in.MatchId)
	lwutil.CheckSsdbError(resp, err)

	resp, err = ssdbc.Do("hdel", H_MATCH, in.MatchId)
	lwutil.CheckSsdbError(resp, err)
}

func apiMatchList(w http.ResponseWriter, r *http.Request) {
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
		StartId   int64
		BeginTime int64
		Limit     int
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

	if in.BeginTime == 0 {
		in.BeginTime = math.MaxInt64
	}

	//get keys
	resp, err := ssdbc.Do("zrscan", Z_MATCH, in.StartId, in.BeginTime, "", in.Limit)
	lwutil.CheckSsdbError(resp, err)

	if len(resp) == 1 {
		w.Write([]byte("[]"))
		return
	}
	resp = resp[1:]

	//get matches
	num := len(resp) / 2
	args := make([]interface{}, num+2)
	args[0] = "multi_hget"
	args[1] = H_MATCH
	for i := 0; i < num; i++ {
		args = append(args, resp[i*2])
	}
	resp, err = ssdbc.Do(args...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	type OutMatch struct {
		Match
		MatchExtra
	}

	matches := make([]OutMatch, len(resp)/2)
	m := make(map[int64]int) //key:matchId, value:index
	for i, _ := range matches {
		packjs := resp[i*2+1]
		err = json.Unmarshal([]byte(packjs), &matches[i])
		lwutil.CheckError(err, "")
		m[matches[i].Id] = i
	}

	//match extra
	if len(matches) > 0 {
		args = make([]interface{}, 2, len(matches)*2+2)
		args[0] = "multi_hget"
		args[1] = H_MATCH_EXTRA
		for _, v := range matches {
			playTimesKey := makeHMatchExtraSubkey(v.Id, MATCH_EXTRA_PLAY_TIMES)
			couponRewardKey := makeHMatchExtraSubkey(v.Id, MATCH_EXTRA_COUPON_REWARD)
			args = append(args, playTimesKey, couponRewardKey)
		}
		resp, err = ssdbc.Do(args...)
		lwutil.CheckSsdbError(resp, err)
		resp = resp[1:]

		num := len(resp) / 2
		for i := 0; i < num; i++ {
			key := resp[i*2]
			var matchId int64
			var fieldKey string
			_, err = fmt.Sscanf(key, "%d/%s", &matchId, &fieldKey)
			lwutil.CheckError(err, "")

			idx := m[matchId]
			if fieldKey == MATCH_EXTRA_PLAY_TIMES {
				playTimes, err := strconv.Atoi(resp[i*2])
				lwutil.CheckError(err, "")
				matches[idx].PlayTimes = playTimes
			} else if fieldKey == MATCH_EXTRA_COUPON_REWARD {
				extraCouponReward, err := strconv.Atoi(resp[i*2])
				lwutil.CheckError(err, "")
				matches[idx].ExtraCouponReward = extraCouponReward
			}
		}
	}

	//out
	lwutil.WriteResponse(w, matches)
}

func apiMatchListMine(w http.ResponseWriter, r *http.Request) {
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
		StartId   int64
		BeginTime int64
		Limit     int
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
	if in.BeginTime == 0 {
		in.BeginTime = math.MaxInt64
	}

	//get keys
	keyMatchMine := makeZPlayerMatchKey(session.Userid)
	resp, err := ssdbc.Do("zrscan", keyMatchMine, in.StartId, in.BeginTime, "", in.Limit)
	lwutil.CheckSsdbError(resp, err)

	if len(resp) == 1 {
		w.Write([]byte("[]"))
		return
	}
	resp = resp[1:]

	//get matches
	num := len(resp) / 2
	args := make([]interface{}, num+2)
	args[0] = "multi_hget"
	args[1] = H_MATCH
	for i := 0; i < num; i++ {
		args = append(args, resp[i*2])
	}
	resp, err = ssdbc.Do(args...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	type OutMatch struct {
		Match
		MatchExtra
	}

	matches := make([]OutMatch, len(resp)/2)
	m := make(map[int64]int) //key:matchId, value:index
	for i, _ := range matches {
		packjs := resp[i*2+1]
		err = json.Unmarshal([]byte(packjs), &matches[i])
		lwutil.CheckError(err, "")
		m[matches[i].Id] = i
	}

	//match extra
	if len(matches) > 0 {
		args = make([]interface{}, 2, len(matches)*2+2)
		args[0] = "multi_hget"
		args[1] = H_MATCH_EXTRA
		for _, v := range matches {
			playTimesKey := makeHMatchExtraSubkey(v.Id, MATCH_EXTRA_PLAY_TIMES)
			couponRewardKey := makeHMatchExtraSubkey(v.Id, MATCH_EXTRA_COUPON_REWARD)
			args = append(args, playTimesKey, couponRewardKey)
		}
		resp, err = ssdbc.Do(args...)
		lwutil.CheckSsdbError(resp, err)
		resp = resp[1:]

		num := len(resp) / 2
		for i := 0; i < num; i++ {
			key := resp[i*2]
			var matchId int64
			var fieldKey string
			_, err = fmt.Sscanf(key, "%d/%s", &matchId, &fieldKey)
			lwutil.CheckError(err, "")

			idx := m[matchId]
			if fieldKey == MATCH_EXTRA_PLAY_TIMES {
				playTimes, err := strconv.Atoi(resp[i*2])
				lwutil.CheckError(err, "")
				matches[idx].PlayTimes = playTimes
			} else if fieldKey == MATCH_EXTRA_COUPON_REWARD {
				extraCouponReward, err := strconv.Atoi(resp[i*2])
				lwutil.CheckError(err, "")
				matches[idx].ExtraCouponReward = extraCouponReward
			}
		}
	}

	//out
	lwutil.WriteResponse(w, matches)
}

func apiMatchGetExtra(w http.ResponseWriter, r *http.Request) {
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
		MatchId int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//
	args := make([]interface{}, 2, 4)
	args[0] = "multi_hget"
	args[1] = H_MATCH_EXTRA
	playTimesKey := makeHMatchExtraSubkey(in.MatchId, MATCH_EXTRA_PLAY_TIMES)
	couponRewardKey := makeHMatchExtraSubkey(in.MatchId, MATCH_EXTRA_COUPON_REWARD)
	args = append(args, playTimesKey, couponRewardKey)
	resp, err := ssdbc.Do(args...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	var playTimes int
	var couponReward int
	num := len(resp) / 2
	for i := 0; i < num; i++ {
		if resp[i*2] == playTimesKey {
			playTimes, err = strconv.Atoi(resp[i*2+1])
			lwutil.CheckError(err, "")
		} else if resp[i*2] == couponRewardKey {
			couponReward, err = strconv.Atoi(resp[i*2+1])
			lwutil.CheckError(err, "")
		}
	}

	//out
	out := map[string]interface{}{
		"PlayTimes":    playTimes,
		"CouponReward": couponReward,
	}
	lwutil.WriteResponse(w, out)
}

func regMatch() {
	http.Handle("/match/new", lwutil.ReqHandler(apiMatchNew))
	http.Handle("/match/del", lwutil.ReqHandler(apiMatchDel))

	http.Handle("/match/list", lwutil.ReqHandler(apiMatchList))
	http.Handle("/match/listMine", lwutil.ReqHandler(apiMatchListMine))

	http.Handle("/match/getExtra", lwutil.ReqHandler(apiMatchGetExtra))

}
