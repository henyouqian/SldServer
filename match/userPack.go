package main

import (
	"encoding/json"
	"fmt"
	"github.com/henyouqian/lwutil"
	"net/http"
)

const (
	H_USER_PACK      = "H_USER_PACK" //key:H_USER_PACK subkey:userPackId value:userPack
	Z_USER_PACK      = "Z_USER_PACK" //key:Z_USER_PACK/userId subkey:userPackId score:userPackId
	USER_PACK_SERIAL = "USER_PACK_SERIAL"
)

type UserPack struct {
	Id        int64
	PackId    int64
	SliderNum int
	PlayTimes int
}

func makeZUserPackKey(userId int64) string {
	return fmt.Sprintf("%s/%d", Z_USER_PACK, userId)
}

func apiUserPackNew(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//ssdb
	ssdbc, err := ssdbPool.Get()
	checkError(err)
	defer ssdbc.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")
	userId := session.Userid

	//in
	var in struct {
		Pack
		SliderNum int
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//new pack
	newPack(ssdbc, &in.Pack)

	//new user pack
	userPackId := GenSerial(ssdbc, USER_PACK_SERIAL)
	userPack := UserPack{
		Id:        userPackId,
		PackId:    in.Pack.Id,
		SliderNum: in.SliderNum,
		PlayTimes: 0,
	}

	//json
	js, err := json.Marshal(userPack)
	lwutil.CheckError(err, "")

	//add to hash
	resp, err := ssdbc.Do("hset", H_USER_PACK, js)
	lwutil.CheckSsdbError(resp, err)

	//add to zset
	zkey := makeZUserPackKey(userId)
	resp, err = ssdbc.Do("zset", zkey, userPackId, userPackId)
	lwutil.CheckSsdbError(resp, err)

	//out
	lwutil.WriteResponse(w, userPack)
}

func apiUserPackList(w http.ResponseWriter, r *http.Request) {
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
		Pack
		SliderNum int
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//out
	lwutil.WriteResponse(w, in)
}

func regUserPack() {
	http.Handle("/userPack/new", lwutil.ReqHandler(apiUserPackNew))
	http.Handle("/userPack/list", lwutil.ReqHandler(apiUserPackList))
}
