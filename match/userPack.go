package main

import (
	"encoding/json"
	"fmt"
	"github.com/henyouqian/lwutil"
	"math"
	"net/http"
	"strconv"
)

const (
	H_USER_PACK            = "H_USER_PACK"        //key:H_USER_PACK subkey:userPackId value:userPack
	Z_USER_PACK            = "Z_USER_PACK"        //key:Z_USER_PACK/userId subkey:userPackId score:userPackId
	Z_USER_PACK_LATEST     = "Z_USER_PACK_LATEST" //subkey:userPackId
	USER_PACK_SERIAL       = "USER_PACK_SERIAL"
	USER_PACK_PRICE_MAX    = 10
	Z_USER_PACK_RANKS      = "Z_USER_PACK_RANKS"      //key:Z_USER_PACK_RANKS/userPackId subkey:userId
	H_USER_PACK_PLAY_TIMES = "H_USER_PACK_PLAY_TIMES" //subkey:userPackId
)

type UserPackRank struct {
	UserId   int64
	UserName string
	Score    int
}

type UserPack struct {
	Id        int64
	PackId    int64
	SliderNum int
	Price     int
	Thumb     string
}

func makeZUserPackKey(userId int64) string {
	return fmt.Sprintf("%s/%d", Z_USER_PACK, userId)
}

func makeZUserPackRanksKey(userPackId int64) string {
	return fmt.Sprintf("%s/%d", Z_USER_PACK_RANKS, userPackId)
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
		Price     int
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if in.SliderNum < 3 {
		in.SliderNum = 3
	} else if in.SliderNum > 9 {
		in.SliderNum = 9
	}
	if in.Price < 0 {
		in.Price = 1
	} else if in.Price > USER_PACK_PRICE_MAX {
		in.Price = USER_PACK_PRICE_MAX
	}

	stringLimit(&in.Title, 100)
	stringLimit(&in.Text, 2000)

	//new pack
	newPack(ssdbc, &in.Pack, session.Userid)

	//new user pack
	userPackId := GenSerial(ssdbc, USER_PACK_SERIAL)
	userPack := UserPack{
		Id:        userPackId,
		PackId:    in.Pack.Id,
		SliderNum: in.SliderNum,
		Price:     in.Price,
		Thumb:     in.Thumb,
	}

	//json
	js, err := json.Marshal(userPack)
	lwutil.CheckError(err, "")

	//add to hash
	resp, err := ssdbc.Do("hset", H_USER_PACK, userPackId, js)
	lwutil.CheckSsdbError(resp, err)

	//add to zset
	zkey := makeZUserPackKey(userId)
	resp, err = ssdbc.Do("zset", zkey, userPackId, userPackId)
	lwutil.CheckSsdbError(resp, err)

	//add to Z_USER_PACK_LATEST
	resp, err = ssdbc.Do("zset", Z_USER_PACK_LATEST, userPackId, userPackId)
	lwutil.CheckSsdbError(resp, err)

	//out
	lwutil.WriteResponse(w, userPack)
}

func apiUserPackDel(w http.ResponseWriter, r *http.Request) {
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
		UserPackId int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//check owner
	zkey := makeZUserPackKey(session.Userid)
	if !isAdmin(session.Username) {
		resp, err := ssdbc.Do("zexists", zkey, in.UserPackId)
		lwutil.CheckError(err, "")
		if resp[0] == "0" {
			lwutil.SendError("err_owner", "not the pack's owner")
		}
	}

	//del
	resp, err := ssdbc.Do("zdel", zkey, in.UserPackId)
	lwutil.CheckSsdbError(resp, err)

	resp, err = ssdbc.Do("zdel", Z_USER_PACK_LATEST, in.UserPackId)
	lwutil.CheckSsdbError(resp, err)

	resp, err = ssdbc.Do("hdel", H_USER_PACK, in.UserPackId)
	lwutil.CheckSsdbError(resp, err)

	ranksKey := makeZUserPackRanksKey(in.UserPackId)
	resp, err = ssdbc.Do("zclear", ranksKey)
	lwutil.CheckSsdbError(resp, err)

	resp, err = ssdbc.Do("hdel", H_USER_PACK_PLAY_TIMES, in.UserPackId)
	lwutil.CheckSsdbError(resp, err)
}

func apiUserPackListMine(w http.ResponseWriter, r *http.Request) {
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
		UserId  int64
		StartId int64
		Limit   int
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

	//get keys
	userId := session.Userid
	zkey := makeZUserPackKey(userId)
	resp, err := ssdbc.Do("zrscan", zkey, in.StartId, in.StartId, "", in.Limit)
	lwutil.CheckSsdbError(resp, err)

	if len(resp) == 1 {
		w.Write([]byte("[]"))
		return
	}
	resp = resp[1:]

	//get user packs
	num := len(resp) / 2
	args := make([]interface{}, num+2)
	args[0] = "multi_hget"
	args[1] = H_USER_PACK

	for i := 0; i < num; i++ {
		args = append(args, resp[i*2])
	}
	resp, err = ssdbc.Do(args...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	type UserPackOut struct {
		UserPack
		PlayTimes int
	}

	userPacks := make([]UserPackOut, len(resp)/2)
	m := make(map[int64]int) //key:userPackId, value:index
	for i, _ := range userPacks {
		packjs := resp[i*2+1]
		err = json.Unmarshal([]byte(packjs), &userPacks[i])
		lwutil.CheckError(err, "")
		m[userPacks[i].Id] = i
	}

	if len(userPacks) > 0 {
		args = make([]interface{}, len(userPacks)+2)
		args[0] = "multi_hget"
		args[1] = H_USER_PACK_PLAY_TIMES
		for _, v := range userPacks {
			args = append(args, v.Id)
		}
		resp, err = ssdbc.Do(args...)
		lwutil.CheckSsdbError(resp, err)
		resp = resp[1:]

		num := len(resp) / 2
		for i := 0; i < num; i++ {
			userPackId, err := strconv.ParseInt(resp[i*2], 10, 64)
			lwutil.CheckError(err, "")
			idx := m[userPackId]
			playTimes, err := strconv.ParseInt(resp[i*2+1], 10, 64)
			lwutil.CheckError(err, "")
			userPacks[idx].PlayTimes = int(playTimes)
		}
	}

	//out
	lwutil.WriteResponse(w, userPacks)
}

func apiUserPackListLatest(w http.ResponseWriter, r *http.Request) {
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
		StartId int64
		Limit   int
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

	//get keys
	resp, err := ssdbc.Do("zrscan", Z_USER_PACK_LATEST, in.StartId, in.StartId, "", in.Limit)
	lwutil.CheckSsdbError(resp, err)

	if len(resp) == 1 {
		w.Write([]byte("[]"))
		return
	}
	resp = resp[1:]

	//get user packs
	num := len(resp) / 2
	args := make([]interface{}, num+2)
	args[0] = "multi_hget"
	args[1] = H_USER_PACK
	for i := 0; i < num; i++ {
		args = append(args, resp[i*2+1])
	}
	resp, err = ssdbc.Do(args...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	type UserPackOut struct {
		UserPack
		PlayTimes int
	}

	userPacks := make([]UserPackOut, len(resp)/2)
	m := make(map[int64]int) //key:userPackId, value:index
	for i, _ := range userPacks {
		packjs := resp[i*2+1]
		err = json.Unmarshal([]byte(packjs), &userPacks[i])
		lwutil.CheckError(err, "")
		m[userPacks[i].Id] = i
	}

	if len(userPacks) > 0 {
		args = make([]interface{}, len(userPacks)+2)
		args[0] = "multi_hget"
		args[1] = H_USER_PACK_PLAY_TIMES
		for _, v := range userPacks {
			args = append(args, v.Id)
		}
		resp, err = ssdbc.Do(args...)
		lwutil.CheckSsdbError(resp, err)
		resp = resp[1:]

		num := len(resp) / 2
		for i := 0; i < num; i++ {
			userPackId, err := strconv.ParseInt(resp[i*2], 10, 64)
			lwutil.CheckError(err, "")
			idx := m[userPackId]
			playTimes, err := strconv.ParseInt(resp[i*2+1], 10, 64)
			lwutil.CheckError(err, "")
			userPacks[idx].PlayTimes = int(playTimes)
		}
	}

	//out
	lwutil.WriteResponse(w, userPacks)
}

func apiUserPackGet(w http.ResponseWriter, r *http.Request) {
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
		UserPackId int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	resp, err := ssdbc.Do("hget", H_USER_PACK, in.UserPackId)
	lwutil.CheckSsdbError(resp, err)

	userPack := UserPack{}
	err = json.Unmarshal([]byte(resp[1]), &userPack)
	lwutil.CheckError(err, "")

	//out
	lwutil.WriteResponse(w, userPack)
}

func regUserPack() {
	http.Handle("/userPack/new", lwutil.ReqHandler(apiUserPackNew))
	http.Handle("/userPack/del", lwutil.ReqHandler(apiUserPackDel))
	http.Handle("/userPack/listMine", lwutil.ReqHandler(apiUserPackListMine))
	http.Handle("/userPack/listLatest", lwutil.ReqHandler(apiUserPackListLatest))
	http.Handle("/userPack/get", lwutil.ReqHandler(apiUserPackGet))
}
