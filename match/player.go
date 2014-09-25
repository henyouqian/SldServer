package main

import (
	"./ssdb"
	"encoding/json"
	"fmt"
	"github.com/golang/glog"
	"github.com/henyouqian/lwutil"
	"github.com/qiniu/api/rs"
	"math"
	"net/http"
	"strconv"
)

const (
	H_PLAYER_INFO        = "H_PLAYER_INFO"     //key:H_PLAYER_INFO/<userId> subkey:property
	H_APP_PLAYER_RATE    = "H_APP_PLAYER_RATE" //subkey:appName/userId value:1
	USER_UPLOAD_BUCKET   = "pintuuserupload"
	ADS_PERCENT_DEFAUT   = 0.5
	H_PLAYER_REWARD      = "H_PLAYER_REWARD" //key:H_PLAYER_REWARD subkey:rewardRecordId value rewardRecordJson
	Z_PLAYER_REWARD      = "Z_PLAYER_REWARD" //key:Z_PLAYER_REWARD/userId subkey:rewardRecordId score:timeUnix
	SEREAL_PLAYER_REWARD = "SEREAL_PLAYER_REWARD"
)

type PlayerInfo struct {
	NickName        string
	TeamName        string
	Gender          int
	CustomAvatarKey string
	GravatarKey     string
	GoldCoin        int
	Coupon          int
	CouponCache     int64
	TotalCoupon     int64
	Secret          string
	CurrChallengeId int
	BetMax          int
	AllowSave       bool
}

//player property
const (
	PLAYER_GOLD_COIN         = "GoldCoin"
	PLAYER_COUPON            = "Coupon"
	PLAYER_COUPON_CACHE      = "CouponCache"
	PLAYER_TOTAL_COUPON      = "TotalCoupon"
	PLAYER_IAP_SECRET        = "IapSecret"
	PLAYER_CURR_CHALLENGE_ID = "CurrChallengeId"
)

const (
	REWARD_REASON_RANK  = "排名奖励"
	REWARD_REASON_LUCK  = "幸运奖"
	REWARD_REASON_OWNER = "发布分成"
)

type RewardRecord struct {
	Thumb   string
	Reason  string
	MatchId int64
	Num     int
}

func makeZPlayerRewardKey(userId int64) string {
	return fmt.Sprintf("%s, %d", Z_PLAYER_REWARD, userId)
}

func addCouponToCache(ssdbc *ssdb.Client, userId int64, matchId int64, matchThumb string, coinNum int, reason string) {
	if coinNum == 0 {
		return
	}
	playerKey := makePlayerInfoKey(userId)
	resp, err := ssdbc.Do("hincr", playerKey, PLAYER_COUPON_CACHE, coinNum)
	lwutil.CheckSsdbError(resp, err)

	var record RewardRecord
	record.Thumb = matchThumb
	record.Reason = reason
	record.MatchId = matchId
	record.Num = coinNum
	js, err := json.Marshal(record)
	lwutil.CheckError(err, "")

	recordId := GenSerial(ssdbc, SEREAL_PLAYER_REWARD)
	resp, err = ssdbc.Do("hset", H_PLAYER_REWARD, recordId, js)
	lwutil.CheckSsdbError(resp, err)

	zkey := makeZPlayerRewardKey(userId)
	resp, err = ssdbc.Do("zset", zkey, recordId, recordId)
	lwutil.CheckSsdbError(resp, err)
}

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
		ssdb.HSet(key, PLAYER_CURR_CHALLENGE_ID, 1)
	}

	return &playerInfo, err
}

func addPlayerGoldCoin(ssc *ssdb.Client, playerKey string, addNum int) (rNum int) {
	resp, err := ssc.Do("hincr", playerKey, PLAYER_GOLD_COIN, addNum)
	lwutil.CheckSsdbError(resp, err)
	num, err := strconv.Atoi(resp[1])
	lwutil.CheckError(err, "")
	return num
}
func getPlayerGoldCoin(ssc *ssdb.Client, playerKey string) (rNum int) {
	resp, err := ssc.Do("hget", playerKey, PLAYER_GOLD_COIN)
	lwutil.CheckError(err, "")
	if resp[0] == ssdb.NOT_FOUND {
		return 0
	}

	num, err := strconv.Atoi(resp[1])
	lwutil.CheckError(err, "")
	return num
}

func addPlayerCoupon(ssc *ssdb.Client, playerKey string, addCoupon int) (rCoupon int) {
	resp, err := ssc.Do("hincr", playerKey, PLAYER_COUPON, addCoupon)
	lwutil.CheckSsdbError(resp, err)
	num, err := strconv.Atoi(resp[1])
	lwutil.CheckError(err, "")

	return num
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

	//out
	out := struct {
		*PlayerInfo
		UserId int64
		// BetCloseBeforeEndSec  int
		AdsConf               AdsConf
		ClientConf            map[string]string
		OwnerRewardProportion float32
	}{
		playerInfo,
		session.Userid,
		// BET_CLOSE_BEFORE_END_SEC,
		_adsConf,
		_clientConf,
		MATCH_OWNER_REWARD_PROPORTION,
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
	}{
		*playerInfo,
	}
	lwutil.WriteResponse(w, out)
}

func apiAddCouponFromCache(w http.ResponseWriter, r *http.Request) {
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
	var couponCache int64
	err = ssdb.HGet(key, PLAYER_COUPON_CACHE, &couponCache)
	lwutil.CheckError(err, "")
	if couponCache > 0 {
		ssdb.Do("hincr", key, PLAYER_COUPON_CACHE, -couponCache)
		ssdb.Do("hincr", key, PLAYER_COUPON, couponCache)
		ssdb.Do("hincr", key, PLAYER_TOTAL_COUPON, couponCache)
	}

	playerInfo, err := getPlayerInfo(ssdb, session.Userid)
	lwutil.CheckError(err, "")

	//out
	out := map[string]interface{}{
		"Coupon":      playerInfo.Coupon,
		"TotalCoupon": playerInfo.TotalCoupon,
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

func apiListMyCoupon(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//in
	var in struct {
		StartId int64
		Limit   int
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if in.StartId <= 0 {
		in.StartId = math.MaxInt32
	}

	if in.Limit > 50 {
		in.Limit = 50
	}

	//
	playerCouponZKey := makePlayerCouponZsetKey(session.Userid)
	resp, err := ssdbc.Do("zrscan", playerCouponZKey, in.StartId, in.StartId, "", in.Limit)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	itemNum := len(resp) / 2
	cmds := make([]interface{}, itemNum+2)
	cmds[0] = "multi_hget"
	cmds[1] = H_COUPON_ITEM
	for i := 0; i < itemNum; i++ {
		cmds[i+2] = resp[i*2]
	}

	//
	resp, err = ssdbc.Do(cmds...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]
	num := len(resp) / 2

	//
	out := make([]CouponItem, num)
	for i := 0; i < num; i++ {
		err = json.Unmarshal([]byte(resp[i*2+1]), &out[i])
		lwutil.CheckError(err, "")
	}

	lwutil.WriteResponse(w, out)
}

func apiListMyReward(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//in
	var in struct {
		StartId    int64
		StartScore int64
		Limit      int
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if in.StartId <= 0 {
		in.StartId = math.MaxInt32
	}

	if in.Limit > 50 {
		in.Limit = 50
	}

	//out
	out := []RewardRecord{}

	//
	zkey := makeZPlayerRewardKey(session.Userid)
	resp, err := ssdbc.Do("zrscan", zkey, in.StartId, in.StartScore, "", in.Limit)
	lwutil.CheckError(err, "")
	resp = resp[1:]
	if len(resp) == 0 {
		lwutil.WriteResponse(w, out)
		return
	}

	num := len(resp) / 2
	cmds := make([]interface{}, num+2)
	cmds[0] = "multi_hget"
	cmds[1] = H_PLAYER_REWARD
	for i := 0; i < num; i++ {
		cmds[i+2] = resp[i*2]
	}

	//
	resp, err = ssdbc.Do(cmds...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]
	num = len(resp) / 2
	for i := 0; i < num; i++ {
		js := resp[i*2+1]
		rewardRecord := RewardRecord{}
		err = json.Unmarshal([]byte(js), &rewardRecord)
		lwutil.CheckError(err, "")
		out = append(out, rewardRecord)
	}

	lwutil.WriteResponse(w, out)
}

func regPlayer() {
	http.Handle("/player/getInfo", lwutil.ReqHandler(apiGetPlayerInfo))
	http.Handle("/player/setInfo", lwutil.ReqHandler(apiSetPlayerInfo))
	http.Handle("/player/addCouponFromCache", lwutil.ReqHandler(apiAddCouponFromCache))
	http.Handle("/player/getUptokens", lwutil.ReqHandler(apiGetUptokens))
	http.Handle("/player/getUptoken", lwutil.ReqHandler(apiGetUptoken))
	http.Handle("/player/listMyCoupon", lwutil.ReqHandler(apiListMyCoupon))
	http.Handle("/player/listMyReward", lwutil.ReqHandler(apiListMyReward))
}
