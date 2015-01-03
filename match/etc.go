package main

import (
	"encoding/json"
	"github.com/golang/glog"
	"github.com/henyouqian/lwutil"
	"math"
	"net/http"
	"time"

	"github.com/qiniu/api/rs"
)

const (
	Z_ADVICE       = "Z_ADVICE"
	H_ADVICE       = "H_ADVICE"
	SEREIAL_ADVICE = "SEREIAL_ADVICE"
	Z_REPORT       = "Z_REPORT" //subkey:matchId score:time
)

type Advice struct {
	Id              int64
	UserId          int64
	UserNickName    string
	GravatarKey     string
	CustomAvatarKey string
	Team            string
	Text            string
	TimeUnix        int64
}

func etcGlog() {
	glog.Info("")
}

func apiBetHelp(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")

	//session
	_, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//
	text := `• 只有投中第一名的队伍才能获得奖金。

• 投中者平分所有奖金。

• 获得奖金到数量为投注金额乘以赔率。

• 赔率根据每支队伍投注数量动态改变，被投注越少的队伍赔率越高。

• 奖金以投注结束时的赔率来计算。

• 队伍积分计算方法为个人前一百名的积分相加。第一名150分，第二名99分，第三名98分，第四名97分...依此类推...第一百名1分。
`

	//out
	out := struct {
		Text string
	}{
		text,
	}

	lwutil.WriteResponse(w, out)
}

func apiAddAdvice(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//in
	var in struct {
		Text string
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if len(in.Text) == 0 {
		lwutil.SendError("err_empty_text", "")
	}
	stringLimit(&in.Text, 2000)

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//player
	playerInfo, err := getPlayerInfo(ssdb, session.Userid)
	lwutil.CheckError(err, "")

	//override in
	advice := Advice{}
	advice.Text = in.Text
	advice.Id = GenSerial(ssdb, SEREIAL_ADVICE)
	advice.UserId = session.Userid
	advice.UserNickName = playerInfo.NickName
	advice.Team = playerInfo.TeamName
	advice.GravatarKey = playerInfo.GravatarKey
	advice.CustomAvatarKey = playerInfo.CustomAvatarKey
	advice.TimeUnix = time.Now().Unix()

	//save
	resp, err := ssdb.Do("zset", Z_ADVICE, advice.Id, advice.Id)
	lwutil.CheckSsdbError(resp, err)

	js, err := json.Marshal(advice)
	lwutil.CheckError(err, "")
	resp, err = ssdb.Do("hset", H_ADVICE, advice.Id, js)
	lwutil.CheckSsdbError(resp, err)

	//out
	lwutil.WriteResponse(w, &advice)
}

func apiListAdvice(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//in
	var in struct {
		StartId int64
		Limit   uint
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if in.StartId == 0 {
		in.StartId = math.MaxInt64
	}
	if in.Limit > 50 {
		in.Limit = 50
	}

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//get zset
	resp, err := ssdb.Do("zrscan", Z_ADVICE, in.StartId, in.StartId, "", in.Limit)
	lwutil.CheckSsdbError(resp, err)

	if len(resp) == 1 {
		w.Write([]byte("[]"))
		return
	}

	resp = resp[1:]

	//get advices
	cmds := make([]interface{}, len(resp)/2+2)
	cmds[0] = "multi_hget"
	cmds[1] = H_ADVICE
	for i, _ := range cmds {
		if i >= 2 {
			cmds[i] = resp[(i-2)*2]
		}
	}
	resp, err = ssdb.Do(cmds...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	advices := make([]Advice, len(resp)/2)
	for i, _ := range advices {
		js := resp[i*2+1]
		err = json.Unmarshal([]byte(js), &advices[i])
		lwutil.CheckError(err, "")
	}

	//out
	lwutil.WriteResponse(w, &advices)
}

func apiCheckPrivateFilesExist(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//in
	var in struct {
		FileKeys []string
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	out := map[string]bool{}

	entryPathes := []rs.EntryPath{}

	for _, key := range in.FileKeys {
		entryPath := rs.EntryPath{
			Bucket: USER_PRIVATE_UPLOAD_BUCKET,
			Key:    key,
		}
		entryPathes = append(entryPathes, entryPath)
	}

	rsCli := rs.New(nil)

	var batchStatRets []rs.BatchStatItemRet
	batchStatRets, err = rsCli.BatchStat(nil, entryPathes)
	for i, item := range batchStatRets {
		if item.Code == 200 {
			out[in.FileKeys[i]] = true
		}
	}

	//out
	lwutil.WriteResponse(w, out)
}

func regEtc() {
	http.Handle("/etc/betHelp", lwutil.ReqHandler(apiBetHelp))
	http.Handle("/etc/addAdvice", lwutil.ReqHandler(apiAddAdvice))
	http.Handle("/etc/listAdvice", lwutil.ReqHandler(apiListAdvice))
	http.Handle("/etc/checkPrivateFilesExist", lwutil.ReqHandler(apiCheckPrivateFilesExist))
	// http.Handle("/etc/getAppConf", lwutil.ReqHandler(apiGetAppConf))
}
