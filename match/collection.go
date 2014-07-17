package main

import (
	"encoding/json"
	"fmt"
	// "github.com/garyburd/redigo/redis"
	// "github.com/golang/glog"
	"github.com/henyouqian/lwutil"
	//. "github.com/qiniu/api/conf"
	//"github.com/qiniu/api/rs"
	"net/http"
	"strconv"
)

const (
	H_COLLECTION          = "hCollection" //key:collectionId, value:collectionData
	Z_USER_COLLECTION_PRE = ""            //name:Z_USER_COLLECTION_PRE/userId, key:collectionId, score:collectionId
)

type Collection struct {
	Id    uint64
	Title string
	Text  string
	Thumb string
	Packs []uint64
}

func apiNewCollection(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var col Collection
	err = lwutil.DecodeRequestBody(r, &col)
	lwutil.CheckError(err, "err_decode_body")

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//gen collection id
	resp, err := ssdb.Do("hincr", H_SERIAL, "collection", 1)
	lwutil.CheckSsdbError(resp, err)
	col.Id, _ = strconv.ParseUint(resp[1], 10, 32)

	//add to hash
	jsCol, _ := json.Marshal(&col)
	resp, err = ssdb.Do("hset", H_COLLECTION, col.Id, jsCol)
	lwutil.CheckSsdbError(resp, err)

	//add to user collection zset
	name := fmt.Sprintf("%s/%d", Z_USER_COLLECTION_PRE, session.Userid)
	resp, err = ssdb.Do("zset", name, col.Id, col.Id)
	lwutil.CheckSsdbError(resp, err)

	//out
	lwutil.WriteResponse(w, col)
}

func apiDelCollection(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var in struct {
		Id uint64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//check owner
	name := fmt.Sprintf("%s/%d", Z_USER_COLLECTION_PRE, session.Userid)
	resp, err := ssdb.Do("zexists", name, in.Id)
	lwutil.CheckSsdbError(resp, err)
	if resp[1] == "0" {
		lwutil.SendError("err_not_exist", fmt.Sprintf("not own the collection: userId=%d, collectionId=%d", session.Userid, in.Id))
	}
	resp, err = ssdb.Do("zdel", name, in.Id)
	lwutil.CheckSsdbError(resp, err)
	resp, err = ssdb.Do("hdel", H_COLLECTION, in.Id)
	lwutil.CheckSsdbError(resp, err)

	//out
	lwutil.WriteResponse(w, in)
}

func apiListCollection(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//in
	var in struct {
		UserId  uint64
		StartId uint32
		Limit   uint32
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")
	if in.UserId == 0 {
		in.UserId = ADMIN_USERID
	}
	if in.Limit > 60 {
		in.Limit = 60
	}

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//get keys
	name := fmt.Sprintf("%s/%d", Z_USER_COLLECTION_PRE, in.UserId)
	resp, err := ssdb.Do("zkeys", name, in.StartId, in.StartId, "", in.Limit)
	lwutil.CheckSsdbError(resp, err)

	if len(resp) == 1 {
		lwutil.SendError("err_not_found", "")
	}

	//get collections
	args := make([]interface{}, len(resp)+1)
	args[0] = "multi_hget"
	args[1] = H_COLLECTION
	for i, _ := range args {
		if i >= 2 {
			args[i] = resp[i-1]
		}
	}
	resp, err = ssdb.Do(args...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	collections := make([]Collection, len(resp)/2)
	for i, _ := range collections {
		coljs := resp[i*2+1]
		err = json.Unmarshal([]byte(coljs), &collections[i])
		lwutil.CheckError(err, "")
	}

	//out
	lwutil.WriteResponse(w, &collections)
}

func apiModCollection(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var col Collection
	err = lwutil.DecodeRequestBody(r, &col)
	lwutil.CheckError(err, "err_decode_body")

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//check exist and owner
	name := fmt.Sprintf("%s/%d", Z_USER_COLLECTION_PRE, session.Userid)
	resp, err := ssdb.Do("zexists", name, col.Id)
	lwutil.CheckSsdbError(resp, err)
	if resp[1] == "0" {
		lwutil.SendError("err_not_exist", fmt.Sprintf("not own the collection: userId=%d, collectionId=%d", session.Userid, col.Id))
	}

	//set to hash
	jsCol, _ := json.Marshal(&col)
	resp, err = ssdb.Do("hset", H_COLLECTION, col.Id, jsCol)
	lwutil.CheckSsdbError(resp, err)

	//out
	lwutil.WriteResponse(w, col)
}

func apiListCollectionPack(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//in
	var in struct {
		Id uint64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//get collection
	resp, err := ssdb.Do("hget", H_COLLECTION, in.Id)
	lwutil.CheckSsdbError(resp, err)
	if len(resp) == 1 {
		lwutil.SendError("err_not_found", "")
	}
	col := Collection{}
	err = json.Unmarshal([]byte(resp[1]), &col)
	lwutil.CheckError(err, "")

	//get packs
	args := make([]interface{}, len(col.Packs)+2)
	args[0] = "multi_hget"
	args[1] = H_PACK
	for i, pack := range col.Packs {
		args[i+2] = pack
	}
	resp, err = ssdb.Do(args...)
	lwutil.CheckSsdbError(resp, err)
	jsPacks := resp[1:]
	if len(jsPacks) == 0 {
		lwutil.SendError("err_not_found", "no packs")
	}

	packs := make([]Pack, len(col.Packs))
	for i, v := range jsPacks {
		if i%2 == 1 {
			err = json.Unmarshal([]byte(v), &packs[i/2])
			lwutil.CheckError(err, v)
			if packs[i/2].Tags == nil {
				packs[i/2].Tags = make([]string, 0)
			}
		}
	}

	//out
	lwutil.WriteResponse(w, &packs)
}

func regCollection() {
	http.Handle("/collection/new", lwutil.ReqHandler(apiNewCollection))
	http.Handle("/collection/del", lwutil.ReqHandler(apiDelCollection))
	http.Handle("/collection/mod", lwutil.ReqHandler(apiModCollection))
	http.Handle("/collection/list", lwutil.ReqHandler(apiListCollection))
	http.Handle("/collection/listPack", lwutil.ReqHandler(apiListCollectionPack))
}
