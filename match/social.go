package main

import (
	"encoding/json"
	"fmt"
	"github.com/golang/glog"
	"github.com/henyouqian/lwutil"
	"net/http"
)

const (
	H_SOCIAL_PACK      = "H_SOCIAL_PACK" //subkey:userId/packId/sliderNum value:socialPack
	Z_SOCIAL_PACK      = "Z_SOCIAL_PACK" //key: Z_SOCIAL_PACK/userId subkey: socialPackKey score:serial
	SERIAL_SOCIAL_PACK = "SERIAL_SOCIAL_PACK"
)

type SocialPack struct {
	UserId    int64
	PackId    int64
	SliderNum int
	PlayTimes int
	IsOwner   bool
}

func _glogSocial() {
	glog.Info("social")
}

func makeHSocialPackSubKey(userId int64, packId int64, sliderNum int) string {
	key := fmt.Sprintf("%d/%d/%d", userId, packId, sliderNum)
	key = lwutil.Sha224(key)
	return key
}

func makeZSocialPackKey(userId int64) string {
	key := fmt.Sprintf("%s/%d", Z_SOCIAL_PACK, userId)
	return key
}

func apiSocialNewPack(w http.ResponseWriter, r *http.Request) {
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
		PackId    int64
		SliderNum int
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if in.SliderNum < 3 || in.SliderNum > 8 {
		lwutil.SendError("err_slider_num", "in.SliderNum < 3 || SliderNum > 8")
	}

	//out
	out := struct {
		Key string
	}{}

	//
	subkey := makeHSocialPackSubKey(session.Userid, in.PackId, in.SliderNum)

	//check exist
	resp, err := ssdbc.Do("hget", H_SOCIAL_PACK, subkey)
	lwutil.CheckError(err, "")
	if len(resp) == 2 {
		var socialPack SocialPack
		err = json.Unmarshal([]byte(resp[1]), &socialPack)
		lwutil.CheckError(err, "")

		out.Key = subkey
		lwutil.WriteResponse(w, out)
		return
	}

	//get pack
	pack, err := getPack(ssdbc, in.PackId)
	lwutil.CheckError(err, "")

	isOwner := pack.AuthorId == session.Userid
	if isAdmin(session.Username) && pack.AuthorId == 0 {
		isOwner = true
	}

	socialPack := SocialPack{
		UserId:    session.Userid,
		PackId:    in.PackId,
		SliderNum: in.SliderNum,
		PlayTimes: 0,
		IsOwner:   isOwner,
	}

	//json
	js, err := json.Marshal(socialPack)
	lwutil.CheckError(err, "")

	//add to hash
	resp, err = ssdbc.Do("hset", H_SOCIAL_PACK, subkey, js)
	lwutil.CheckSsdbError(resp, err)

	//add to zset
	zkey := makeZSocialPackKey(session.Userid)
	score := GenSerial(ssdbc, SERIAL_SOCIAL_PACK)
	resp, err = ssdbc.Do("zset", zkey, subkey, score)
	lwutil.CheckSsdbError(resp, err)

	//out
	out.Key = subkey
	lwutil.WriteResponse(w, out)
}

func apiSocialGetPack(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//ssdb
	ssdbc, err := ssdbPool.Get()
	checkError(err)
	defer ssdbc.Close()

	//in
	var in struct {
		Key string
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	resp, err := ssdbc.Do("hget", H_SOCIAL_PACK, in.Key)
	lwutil.CheckSsdbError(resp, err)

	socialPack := SocialPack{}
	err = json.Unmarshal([]byte(resp[1]), &socialPack)
	lwutil.CheckError(err, "")

	//get pack
	pack, err := getPack(ssdbc, socialPack.PackId)
	lwutil.CheckError(err, "")

	//out
	out := struct {
		SocialPack
		Pack *Pack
	}{
		socialPack,
		pack,
	}

	lwutil.WriteResponse(w, out)
}

func regSocial() {
	http.Handle("/social/newPack", lwutil.ReqHandler(apiSocialNewPack))
	http.Handle("/social/getPack", lwutil.ReqHandler(apiSocialGetPack))
}
