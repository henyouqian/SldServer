package main

import (
	"./ssdb"
	"encoding/json"
	"fmt"
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

	if resp[0] == ssdb.NOT_FOUND {
		//save
		_adsConf.ShowPercent = 1
		_adsConf.DelayPercent = 0
		_adsConf.DelaySec = 0.5

		js, err := json.Marshal(_adsConf)
		lwutil.CheckError(err, "")
		resp, err := ssdbc.Do("set", ADS_CONF_KEY, js)
		lwutil.CheckSsdbError(resp, err)
	} else {
		err = json.Unmarshal([]byte(resp[1]), &_adsConf)
		checkError(err)
	}
}

func apiGetUserInfo(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//in
	var in struct {
		UserId int64
		Email  string
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//
	matchDb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer matchDb.Close()

	//
	if in.UserId > 0 {
		player, err := getPlayerInfo(matchDb, in.UserId)
		lwutil.CheckError(err, "")
		lwutil.WriteResponse(w, player)
	} else if in.Email != "" {
		authDb, err := ssdbAuthPool.Get()
		lwutil.CheckError(err, "")
		defer authDb.Close()

		resp, err := authDb.Do("hget", H_NAME_ACCONT, in.Email)
		lwutil.CheckError(err, "")
		if resp[0] != "ok" {
			lwutil.SendError("err_not_found", "email error")
		}
		userId, err := strconv.ParseInt(resp[1], 10, 64)
		lwutil.CheckError(err, "")

		out := struct {
			*PlayerInfo
			UserId int64
		}{}
		out.PlayerInfo, err = getPlayerInfo(matchDb, userId)
		lwutil.CheckError(err, fmt.Sprintf("userId:%d", userId))
		out.UserId = userId

		lwutil.WriteResponse(w, out)
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
		UserId      int64
		AddGoldCoin int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//
	key := makePlayerInfoKey(in.UserId)
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
	addCoupon(ssdbc, key, float32(in.AddCoupon))

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

func regAdmin() {
	http.Handle("/admin/getUserInfo", lwutil.ReqHandler(apiGetUserInfo))
	http.Handle("/admin/addGoldCoin", lwutil.ReqHandler(apiAddGoldCoin))
	http.Handle("/admin/addCoupon", lwutil.ReqHandler(apiAddCoupon))
	http.Handle("/admin/setAdsConf", lwutil.ReqHandler(apiSetAdsConf))
}
