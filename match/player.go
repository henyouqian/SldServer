package main

import (
	"./ssdb"
	"fmt"
	"github.com/golang/glog"
	"github.com/henyouqian/lwutil"
	"github.com/qiniu/api/rs"
	"net/http"
	"strconv"
)

const (
	H_PLAYER_INFO      = "H_PLAYER_INFO"     //key:H_PLAYER_INFO/<userId> subkey:property
	H_APP_PLAYER_RATE  = "H_APP_PLAYER_RATE" //subkey:appName/userId value:1
	USER_UPLOAD_BUCKET = "pintuuserupload"
	INIT_MONEY         = 500
	FLD_PLAYER_MONEY   = "money"
	FLD_PLAYER_TEAM    = "team"
	ADS_PERCENT_DEFAUT = 0.5
	RATE_REWARD        = 500
)

type PlayerInfo struct {
	NickName        string
	TeamName        string
	Gender          int
	CustomAvatarKey string
	GravatarKey     string
	Money           int64
	BetMax          int
	RewardCache     int64
	TotalReward     int64
	Secret          string
	CurrChallengeId int
	AllowSave       bool
}

//player property
const (
	playerMoney           = "Money"
	playerRewardCache     = "RewardCache"
	playerTotalReward     = "TotalReward"
	playerIapSecret       = "IapSecret"
	playerCurrChallengeId = "CurrChallengeId"
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
	if err != nil {
		return nil, err
	}

	if playerInfo.CurrChallengeId == 0 {
		playerInfo.CurrChallengeId = 1
		ssdb.HSet(key, playerCurrChallengeId, 1)
	}

	return &playerInfo, err
}

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
		UserId               int64
		BetCloseBeforeEndSec int
		AdsConf              AdsConf
		RateReward           int
		ClientConf           map[string]string
	}{
		playerInfo,
		session.Userid,
		BET_CLOSE_BEFORE_END_SEC,
		_adsConf,
		rateReward,
		_clientConf,
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
	playerInfo, err := getPlayerInfo(ssdb, session.Userid)
	if playerInfo == nil {
		//set default value
		playerInfo = &in
		playerInfo.BetMax = 100
		playerInfo.Money = INIT_MONEY
		playerInfo.CurrChallengeId = 1
	} else {
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

	//save
	playerKey := makePlayerInfoKey(session.Userid)
	err = ssdb.HSetStruct(playerKey, *playerInfo)
	lwutil.CheckError(err, "")

	//out
	out := struct {
		PlayerInfo
		BetCloseBeforeEndSec int
	}{
		*playerInfo,
		BET_CLOSE_BEFORE_END_SEC,
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
		ssdb.Do("hincr", key, playerTotalReward, rewardCache)
	}

	playerInfo, err := getPlayerInfo(ssdb, session.Userid)
	lwutil.CheckError(err, "")

	//out
	out := map[string]interface{}{
		"Money":       playerInfo.Money,
		"TotalReward": playerInfo.TotalReward,
	}
	lwutil.WriteResponse(w, out)
}

func apiGetUptokens(w http.ResponseWriter, r *http.Request) {
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

func apiGetUptoken(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")

	//session
	_, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//
	scope := fmt.Sprintf("%s", USER_UPLOAD_BUCKET)
	putPolicy := rs.PutPolicy{
		Scope: scope,
	}

	//out
	out := struct {
		Token string
	}{
		putPolicy.Token(nil),
	}

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
	http.Handle("/player/getUptokens", lwutil.ReqHandler(apiGetUptokens))
	http.Handle("/player/getUptoken", lwutil.ReqHandler(apiGetUptoken))
	http.Handle("/player/rate", lwutil.ReqHandler(apiRate))
}
