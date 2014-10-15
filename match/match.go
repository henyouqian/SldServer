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
	"github.com/golang/glog"
	"github.com/henyouqian/lwutil"
)

const (
	MATCH_SERIAL            = "MATCH_SERIAL"
	H_MATCH                 = "H_MATCH"                 //subkey:matchId value:matchJson
	H_MATCH_EXTRA           = "H_MATCH_EXTRA"           //key:H_MATCH_EXTRA subkey:matchId/fieldKey value:fieldValue
	H_MATCH_PLAY            = "H_MATCH_PLAY"            //key:H_MATCH_PLAY subkey:matchId/userId value:match json
	H_MATCH_RANK            = "H_MATCH_RANK"            //key:H_MATCH_RANK/matchId subkey:rank value:userId
	Z_MATCH                 = "Z_MATCH"                 //subkey:matchId score:beginTime
	Z_PENDING_MATCH         = "Z_PENDING_MATCH"         //subkey:matchId score:beginTime
	Z_PLAYER_MATCH          = "Z_PLAYER_MATCH"          //key:Z_PLAYER_MATCH/userId subkey:matchId score:beginTime
	Z_MY_PLAYED_MATCH       = "Z_MY_PLAYED_MATCH"       //key:Z_MY_PLAYED_MATCH/userId subkey:matchId score:lastPlayTime
	Z_OPEN_MATCH            = "Z_OPEN_MATCH"            //subkey:matchId score:endTime
	Z_HOT_MATCH             = "Z_HOT_MATCH"             //subkey:matchId score:reward(totalReward)
	RDS_Z_MATCH_LEADERBOARD = "RDS_Z_MATCH_LEADERBOARD" //key:RDS_Z_MATCH_LEADERBOARD/matchId

	FREE_TRY_NUM             = 3
	MATCH_TRY_EXPIRE_SECONDS = 600
	MATCH_TIME_SEC           = 60 * 60 * 24
)

type Match struct {
	Id                      int64
	PackId                  int64
	OwnerId                 int64
	OwnerName               string
	SliderNum               int
	Thumb                   string
	Icon                    string
	Title                   string
	RewardCoupon            int
	BeginTime               int64
	BeginTimeStr            string
	EndTime                 int64
	HasResult               bool
	RankRewardProportions   []float32
	LuckyRewardProportion   float32
	OneCoinRewardProportion float32
	OwnerRewardProportion   float32
	ChallengeSeconds        int
	PromoUrl                string
	PromoImage              string
	Private                 bool
}

type MatchExtra struct {
	PlayTimes   int
	ExtraCoupon int
}

type MatchPlay struct {
	PlayerName       string
	GravatarKey      string
	CustomAvartarKey string
	HighScore        int
	HighScoreTime    int64
	FinalRank        int
	FreeTries        int
	Tries            int
	Team             string
	Secret           string
	SecretExpire     int64
	LuckyNums        []int64
	Reward           float32
}

const (
	MATCH_EXTRA_PLAY_TIMES    = "PlayTimes"
	MATCH_EXTRA_REWARD_COUPON = "ExtraCoupon"
)

const (
	MATCH_LUCKY_REWARD_PROPORTION    = float32(0.00)
	MATCH_ONE_COIN_REWARD_PROPORTION = float32(0.05)
	MATCH_OWNER_REWARD_PROPORTION    = float32(0.1)
)

var (
	MATCH_RANK_REWARD_PROPORTIONS = []float32{
		0.15, 0.10, 0.09, 0.08, 0.07, 0.06, 0.05, 0.04, 0.03, 0.02,
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

func makeMyPlayedMatchKey(userId int64) string {
	return fmt.Sprintf("%s/%d", Z_MY_PLAYED_MATCH, userId)
}

func makeMatchPlaySubkey(matchId int64, userId int64) string {
	return fmt.Sprintf("%d/%d", matchId, userId)
}

func makeMatchLeaderboardRdsKey(matchId int64) string {
	return fmt.Sprintf("%s/%d", RDS_Z_MATCH_LEADERBOARD, matchId)
}

func makeHMatchRankKey(matchId int64) string {
	return fmt.Sprintf("%s/%d", H_MATCH_RANK, matchId)
}

func makeSecretKey(secret string) string {
	return fmt.Sprintf("%s/%s", "MATCH_SECRET", secret)
}

func getMatch(ssdbc *ssdb.Client, matchId int64) *Match {
	resp, err := ssdbc.Do("hget", H_MATCH, matchId)
	lwutil.CheckSsdbError(resp, err)
	var match Match
	err = json.Unmarshal([]byte(resp[1]), &match)
	lwutil.CheckError(err, "")
	return &match
}

func getMatchPlay(ssdbc *ssdb.Client, matchId int64, userId int64) *MatchPlay {
	var play MatchPlay
	subkey := makeMatchPlaySubkey(matchId, userId)

	resp, err := ssdbc.Do("hget", H_MATCH_PLAY, subkey)
	lwutil.CheckError(err, "")
	if resp[0] == ssdb.NOT_FOUND {
		play.FreeTries = FREE_TRY_NUM

		playerInfo, err := getPlayerInfo(ssdbc, userId)
		if err != nil {
			glog.Errorf("no playerInfo:userId=%d", userId)
			return nil
		}

		play.Team = playerInfo.TeamName
		play.PlayerName = playerInfo.NickName
		play.GravatarKey = playerInfo.GravatarKey
		play.CustomAvartarKey = playerInfo.CustomAvatarKey

		//save
		js, err := json.Marshal(play)
		lwutil.CheckError(err, "")
		resp, err = ssdbc.Do("hset", H_MATCH_PLAY, subkey, js)
		lwutil.CheckSsdbError(resp, err)
	} else {
		err := json.Unmarshal([]byte(resp[1]), &play)
		lwutil.CheckError(err, "")
	}
	return &play
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
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var in struct {
		Pack
		BeginTimeStr     string
		SliderNum        int
		RewardCoupon     int
		ChallengeSeconds int
		PromoUrl         string
		PromoImage       string
		Private          bool
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

	//check gold coin
	playerKey := makePlayerInfoKey(session.Userid)
	// admin := isAdmin(session.Username)
	// if !admin {
	// 	goldNum := getPlayerGoldCoin(ssdbc, playerKey)
	// 	if goldNum < in.RewardCoupon {
	// 		lwutil.SendError("err_gold_coin", "goldNum < in.RewardCoupon")
	// 	}
	// }
	goldNum := getPlayerGoldCoin(ssdbc, playerKey)
	if goldNum < in.RewardCoupon {
		lwutil.SendError("err_gold_coin", "goldNum < in.RewardCoupon")
	}

	player, err := getPlayerInfo(ssdbc, session.Userid)
	lwutil.CheckError(err, "")

	//new pack
	newPack(ssdbc, &in.Pack, session.Userid)

	//new match
	matchId := GenSerial(ssdbc, MATCH_SERIAL)
	beginTimeUnix := int64(0)
	beginTimeStr := in.BeginTimeStr
	endTimeUnix := int64(0)
	isPublishNow := false
	if in.BeginTimeStr == "" {
		beginTime := lwutil.GetRedisTime()
		beginTimeUnix = beginTime.Unix()
		beginTimeStr = beginTime.Format("2006-01-02T15:04:05")
		endTimeUnix = beginTime.Add(MATCH_TIME_SEC * time.Second).Unix()
		isPublishNow = true
	} else {
		beginTime, err := time.Parse("2006-01-02T15:04:05", in.BeginTimeStr)
		lwutil.CheckError(err, "")
		if beginTime.Before(lwutil.GetRedisTime()) {
			lwutil.SendError("err_time", "begin time must later than now")
		}
		beginTimeUnix = beginTime.Unix()
		endTimeUnix = beginTime.Add(MATCH_TIME_SEC * time.Second).Unix()
	}

	match := Match{
		Id:                      matchId,
		PackId:                  in.Pack.Id,
		OwnerId:                 session.Userid,
		OwnerName:               player.NickName,
		SliderNum:               in.SliderNum,
		Thumb:                   in.Pack.Thumb,
		Title:                   in.Title,
		RewardCoupon:            in.RewardCoupon,
		BeginTime:               beginTimeUnix,
		BeginTimeStr:            beginTimeStr,
		EndTime:                 endTimeUnix,
		HasResult:               false,
		RankRewardProportions:   MATCH_RANK_REWARD_PROPORTIONS,
		LuckyRewardProportion:   MATCH_LUCKY_REWARD_PROPORTION,
		OneCoinRewardProportion: MATCH_ONE_COIN_REWARD_PROPORTION,
		OwnerRewardProportion:   MATCH_OWNER_REWARD_PROPORTION,
		ChallengeSeconds:        in.ChallengeSeconds,
		PromoUrl:                in.PromoUrl,
		PromoImage:              in.PromoImage,
		Private:                 in.Private,
	}

	//json
	js, err := json.Marshal(match)
	lwutil.CheckError(err, "")

	//add to hash
	resp, err := ssdbc.Do("hset", H_MATCH, matchId, js)
	lwutil.CheckSsdbError(resp, err)

	if isPublishNow {
		if !in.Private {
			//add to Z_MATCH
			resp, err = ssdbc.Do("zset", Z_MATCH, matchId, beginTimeUnix)
			lwutil.CheckSsdbError(resp, err)

			//Z_HOT_MATCH
			resp, err = ssdbc.Do("zset", Z_HOT_MATCH, matchId, in.RewardCoupon)
			lwutil.CheckSsdbError(resp, err)
		}

		//Z_OPEN_MATCH
		resp, err = ssdbc.Do("zset", Z_OPEN_MATCH, matchId, endTimeUnix)
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
	// if !admin {
	// 	addPlayerGoldCoin(ssdbc, playerKey, -in.RewardCoupon)
	// }
	addPlayerGoldCoin(ssdbc, playerKey, -in.RewardCoupon)

	//out
	lwutil.WriteResponse(w, match)
}

func apiMatchDel(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
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

func apiMatchMod(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var in struct {
		MatchId    int64
		Title      string
		PromoUrl   string
		PromoImage string
		Private    bool
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//get match
	match := getMatch(ssdbc, in.MatchId)

	//check owner
	if match.OwnerId != session.Userid {
		lwutil.SendError("err_owner", "not the pack's owner")
	}

	//check time
	now := lwutil.GetRedisTimeUnix()
	if now > match.EndTime {
		lwutil.WriteResponse(w, match)
		return
	}

	//private
	if match.Private != in.Private {
		match.Private = in.Private
		if match.Private {
			resp, err := ssdbc.Do("zdel", Z_MATCH, in.MatchId)
			lwutil.CheckSsdbError(resp, err)

			resp, err = ssdbc.Do("zdel", Z_HOT_MATCH, in.MatchId)
			lwutil.CheckSsdbError(resp, err)
		} else {
			//add to Z_MATCH
			resp, err := ssdbc.Do("zset", Z_MATCH, match.Id, match.BeginTime)
			lwutil.CheckSsdbError(resp, err)

			//Z_HOT_MATCH
			rewardCouponKey := makeHMatchExtraSubkey(in.MatchId, MATCH_EXTRA_REWARD_COUPON)
			resp, err = ssdbc.Do("hget", H_MATCH_EXTRA, rewardCouponKey)
			extraCoupon := 0
			if resp[0] == "ok" {
				extraCoupon, err = strconv.Atoi(resp[1])
				lwutil.CheckError(err, "")
			}

			resp, err = ssdbc.Do("zset", Z_HOT_MATCH, in.MatchId, match.RewardCoupon+extraCoupon)
			lwutil.CheckSsdbError(resp, err)
		}
	}

	//update match
	match.Title = in.Title
	match.PromoImage = in.PromoImage
	match.PromoUrl = in.PromoUrl

	js, err := json.Marshal(match)
	lwutil.CheckError(err, "")

	//save
	resp, err := ssdbc.Do("hset", H_MATCH, in.MatchId, js)
	lwutil.CheckSsdbError(resp, err)

	//out
	lwutil.WriteResponse(w, match)
}

func apiMatchList(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
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
			RewardCouponKey := makeHMatchExtraSubkey(v.Id, MATCH_EXTRA_REWARD_COUPON)
			args = append(args, playTimesKey, RewardCouponKey)
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
				playTimes, err := strconv.Atoi(resp[i*2+1])
				lwutil.CheckError(err, "")
				matches[idx].PlayTimes = playTimes
			} else if fieldKey == MATCH_EXTRA_REWARD_COUPON {
				ExtraCoupon, err := strconv.Atoi(resp[i*2+1])
				lwutil.CheckError(err, "")
				matches[idx].ExtraCoupon = ExtraCoupon
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
	lwutil.CheckError(err, "")
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
			RewardCouponKey := makeHMatchExtraSubkey(v.Id, MATCH_EXTRA_REWARD_COUPON)
			args = append(args, playTimesKey, RewardCouponKey)
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
				playTimes, err := strconv.Atoi(resp[i*2+1])
				lwutil.CheckError(err, "")
				matches[idx].PlayTimes = playTimes
			} else if fieldKey == MATCH_EXTRA_REWARD_COUPON {
				ExtraCoupon, err := strconv.Atoi(resp[i*2+1])
				lwutil.CheckError(err, "")
				matches[idx].ExtraCoupon = ExtraCoupon
			}
		}
	}

	//out
	lwutil.WriteResponse(w, matches)
}

func apiMatchListMyPlayed(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//
	lastPlayedTime := int64(0)

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var in struct {
		StartId    int64
		PlayedTime int64
		Limit      int
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
	if in.PlayedTime == 0 {
		in.PlayedTime = math.MaxInt64
	}

	//out struct
	type OutMatch struct {
		Match
		MatchExtra
	}

	type Out struct {
		Matches        []OutMatch
		LastPlayedTime int64
	}
	out := Out{
		[]OutMatch{},
		0,
	}

	//get keys
	key := makeMyPlayedMatchKey(session.Userid)
	resp, err := ssdbc.Do("zrscan", key, in.StartId, in.PlayedTime, "", in.Limit)
	lwutil.CheckSsdbError(resp, err)

	if len(resp) == 1 {
		lwutil.WriteResponse(w, out)
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
		lastPlayedTime, err = strconv.ParseInt(resp[i*2+1], 10, 64)
		lwutil.CheckError(err, "")
	}
	resp, err = ssdbc.Do(args...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

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
			RewardCouponKey := makeHMatchExtraSubkey(v.Id, MATCH_EXTRA_REWARD_COUPON)
			args = append(args, playTimesKey, RewardCouponKey)
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
				playTimes, err := strconv.Atoi(resp[i*2+1])
				lwutil.CheckError(err, "")
				matches[idx].PlayTimes = playTimes
			} else if fieldKey == MATCH_EXTRA_REWARD_COUPON {
				ExtraCoupon, err := strconv.Atoi(resp[i*2+1])
				lwutil.CheckError(err, "")
				matches[idx].ExtraCoupon = ExtraCoupon
			}
		}
	}

	//out
	out = Out{
		matches,
		lastPlayedTime,
	}

	lwutil.WriteResponse(w, out)
}

func apiMatchListHot(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//session
	_, err = findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var in struct {
		StartId   int64
		CouponSum int64
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
	if in.CouponSum == -1 {
		in.CouponSum = math.MaxInt64
	}

	//out struct
	type OutMatch struct {
		Match
		MatchExtra
	}

	type Out struct {
		Matches []OutMatch
	}
	out := Out{
		[]OutMatch{},
	}

	//get keys
	resp, err := ssdbc.Do("zrscan", Z_HOT_MATCH, in.StartId, in.CouponSum, "", in.Limit)
	lwutil.CheckSsdbError(resp, err)

	if len(resp) == 1 {
		lwutil.WriteResponse(w, out)
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
		lwutil.CheckError(err, "")
	}
	resp, err = ssdbc.Do(args...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

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
			RewardCouponKey := makeHMatchExtraSubkey(v.Id, MATCH_EXTRA_REWARD_COUPON)
			args = append(args, playTimesKey, RewardCouponKey)
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
				playTimes, err := strconv.Atoi(resp[i*2+1])
				lwutil.CheckError(err, "")
				matches[idx].PlayTimes = playTimes
			} else if fieldKey == MATCH_EXTRA_REWARD_COUPON {
				ExtraCoupon, err := strconv.Atoi(resp[i*2+1])
				lwutil.CheckError(err, "")
				matches[idx].ExtraCoupon = ExtraCoupon
			}
		}
	}

	//out
	out = Out{
		matches,
	}

	lwutil.WriteResponse(w, out)
}

func apiMatchGetDynamicData(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
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

	//
	args := make([]interface{}, 2, 4)
	args[0] = "multi_hget"
	args[1] = H_MATCH_EXTRA
	playTimesKey := makeHMatchExtraSubkey(in.MatchId, MATCH_EXTRA_PLAY_TIMES)
	RewardCouponKey := makeHMatchExtraSubkey(in.MatchId, MATCH_EXTRA_REWARD_COUPON)
	args = append(args, playTimesKey, RewardCouponKey)
	resp, err := ssdbc.Do(args...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	var playTimes int
	var ExtraCoupon int
	num := len(resp) / 2
	for i := 0; i < num; i++ {
		if resp[i*2] == playTimesKey {
			playTimes, err = strconv.Atoi(resp[i*2+1])
			lwutil.CheckError(err, "")
		} else if resp[i*2] == RewardCouponKey {
			ExtraCoupon, err = strconv.Atoi(resp[i*2+1])
			lwutil.CheckError(err, "")
		}
	}

	//get match play
	play := getMatchPlay(ssdbc, in.MatchId, session.Userid)

	//get match
	match := getMatch(ssdbc, in.MatchId)

	//get rank
	myRank := 0
	rankNum := 0
	if match.HasResult {
		myRank = play.FinalRank

		//rankNum
		hRankKey := makeHMatchRankKey(in.MatchId)
		resp, err = ssdbc.Do("hsize", hRankKey)
		lwutil.CheckSsdbError(resp, err)
		rankNum, err = strconv.Atoi(resp[1])
		lwutil.CheckError(err, "")
	} else {
		//redis
		rc := redisPool.Get()
		defer rc.Close()

		lbKey := makeMatchLeaderboardRdsKey(in.MatchId)

		//get my rank and rank count
		rc.Send("ZREVRANK", lbKey, session.Userid)
		rc.Send("ZCARD", lbKey)
		err = rc.Flush()
		lwutil.CheckError(err, "")
		myRank, err = redis.Int(rc.Receive())
		if err == nil {
			myRank += 1
		} else {
			myRank = 0
		}
		rankNum, err = redis.Int(rc.Receive())
		if err != nil {
			rankNum = 0
		}
	}

	//out
	out := struct {
		PlayTimes   int
		ExtraCoupon int
		MyRank      int
		RankNum     int
		MatchPlay
	}{
		playTimes,
		ExtraCoupon,
		myRank,
		rankNum,
		*play,
	}
	lwutil.WriteResponse(w, out)
}

func apiMatchPlayBegin(w http.ResponseWriter, r *http.Request) {
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
	now := lwutil.GetRedisTimeUnix()

	if now < match.BeginTime || now >= match.EndTime || match.HasResult {
		lwutil.SendError("err_time", "match out of time")
	}

	//get matchPlay
	play := getMatchPlay(ssdbc, in.MatchId, session.Userid)

	//free try or use goldCoin
	genLuckyNum := false
	if play.FreeTries == FREE_TRY_NUM || play.FreeTries == 0 {
		genLuckyNum = true
	}

	playerKey := makePlayerInfoKey(session.Userid)
	goldCoin := getPlayerGoldCoin(ssdbc, playerKey)
	autoPaging := false
	if play.FreeTries > 0 {
		play.FreeTries--
	} else {
		if goldCoin > 0 {
			addPlayerGoldCoin(ssdbc, playerKey, -1)
			goldCoin--
			autoPaging = true

			RewardCouponKey := makeHMatchExtraSubkey(in.MatchId, MATCH_EXTRA_REWARD_COUPON)
			resp, err = ssdbc.Do("hincr", H_MATCH_EXTRA, RewardCouponKey, 1)
			lwutil.CheckSsdbError(resp, err)
			ExtraCoupon, err := strconv.Atoi(resp[1])
			lwutil.CheckError(err, "")

			resp, err = ssdbc.Do("zset", Z_HOT_MATCH, in.MatchId, match.RewardCoupon+ExtraCoupon)
			lwutil.CheckSsdbError(resp, err)
		} else {
			lwutil.SendError("err_gold_coin", "no coin")
		}
	}
	play.Tries++

	playTimesKey := makeHMatchExtraSubkey(in.MatchId, MATCH_EXTRA_PLAY_TIMES)
	resp, err = ssdbc.Do("hincr", H_MATCH_EXTRA, playTimesKey, 1)

	//gen lucky number
	luckyNum := int64(0)
	if genLuckyNum {
		luckyNum = GenSerial(ssdbc, fmt.Sprintf("MATCH_LUCKY_NUM/%d", in.MatchId))
		play.LuckyNums = append(play.LuckyNums, luckyNum)
	}

	//gen secret
	play.Secret = lwutil.GenUUID()
	play.SecretExpire = lwutil.GetRedisTimeUnix() + MATCH_TRY_EXPIRE_SECONDS

	secretKey := makeSecretKey(play.Secret)
	resp, err = ssdbc.Do("setx", secretKey, in.MatchId, MATCH_TRY_EXPIRE_SECONDS)
	lwutil.CheckSsdbError(resp, err)

	//update play
	js, err := json.Marshal(play)
	lwutil.CheckError(err, "")
	subkey := makeMatchPlaySubkey(in.MatchId, session.Userid)
	resp, err = ssdbc.Do("hset", H_MATCH_PLAY, subkey, js)
	lwutil.CheckSsdbError(resp, err)

	//update Z_MY_PLAYED_MATCH
	key := makeMyPlayedMatchKey(session.Userid)
	resp, err = ssdbc.Do("zset", key, in.MatchId, now)
	lwutil.CheckSsdbError(resp, err)

	//out
	out := map[string]interface{}{
		"Secret":       play.Secret,
		"SecretExpire": play.SecretExpire,
		"LuckyNum":     luckyNum,
		"GoldCoin":     goldCoin,
		"FreeTries":    play.FreeTries,
		"AutoPaging":   autoPaging,
	}
	lwutil.WriteResponse(w, out)
}

func apiMatchPlayEnd(w http.ResponseWriter, r *http.Request) {
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
		MatchId  int64
		Secret   string
		Score    int
		Checksum string
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//check score
	if in.Score > -2000 {
		glog.Errorf("invalid score:%d, userId:%d, userName:%s", in.Score, session.Userid, session.Username)
		return
	}

	//secret
	secretKey := makeSecretKey(in.Secret)
	resp, err := ssdbc.Do("get", secretKey)
	lwutil.CheckSsdbError(resp, err)
	in.MatchId, err = strconv.ParseInt(resp[1], 10, 64)
	lwutil.CheckError(err, "")

	//checksum
	checksum := fmt.Sprintf("%s+%d9d7a", in.Secret, in.Score+8703)
	hasher := sha1.New()
	hasher.Write([]byte(checksum))
	checksum = hex.EncodeToString(hasher.Sum(nil))
	if in.Checksum != checksum {
		lwutil.SendError("err_checksum", "")
	}

	//check match play
	now := lwutil.GetRedisTimeUnix()
	matchPlayKey := makeMatchPlaySubkey(in.MatchId, session.Userid)
	resp, err = ssdbc.Do("hget", H_MATCH_PLAY, matchPlayKey)
	lwutil.CheckError(err, "")
	if resp[0] != "ok" {
		lwutil.SendError("err_not_found", "match play not found")
	}

	matchPlay := MatchPlay{}
	err = json.Unmarshal([]byte(resp[1]), &matchPlay)
	lwutil.CheckError(err, "")
	if matchPlay.Secret != in.Secret {
		lwutil.SendError("err_not_match", "Secret not match")
	}
	if now > matchPlay.SecretExpire {
		lwutil.SendError("err_expired", "secret expired")
	}

	//clear secret
	matchPlay.SecretExpire = 0

	//update score
	scoreUpdate := false
	if matchPlay.HighScore == 0 {
		matchPlay.HighScore = in.Score
		matchPlay.HighScoreTime = now
		scoreUpdate = true
	} else {
		if in.Score > matchPlay.HighScore {
			matchPlay.HighScore = in.Score
			scoreUpdate = true
		}
	}

	//save match play
	js, err := json.Marshal(matchPlay)
	resp, err = ssdbc.Do("hset", H_MATCH_PLAY, matchPlayKey, js)
	lwutil.CheckSsdbError(resp, err)

	//redis
	rc := redisPool.Get()
	defer rc.Close()

	//match leaderboard
	lbKey := makeMatchLeaderboardRdsKey(in.MatchId)
	if scoreUpdate {
		_, err = rc.Do("ZADD", lbKey, matchPlay.HighScore, session.Userid)
		lwutil.CheckError(err, "")
	}

	//get rank
	rc.Send("ZREVRANK", lbKey, session.Userid)
	rc.Send("ZCARD", lbKey)
	err = rc.Flush()
	lwutil.CheckError(err, "")
	rank, err := redis.Int(rc.Receive())
	lwutil.CheckError(err, "")
	rankNum, err := redis.Int(rc.Receive())
	lwutil.CheckError(err, "")

	// //recaculate team score
	// if scoreUpdate && rank <= TEAM_SCORE_RANK_MAX {
	// 	recaculateTeamScore(ssdbc, rc, in.EventId)
	// }

	//out
	out := struct {
		MyRank  uint32
		RankNum uint32
	}{
		uint32(rank + 1),
		uint32(rankNum),
	}

	//out
	lwutil.WriteResponse(w, out)
}

func apiMatchGetRanks(w http.ResponseWriter, r *http.Request) {
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
		MatchId int64
		Offset  int
		Limit   int
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if in.Limit > 50 {
		in.Limit = 50
	}

	//check match id
	resp, err := ssdbc.Do("hget", H_MATCH, in.MatchId)
	lwutil.CheckSsdbError(resp, err)
	var match Match
	err = json.Unmarshal([]byte(resp[1]), &match)
	lwutil.CheckError(err, "")

	//RankInfo
	type RankInfo struct {
		Rank            int
		UserId          int64
		NickName        string
		TeamName        string
		GravatarKey     string
		CustomAvatarKey string
		Score           int
		Time            int64
		Tries           int
	}

	type Out struct {
		MatchId int64
		MyRank  int
		Ranks   []RankInfo
		RankNum int
	}

	//get ranks
	var ranks []RankInfo
	myRank := 0
	rankNum := 0

	if match.HasResult {
		cmds := make([]interface{}, in.Limit+2)
		cmds[0] = "multi_hget"
		cmds[1] = makeHMatchRankKey(match.Id)
		hRankKey := cmds[1]
		for i := 0; i < in.Limit; i++ {
			rank := i + in.Offset + 1
			cmds[i+2] = rank
		}

		resp, err := ssdbc.Do(cmds...)
		lwutil.CheckSsdbError(resp, err)
		resp = resp[1:]

		num := len(resp) / 2
		ranks = make([]RankInfo, num)

		for i := 0; i < num; i++ {
			ranks[i].Rank, err = strconv.Atoi(resp[i*2])
			lwutil.CheckError(err, "")
			ranks[i].UserId, err = strconv.ParseInt(resp[i*2+1], 10, 64)
			lwutil.CheckError(err, "")
		}

		//my rank
		play := getMatchPlay(ssdbc, in.MatchId, session.Userid)
		myRank = play.FinalRank

		//rankNum
		resp, err = ssdbc.Do("hsize", hRankKey)
		lwutil.CheckSsdbError(resp, err)
		rankNum, err = strconv.Atoi(resp[1])
		lwutil.CheckError(err, "")
	} else {
		//redis
		rc := redisPool.Get()
		defer rc.Close()

		lbKey := makeMatchLeaderboardRdsKey(in.MatchId)

		//get ranks from redis
		values, err := redis.Values(rc.Do("ZREVRANGE", lbKey, in.Offset, in.Offset+in.Limit-1))
		lwutil.CheckError(err, "")
		glog.Infof("values: %+v", values)

		num := len(values)
		if num > 0 {
			ranks = make([]RankInfo, num)

			currRank := in.Offset + 1
			for i := 0; i < num; i++ {
				ranks[i].Rank = currRank
				currRank++
				ranks[i].UserId, err = redisInt64(values[i], nil)
				lwutil.CheckError(err, "")
			}
		}

		//get my rank and rank count
		rc.Send("ZREVRANK", lbKey, session.Userid)
		rc.Send("ZCARD", lbKey)
		err = rc.Flush()
		lwutil.CheckError(err, "")
		myRank, err = redis.Int(rc.Receive())
		if err == nil {
			myRank += 1
		} else {
			myRank = 0
		}
		rankNum, err = redis.Int(rc.Receive())
		if err != nil {
			rankNum = 0
		}
	}

	num := len(ranks)
	if num == 0 {
		out := Out{
			in.MatchId,
			myRank,
			[]RankInfo{},
			rankNum,
		}
		lwutil.WriteResponse(w, out)
		return
	}

	//get match plays
	cmds := make([]interface{}, 0, num+2)
	cmds = append(cmds, "multi_hget")
	cmds = append(cmds, H_MATCH_PLAY)
	for _, rank := range ranks {
		subkey := makeMatchPlaySubkey(in.MatchId, rank.UserId)
		cmds = append(cmds, subkey)
	}
	resp, err = ssdbc.Do(cmds...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	if num*2 != len(resp) {
		glog.Infof("cmds: %+v", cmds)
		glog.Infof("resp: %+v", resp)
		glog.Infof("num: %d", num)
		glog.Infof("ranks: %+v", ranks)

		lwutil.SendError("err_data_missing", "")
	}

	var play MatchPlay
	for i := range ranks {
		err = json.Unmarshal([]byte(resp[i*2+1]), &play)
		lwutil.CheckError(err, "")
		ranks[i].Score = play.HighScore
		ranks[i].NickName = play.PlayerName
		ranks[i].Time = play.HighScoreTime
		ranks[i].Tries = play.Tries
		ranks[i].TeamName = play.Team
		ranks[i].GravatarKey = play.GravatarKey
		ranks[i].CustomAvatarKey = play.CustomAvartarKey
	}

	//out
	out := Out{
		in.MatchId,
		myRank,
		ranks,
		rankNum,
	}

	lwutil.WriteResponse(w, out)
}

func regMatch() {
	http.Handle("/match/new", lwutil.ReqHandler(apiMatchNew))
	http.Handle("/match/del", lwutil.ReqHandler(apiMatchDel))
	http.Handle("/match/mod", lwutil.ReqHandler(apiMatchMod))

	http.Handle("/match/list", lwutil.ReqHandler(apiMatchList))
	http.Handle("/match/listMine", lwutil.ReqHandler(apiMatchListMine))
	http.Handle("/match/listMyPlayed", lwutil.ReqHandler(apiMatchListMyPlayed))
	http.Handle("/match/listHot", lwutil.ReqHandler(apiMatchListHot))

	http.Handle("/match/playBegin", lwutil.ReqHandler(apiMatchPlayBegin))
	http.Handle("/match/playEnd", lwutil.ReqHandler(apiMatchPlayEnd))

	http.Handle("/match/getDynamicData", lwutil.ReqHandler(apiMatchGetDynamicData))
	http.Handle("/match/getRanks", lwutil.ReqHandler(apiMatchGetRanks))
}
