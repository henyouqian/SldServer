package main

import (
	"./ssdb"
	"encoding/json"
	// "errors"
	"github.com/golang/glog"
	"github.com/henyouqian/lwutil"
	// . "github.com/qiniu/api/conf"
	"fmt"
	"github.com/qiniu/api/rs"
	"net/http"
	"strconv"
)

const (
	H_PLAYER_INFO      = "H_PLAYER_INFO"     //subkey:(int64)userId value:(PlayerInfo)playerInfo
	H_APP_PLAYER_RATE  = "H_APP_PLAYER_RATE" //subkey:appName/userId value:1
	USER_UPLOAD_BUCKET = "pintugame"
	INIT_MONEY         = 500
	FLD_PLAYER_MONEY   = "money"
	FLD_PLAYER_TEAM    = "team"
	ADS_PERCENT_DEFAUT = 0.5
	RATE_REWARD        = 500
)

type PlayerInfo struct {
	NickName         string
	TeamName         string
	Gender           int
	CustomAvatarKey  string
	GravatarKey      string
	Money            int64
	BetMax           int
	RewardCache      int64
	TotalReward      int64
	Secret           string
	ChallengeEventId int
	AllowSave        bool
}

//player property
const (
	playerMoney            = "Money"
	playerRewardCache      = "RewardCache"
	playerTotalReward      = "TotalReward"
	playerIapSecret        = "IapSecret"
	playerChallengeEventId = "ChallengeEventId"
)

func init() {
	glog.Info("")
}

func makePlayerInfoKey(userId int64) string {
	return fmt.Sprintf("%s/%d", H_PLAYER_INFO, userId)
}

func makeAppPlayerRateSubkey(appName string, userId int64) string {
	return fmt.Sprintf("%s/%d", appName, userId)
}

func getPlayerInfo(ssdb *ssdb.Client, userId int64) (*PlayerInfo, error) {
	key := makePlayerInfoKey(userId)

	var playerInfo PlayerInfo
	err := ssdb.HGetStruct(key, &playerInfo)

	if playerInfo.ChallengeEventId == 0 {
		playerInfo.ChallengeEventId = 1
		ssdb.HSet(key, playerChallengeEventId, 1)
	}

	return &playerInfo, err
	// resp, err := ssdb.Do("hget", H_PLAYER_INFO, userId)
	// lwutil.CheckSsdbError(resp, err)

	// var playerInfo PlayerInfo
	// if resp[0] == "not_found" {
	// 	return nil, errors.New("not_found")
	// } else {
	// 	err = json.Unmarshal([]byte(resp[1]), &playerInfo)
	// 	lwutil.CheckError(err, "")
	// }
	// playerInfo.AllowSave = true
	// return &playerInfo, nil
}

// func savePlayerInfo(ssdb *ssdb.Client, userId int64, playerInfo *PlayerInfo) {
// 	key := makePlayerInfoKey(userId)
// 	ssdb.HSetStruct(key, playerInfo)

// 	// if !playerInfo.AllowSave {
// 	// 	lwutil.SendError("err_player_save_locked", "")
// 	// }
// 	// js, err := json.Marshal(playerInfo)
// 	// lwutil.CheckError(err, "")

// 	// resp, err := ssdb.Do("hset", H_PLAYER_INFO, userId, js)
// 	// lwutil.CheckSsdbError(resp, err)
// }

func addPlayerMoney(ssc *ssdb.Client, userId int64, addMoney int64) (rMoney int64) {
	playerKey := makePlayerInfoKey(userId)
	resp, err := ssc.Do("hincr", playerKey, playerMoney, addMoney)
	lwutil.CheckSsdbError(resp, err)
	money, err := strconv.ParseInt(resp[1], 10, 64)
	lwutil.CheckError(err, "")

	resp, err = ssc.Do("hincr", playerKey, playerTotalReward, addMoney)
	lwutil.CheckSsdbError(resp, err)

	return money
}

func addPlayerMoneyToCache(ssc *ssdb.Client, userId int64, addMoney int64) {
	playerKey := makePlayerInfoKey(userId)
	resp, err := ssc.Do("hincr", playerKey, playerRewardCache, addMoney)
	lwutil.CheckSsdbError(resp, err)
	resp, err = ssc.Do("hincr", playerKey, playerTotalReward, addMoney)
	lwutil.CheckSsdbError(resp, err)
}

func apiGetPlayerInfo(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//get info
	playerInfo, err := getPlayerInfo(ssdb, session.Userid)
	lwutil.CheckError(err, "")

	//get adsPercent
	ap := _adsPercent
	if ap < 0 {
		resp, err := ssdb.Do("get", ADS_PERCENT_KEY)
		if resp[0] == "not_found" {
			ap = ADS_PERCENT_DEFAUT
		} else {
			lwutil.CheckSsdbError(resp, err)
			ap64, err := strconv.ParseFloat(resp[1], 32)
			ap = float32(ap64)
			lwutil.CheckError(err, "")
		}
	}

	//get appPlayerRate
	subkey := makeAppPlayerRateSubkey(_conf.AppName, session.Userid)
	resp, err := ssdb.Do("hget", H_APP_PLAYER_RATE, subkey)
	rateReward := 0
	if resp[0] == "not_found" {
		rateReward = RATE_REWARD
	}

	//out
	out := struct {
		*PlayerInfo
		BetCloseBeforeEndSec int
		AdsPercent           float32
		RateReward           int
	}{
		playerInfo,
		BET_CLOSE_BEFORE_END_SEC,
		ap,
		rateReward,
	}
	lwutil.WriteResponse(w, out)
}

func apiSetPlayerInfo(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var in PlayerInfo
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")
	if in.Gender > 2 {
		lwutil.SendError("err_gender", "")
	}

	stringLimit(&in.NickName, 40)
	stringLimit(&in.GravatarKey, 20)
	stringLimit(&in.CustomAvatarKey, 40)
	stringLimit(&in.TeamName, 40)

	//check playerInfo
	if in.NickName == "" || in.TeamName == "" || (in.GravatarKey == "" && in.CustomAvatarKey == "") {
		lwutil.SendError("err_info_incomplete", "")
	}

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//check player info exist
	var playerInfo PlayerInfo
	resp, err := ssdb.Do("hget", H_PLAYER_INFO, session.Userid)
	if resp[0] == "not_found" {
		//set default value
		playerInfo = in
		playerInfo.BetMax = 100
		playerInfo.Money = INIT_MONEY
		playerInfo.ChallengeEventId = 1
	} else {
		err = json.Unmarshal([]byte(resp[1]), &playerInfo)
		lwutil.CheckError(err, "")
		if len(in.NickName) > 0 {
			playerInfo.NickName = in.NickName
		}
		playerInfo.GravatarKey = in.GravatarKey
		playerInfo.CustomAvatarKey = in.CustomAvatarKey
		if len(in.TeamName) > 0 {
			playerInfo.TeamName = in.TeamName
		}
		playerInfo.Gender = in.Gender
	}

	//set
	playerInfo.AllowSave = true

	//save
	key := makePlayerInfoKey(session.Userid)
	ssdb.HSetStruct(key, playerInfo)

	//out
	out := struct {
		PlayerInfo
		BetCloseBeforeEndSec int
		AdsPercent           float32
	}{
		playerInfo,
		BET_CLOSE_BEFORE_END_SEC,
		ADS_PERCENT_DEFAUT,
	}
	lwutil.WriteResponse(w, out)
}

func apiAddRewardFromCache(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//
	key := makePlayerInfoKey(session.Userid)
	var rewardCache int64
	err = ssdb.HGet(key, playerRewardCache, &rewardCache)
	lwutil.CheckError(err, "")
	if rewardCache > 0 {
		ssdb.Do("hincr", key, playerRewardCache, -rewardCache)
		ssdb.Do("hincr", key, playerMoney, rewardCache)
	}

	var money int64
	err = ssdb.HGet(key, playerMoney, &money)

	// //
	// playerInfo, err := getPlayerInfo(ssdb, session.Userid)
	// lwutil.CheckError(err, "")
	// if playerInfo.RewardCache > 0 {
	// 	playerInfo.Money += playerInfo.RewardCache
	// 	playerInfo.RewardCache = 0
	// 	savePlayerInfo(ssdb, session.Userid, playerInfo)
	// }

	//out
	out := map[string]interface{}{
		"Money": money,
	}
	lwutil.WriteResponse(w, out)
}

func apiGetUptoken(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")

	//in
	var in []string
	err := lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	inLen := len(in)
	type outElem struct {
		Key   string
		Token string
	}
	out := make([]outElem, inLen, inLen)
	for i, v := range in {
		scope := fmt.Sprintf("%s:%s", USER_UPLOAD_BUCKET, v)
		putPolicy := rs.PutPolicy{
			Scope: scope,
		}
		out[i] = outElem{
			in[i],
			putPolicy.Token(nil),
		}
	}

	//out
	lwutil.WriteResponse(w, &out)
}

func apiRate(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//ssdb
	ssc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssc.Close()

	//
	subkey := makeAppPlayerRateSubkey(_conf.AppName, session.Userid)
	resp, err := ssc.Do("hget", H_APP_PLAYER_RATE, subkey)
	addMoney := 0
	if resp[0] == "not_found" {
		addPlayerMoney(ssc, session.Userid, RATE_REWARD)
		ssc.Do("hset", H_APP_PLAYER_RATE, subkey, 1)
		addMoney = RATE_REWARD
	}

	//out
	out := struct {
		AddMoney int
	}{
		addMoney,
	}
	lwutil.WriteResponse(w, out)
}

func regPlayer() {
	http.Handle("/player/getInfo", lwutil.ReqHandler(apiGetPlayerInfo))
	http.Handle("/player/setInfo", lwutil.ReqHandler(apiSetPlayerInfo))
	http.Handle("/player/addRewardFromCache", lwutil.ReqHandler(apiAddRewardFromCache))
	http.Handle("/player/getUptoken", lwutil.ReqHandler(apiGetUptoken))
	http.Handle("/player/rate", lwutil.ReqHandler(apiRate))
}
