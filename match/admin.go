package main

import (
	// "encoding/json"
	"github.com/golang/glog"
	"github.com/henyouqian/lwutil"
	"net/http"
	"strconv"
)

const (
	ADS_PERCENT_KEY = "ADS_PERCENT_KEY"
)

var (
	_adsPercent = float32(-1.0)
)

func glogAdmin() {
	glog.Info("")
}

func apiAddMoney(w http.ResponseWriter, r *http.Request) {
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
		UserId   int64
		UserName string
		AddMoney int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//get userid
	userId := in.UserId
	if userId == 0 {
		resp, err := ssdbAuth.Do("hget", H_NAME_ACCONT, in.UserName)
		lwutil.CheckError(err, "")
		if resp[0] != "ok" {
			lwutil.SendError("err_not_match", "name and password not match")
		}
		userId, err = strconv.ParseInt(resp[1], 10, 64)
		lwutil.CheckError(err, "")
	}

	key := makePlayerInfoKey(userId)
	resp, err := ssdb.Do("hincr", key, playerMoney, in.AddMoney)
	lwutil.CheckSsdbError(resp, err)

	var playerInfo PlayerInfo
	ssdb.HGetStruct(key, &playerInfo)

	//out
	lwutil.WriteResponse(w, playerInfo)
}

func apiSetAdsPercent(w http.ResponseWriter, r *http.Request) {
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
	var in struct {
		AdsPercent float32
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")
	if in.AdsPercent < 0 {
		in.AdsPercent = 0
	} else if in.AdsPercent > 1 {
		in.AdsPercent = 1
	}

	_adsPercent = in.AdsPercent

	//save
	resp, err := ssdb.Do("set", ADS_PERCENT_KEY, in.AdsPercent)
	lwutil.CheckSsdbError(resp, err)

	//out
	lwutil.WriteResponse(w, in)
}

func apiSetChallengeEventId(w http.ResponseWriter, r *http.Request) {
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
		UserName         string
		ChallengeEventId int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if in.ChallengeEventId <= 0 {
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

	resp, err = ssdb.Do("hset", playerKey, playerChallengeEventId, in.ChallengeEventId)
	lwutil.CheckSsdbError(resp, err)

	//out
	lwutil.WriteResponse(w, in)
}

func regAdmin() {
	http.Handle("/admin/addMoney", lwutil.ReqHandler(apiAddMoney))
	http.Handle("/admin/setAdsPercent", lwutil.ReqHandler(apiSetAdsPercent))
	http.Handle("/admin/setChallengeEventId", lwutil.ReqHandler(apiSetChallengeEventId))
}
