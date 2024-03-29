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
		AddGoldCoin int
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//
	key := makePlayerInfoKey(in.UserId)
	resp, err := ssdb.Do("hincr", key, PLAYER_GOLD_COIN, in.AddGoldCoin)
	lwutil.CheckSsdbError(resp, err)

	//
	err = addEcoRecord(ssdb, session.Userid, in.AddGoldCoin, ECO_FORWHAT_MATCHBEGIN)
	lwutil.CheckError(err, "")

	var playerInfo PlayerInfo
	ssdb.HGetStruct(key, &playerInfo)

	//out
	lwutil.WriteResponse(w, playerInfo)
}

func apiAddPrize(w http.ResponseWriter, r *http.Request) {
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
		UserId   int64
		UserName string
		Prize    int
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
	addPrize(ssdbc, key, in.Prize)

	err = addEcoRecord(ssdbc, session.Userid, in.Prize, ECO_FORWHAT_ADMIN_PRIZE)
	lwutil.CheckError(err, "")

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

func apiAdminClearPlayer(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	checkAdmin(session)

	//in
	var in struct {
		UserId int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//
	key := makeZPlayerMatchKey(in.UserId)
	resp, err := ssdbc.Do("zclear", key)
	lwutil.CheckSsdbError(resp, err)

	key = makeZLikeMatchKey(in.UserId)
	resp, err = ssdbc.Do("zclear", key)
	lwutil.CheckSsdbError(resp, err)

	key = makeQPlayerMatchKey(in.UserId)
	resp, err = ssdbc.Do("qclear", key)
	lwutil.CheckSsdbError(resp, err)

	key = makeQLikeMatchKey(in.UserId)
	resp, err = ssdbc.Do("qclear", key)
	lwutil.CheckSsdbError(resp, err)

	lwutil.WriteResponse(w, in)
}

func apiAdminDelMatch(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	checkAdmin(session)

	//in
	var in struct {
		MatchId int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//
	match := getMatch(ssdbc, in.MatchId)
	match.Deleted = true
	saveMatch(ssdbc, match)

	//del
	resp, err := ssdbc.Do("zdel", Z_MATCH, in.MatchId)
	lwutil.CheckSsdbError(resp, err)

	resp, err = ssdbc.Do("zdel", Z_HOT_MATCH, in.MatchId)
	lwutil.CheckSsdbError(resp, err)

	key := makeZPlayerMatchKey(match.OwnerId)
	resp, err = ssdbc.Do("zdel", key, in.MatchId)
	lwutil.CheckSsdbError(resp, err)

	key = makeZLikeMatchKey(match.OwnerId)
	resp, err = ssdbc.Do("zdel", key, in.MatchId)
	lwutil.CheckSsdbError(resp, err)

	key = makeQPlayerMatchKey(match.OwnerId)
	resp, err = ssdbc.Do("qback", key)
	lwutil.CheckSsdbError(resp, err)
	matchId, err := strconv.ParseInt(resp[1], 10, 64)
	lwutil.CheckError(err, "err_strconv")
	if matchId == in.MatchId {
		ssdbc.Do("qpop_back", key)
	}

	key = makeQLikeMatchKey(match.OwnerId)
	resp, err = ssdbc.Do("qback", key)
	lwutil.CheckSsdbError(resp, err)
	matchId, err = strconv.ParseInt(resp[1], 10, 64)
	lwutil.CheckError(err, "err_strconv")
	if matchId == in.MatchId {
		ssdbc.Do("qpop_back", key)
	}

	lwutil.WriteResponse(w, in)
}

func regAdmin() {
	http.Handle("/admin/getUserInfo", lwutil.ReqHandler(apiGetUserInfo))
	http.Handle("/admin/addGoldCoin", lwutil.ReqHandler(apiAddGoldCoin))
	http.Handle("/admin/addPrize", lwutil.ReqHandler(apiAddPrize))
	http.Handle("/admin/setAdsConf", lwutil.ReqHandler(apiSetAdsConf))
	http.Handle("/admin/clearPlayer", lwutil.ReqHandler(apiAdminClearPlayer))
	http.Handle("/admin/delMatch", lwutil.ReqHandler(apiAdminDelMatch))
}
