package main

import (
	"encoding/json"
	"github.com/golang/glog"
	"github.com/henyouqian/lwutil"
	"net/http"
	"strconv"
)

const (
	ADS_CONF_KEY = "ADS_CONF_KEY"
)

type AdsConf struct {
	ShowPercent  float32
	DelayPercent float32
	DelaySec     float32
}

var (
	_adsConf AdsConf
)

func glogAdmin() {
	glog.Info("")
}

func initAdmin() {
	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//load adsConf
	resp, err := ssdbc.Do("get", ADS_CONF_KEY)
	checkError(err)

	if len(resp) < 2 {
		//save
		js, err := json.Marshal(_adsConf)
		lwutil.CheckError(err, "")
		resp, err := ssdbc.Do("set", ADS_CONF_KEY, js)
		lwutil.CheckSsdbError(resp, err)
	} else {
		err = json.Unmarshal([]byte(resp[1]), &_adsConf)
		checkError(err)
	}
}

func apiAddGoldCoin(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	ssdbAuth, err := ssdbAuthPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbAuth.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	checkAdmin(session)

	//in
	var in struct {
		AddGoldCoin int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//
	key := makePlayerInfoKey(session.Userid)
	resp, err := ssdb.Do("hincr", key, PLAYER_GOLD_COIN, in.AddGoldCoin)
	lwutil.CheckSsdbError(resp, err)

	var playerInfo PlayerInfo
	ssdb.HGetStruct(key, &playerInfo)

	//out
	lwutil.WriteResponse(w, playerInfo)
}

func apiAddCoupon(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	ssdbAuth, err := ssdbAuthPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbAuth.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	checkAdmin(session)

	//in
	var in struct {
		UserId    int64
		UserName  string
		AddCoupon int
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//get userid
	userId := in.UserId
	if userId == 0 {
		resp, err := ssdbAuth.Do("hget", H_NAME_ACCONT, in.UserName)
		lwutil.CheckError(err, "")
		if resp[0] != "ok" {
			lwutil.SendError("err_not_exist", "account not exist")
		}
		userId, err = strconv.ParseInt(resp[1], 10, 64)
		lwutil.CheckError(err, "")
	}

	key := makePlayerInfoKey(userId)
	addPlayerCoupon(ssdbc, key, in.AddCoupon)

	var playerInfo PlayerInfo
	ssdbc.HGetStruct(key, &playerInfo)

	//out
	lwutil.WriteResponse(w, playerInfo)
}

func apiSetAdsConf(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	checkAdmin(session)

	//in
	var in AdsConf
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")
	if in.ShowPercent < 0 {
		in.ShowPercent = 0
	} else if in.ShowPercent > 1 {
		in.ShowPercent = 1
	}

	_adsConf = in

	//save
	js, err := json.Marshal(_adsConf)
	lwutil.CheckError(err, "")
	resp, err := ssdb.Do("set", ADS_CONF_KEY, js)
	lwutil.CheckSsdbError(resp, err)

	//out
	lwutil.WriteResponse(w, in)
}

func apiSetCurrChallengeId(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	ssdbAuth, err := ssdbAuthPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbAuth.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	checkAdmin(session)

	//in
	var in struct {
		UserName    string
		ChallengeId int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if in.ChallengeId <= 0 {
		lwutil.SendError("err_invalid_input", "")
	}

	//get userId
	resp, err := ssdbAuth.Do("hget", H_NAME_ACCONT, in.UserName)
	lwutil.CheckError(err, "")
	if resp[0] != "ok" {
		lwutil.SendError("err_not_match", "name and password not match")
	}
	userId, err := strconv.ParseInt(resp[1], 10, 64)
	lwutil.CheckError(err, "")

	//save
	playerKey := makePlayerInfoKey(userId)

	resp, err = ssdb.Do("hset", playerKey, PLAYER_CURR_CHALLENGE_ID, in.ChallengeId)
	lwutil.CheckSsdbError(resp, err)

	//out
	lwutil.WriteResponse(w, in)
}

func regAdmin() {
	http.Handle("/admin/addGoldCoin", lwutil.ReqHandler(apiAddGoldCoin))
	http.Handle("/admin/addCoupon", lwutil.ReqHandler(apiAddCoupon))
	http.Handle("/admin/setAdsConf", lwutil.ReqHandler(apiSetAdsConf))
	http.Handle("/admin/setCurrChallengeId", lwutil.ReqHandler(apiSetCurrChallengeId))
}
