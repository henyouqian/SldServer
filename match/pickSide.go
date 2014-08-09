package main

import (
	"encoding/json"
	"fmt"
	"github.com/golang/glog"
	"github.com/henyouqian/lwutil"
	"net/http"
	"strconv"
	"time"
)

const (
	BUFF_PICK_SIDE_SERIAL  = "BUFF_PICK_SIDE_SERIAL"
	H_PICK_SIDE_EVENT_BUFF = "H_PICK_SIDE_EVENT_BUFF"
	Z_PICK_SIDE_EVENT_BUFF = "Z_PICK_SIDE_EVENT_BUFF"
	H_PICK_SIDE_EVENT      = "H_PICK_SIDE_EVENT"
	Z_PICK_SIDE_EVENT      = "H_PICK_SIDE_EVENT"
	K_PICK_SIDE_PUBLISH    = "K_PICK_SIDE_PUBLISH"
)

type PickSideEvent struct {
	Event
	Question string
	Sides    []string
}

type BuffPickSideEvent struct {
	Id            int64
	PackId        int64
	PackTitle     string
	SliderNum     int
	ChallengeSecs []int
	Question      string
	Sides         []string
}

var (
	_pickSidePublishInfoes []EventPublishInfo
)

func pickSideGlog() {
	glog.Info("")
}

func initPickSide() {
	//ssdb
	ssdbc, err := ssdbPool.Get()
	checkError(err)
	defer ssdbc.Close()

	//cron
	_cron.AddFunc("0 * * * * *", pickSidePublishTask)

	//eventPublishInfoes
	resp, err := ssdbc.Do("get", K_PICK_SIDE_PUBLISH)
	if err != nil || len(resp) <= 1 {
		_pickSidePublishInfoes = _conf.PickSidePublishInfoes
	} else {
		err = json.Unmarshal([]byte(resp[1]), &_pickSidePublishInfoes)
		checkError(err)
	}
}

func pickSidePublishTask() {
	defer handleError()

	//ssdb
	ssdbc, err := ssdbPool.Get()
	checkError(err)
	defer ssdbc.Close()

	//redis
	rc := redisPool.Get()
	defer rc.Close()

	//
	now := time.Now()
	for _, pubInfo := range _pickSidePublishInfoes {
		if pubInfo.PublishTime[0] == now.Hour() && pubInfo.PublishTime[1] == now.Minute() {
			//pop from Z_PICK_SIDE_EVENT_BUFF and push to Z_PICK_SIDE_EVENT
			for i := 0; i < pubInfo.EventNum; i++ {
				//get front event
				resp, err := ssdbc.Do("zkeys", Z_PICK_SIDE_EVENT_BUFF, "", "", "", 1)
				checkSsdbError(resp, err)
				if len(resp) <= 1 {
					glog.Error("Z_PICK_SIDE_EVENT_BUFF empty!!!!")
					return
				}
				buffEventId, err := strconv.ParseInt(resp[1], 10, 64)
				checkError(err)

				//del front event
				resp, err = ssdbc.Do("zdel", Z_PICK_SIDE_EVENT_BUFF, buffEventId)
				checkSsdbError(resp, err)

				//get event
				resp, err = ssdbc.Do("hget", H_PICK_SIDE_EVENT_BUFF, buffEventId)
				checkSsdbError(resp, err)

				var event PickSideEvent
				err = json.Unmarshal([]byte(resp[1]), &event)
				checkError(err)

				//fill event's begin and end time
				hour := pubInfo.BeginTime[0]
				addDay := hour / 24
				hour = hour % 24
				min := pubInfo.BeginTime[1]
				beginTime := time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, time.Local)
				if addDay > 0 {
					beginTime = beginTime.AddDate(0, 0, addDay)
				}
				event.BeginTime = beginTime.Unix()
				event.BeginTimeString = beginTime.Format(TIME_FORMAT)

				hour = pubInfo.EndTime[0]
				addDay = hour / 24
				hour = hour % 24
				min = pubInfo.EndTime[1]
				endTime := time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, time.Local)
				if addDay > 0 {
					endTime = endTime.AddDate(0, 0, addDay)
				}
				event.EndTime = endTime.Unix()
				event.EndTimeString = endTime.Format(TIME_FORMAT)

				//BetEndTime
				event.BetEndTime = event.EndTime - BET_CLOSE_BEFORE_END_SEC

				//change event id
				resp, err = ssdbc.Do("zrscan", Z_EVENT, "", "", "", 1)
				checkSsdbError(resp, err)
				if resp[0] == "not_found" || len(resp) == 1 {
					event.Id = 1
				} else {
					maxId, err := strconv.ParseInt(resp[1], 10, 64)
					checkError(err)
					event.Id = maxId + 1
				}

				// //init betting pool
				// key := makeEventBettingPoolKey(event.Id)
				// err = ssdbc.HSetMap(key, EVENT_INIT_BETTING_POOL)
				// lwutil.CheckError(err, "")

				//get pack
				pack, err := getPack(ssdbc, event.PackId)
				lwutil.CheckError(err, fmt.Sprintf("packId:%d", event.PackId))

				event.Thumb = pack.Thumb
				event.PackTitle = pack.Title

				//save event
				bts, err := json.Marshal(event)
				checkError(err)
				resp, err = ssdbc.Do("hset", H_PICK_SIDE_EVENT, event.Id, bts)
				checkSsdbError(resp, err)

				//push to Z_EVENT
				resp, err = ssdbc.Do("zset", Z_PICK_SIDE_EVENT, event.Id, event.Id)
				checkSsdbError(resp, err)

				//push to Z_CHALLENGE
				challengeId := int64(1)
				resp, err = ssdbc.Do("zrrange", Z_CHALLENGE, 0, 1)
				lwutil.CheckSsdbError(resp, err)
				if len(resp) > 1 {
					challengeId, err = strconv.ParseInt(resp[1], 10, 32)
					lwutil.CheckError(err, "")
					challengeId++
				}

				challenge := Challenge{
					Id:               challengeId,
					PackId:           event.PackId,
					PackTitle:        event.PackTitle,
					Thumb:            event.Thumb,
					SliderNum:        event.SliderNum,
					ChallengeSecs:    event.ChallengeSecs,
					ChallengeRewards: _conf.ChallengeRewards,
				}
				resp, err = ssdbc.Do("zset", Z_CHALLENGE, challengeId, challengeId)
				checkSsdbError(resp, err)

				//add to H_CHALLENGE
				bts, err = json.Marshal(challenge)
				checkError(err)
				resp, err = ssdbc.Do("hset", H_CHALLENGE, challenge.Id, bts)
				checkSsdbError(resp, err)

				//
				glog.Infof("Add event and challenge ok:id=%d", event.Id)
			}
		}
	}
}

func apiPickSideBuffAdd(w http.ResponseWriter, r *http.Request) {
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
	var buffEvent BuffPickSideEvent
	err = lwutil.DecodeRequestBody(r, &buffEvent)
	lwutil.CheckError(err, "err_decode_body")

	//check input
	if len(buffEvent.ChallengeSecs) != 3 {
		lwutil.SendError("err_input", "len(buffEvent.ChallengeSecs) != 3")
	}

	if len(buffEvent.Sides) < 2 {
		lwutil.SendError("err_input", "len(buffEvent.Sides < 2)")
	}

	//sliderNum
	if buffEvent.SliderNum <= 0 {
		buffEvent.SliderNum = 5
	} else if buffEvent.SliderNum > 10 {
		buffEvent.SliderNum = 10
	}

	//gen serial
	buffEvent.Id = GenSerial(ssdb, BUFF_PICK_SIDE_SERIAL)

	//get pack
	pack, err := getPack(ssdb, buffEvent.PackId)
	lwutil.CheckError(err, "")

	//
	buffEvent.PackTitle = pack.Title

	//save to ssdb
	js, err := json.Marshal(buffEvent)
	lwutil.CheckError(err, "")
	resp, err := ssdb.Do("hset", H_PICK_SIDE_EVENT_BUFF, buffEvent.Id, js)
	lwutil.CheckSsdbError(resp, err)

	resp, err = ssdb.Do("zset", Z_PICK_SIDE_EVENT_BUFF, buffEvent.Id, buffEvent.Id)
	lwutil.CheckSsdbError(resp, err)

	//out
	lwutil.WriteResponse(w, buffEvent)
}

func apiPickSideBuffList(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")
	checkAdmin(session)

	//zkeys
	resp, err := ssdbc.Do("zkeys", Z_PICK_SIDE_EVENT_BUFF, "", "", "", 100)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]
	if len(resp) == 0 {
		lwutil.SendError("err_not_found", "")
	}

	//multi_hget
	keyNum := len(resp)
	cmds := make([]interface{}, keyNum+2)
	cmds[0] = "multi_hget"
	cmds[1] = H_PICK_SIDE_EVENT_BUFF
	for i := 0; i < keyNum; i++ {
		cmds[2+i] = resp[i]
	}
	resp, err = ssdbc.Do(cmds...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	//out
	eventNum := len(resp) / 2
	out := make([]BuffPickSideEvent, eventNum)
	for i := 0; i < eventNum; i++ {
		err = json.Unmarshal([]byte(resp[i*2+1]), &out[i])
		lwutil.CheckError(err, "")
	}

	lwutil.WriteResponse(w, out)
}

func apiPickSideBuffDel(w http.ResponseWriter, r *http.Request) {
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
		Id int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//check exist
	resp, err := ssdb.Do("hexists", H_PICK_SIDE_EVENT_BUFF, in.Id)
	lwutil.CheckError(err, "")
	if resp[1] != "1" {
		lwutil.SendError("err_exist", fmt.Sprintf("buffEvent not exist:id=", in.Id))
	}

	//del
	resp, err = ssdb.Do("zdel", Z_PICK_SIDE_EVENT_BUFF, in.Id)
	lwutil.CheckSsdbError(resp, err)
	resp, err = ssdb.Do("hdel", H_PICK_SIDE_EVENT_BUFF, in.Id)
	lwutil.CheckSsdbError(resp, err)

	lwutil.WriteResponse(w, in)
}

func apiPickSideBuffMod(w http.ResponseWriter, r *http.Request) {
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
	var event BuffPickSideEvent
	err = lwutil.DecodeRequestBody(r, &event)
	lwutil.CheckError(err, "err_decode_body")

	//check exist
	resp, err := ssdb.Do("hget", H_PICK_SIDE_EVENT_BUFF, event.Id)
	if resp[0] == "not_found" {
		lwutil.SendError("err_not_found", "event not found from H_PICK_SIDE_EVENT_BUFF")
	}
	lwutil.CheckSsdbError(resp, err)

	//save to ssdb
	js, err := json.Marshal(event)
	lwutil.CheckError(err, "")
	resp, err = ssdb.Do("hset", H_PICK_SIDE_EVENT_BUFF, event.Id, js)
	lwutil.CheckSsdbError(resp, err)

	//out
	lwutil.WriteResponse(w, event)
}

func apiPickSideList(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//session
	_, err = findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var in struct {
		StartId uint32
		Limit   uint32
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")
	if in.Limit > 50 {
		in.Limit = 50
	}

	var startId interface{}
	if in.StartId == 0 {
		startId = ""
	} else {
		startId = in.StartId
	}

	//zrscan
	resp, err := ssdb.Do("zrscan", Z_PICK_SIDE_EVENT, startId, startId, "", in.Limit)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]
	if len(resp) == 0 {
		w.Write([]byte("[]"))
		return
	}

	//multi_hget
	keyNum := len(resp) / 2
	cmds := make([]interface{}, keyNum+2)
	cmds[0] = "multi_hget"
	cmds[1] = H_PICK_SIDE_EVENT
	for i := 0; i < keyNum; i++ {
		cmds[2+i] = resp[i*2]
	}
	resp, err = ssdb.Do(cmds...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	//out
	eventNum := len(resp) / 2
	out := make([]PickSideEvent, eventNum)
	for i := 0; i < eventNum; i++ {
		err = json.Unmarshal([]byte(resp[i*2+1]), &out[i])
		lwutil.CheckError(err, "")
	}

	lwutil.WriteResponse(w, out)
}

func apiPickSideMod(w http.ResponseWriter, r *http.Request) {
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
	var event PickSideEvent
	err = lwutil.DecodeRequestBody(r, &event)
	lwutil.CheckError(err, "err_decode_body")

	//get pack
	resp, err := ssdb.Do("hget", H_PACK, event.PackId)
	lwutil.CheckSsdbError(resp, err)
	var pack Pack
	err = json.Unmarshal([]byte(resp[1]), &pack)
	lwutil.CheckError(err, "")
	event.Thumb = pack.Thumb
	event.PackTitle = pack.Title

	//check exist
	resp, err = ssdb.Do("hget", H_PICK_SIDE_EVENT, event.Id)
	if resp[0] == "not_found" {
		lwutil.SendError("err_not_found", "event not found from H_PICK_SIDE_EVENT")
	}
	lwutil.CheckSsdbError(resp, err)

	//
	calcEventTimes(&event.Event)

	//save to ssdb
	js, err := json.Marshal(event)
	lwutil.CheckError(err, "")
	resp, err = ssdb.Do("hset", H_PICK_SIDE_EVENT, event.Id, js)
	lwutil.CheckSsdbError(resp, err)

	//out
	lwutil.WriteResponse(w, event)
}

func regPickSide() {
	http.Handle("/pickSide/buffAdd", lwutil.ReqHandler(apiPickSideBuffAdd))
	http.Handle("/pickSide/buffList", lwutil.ReqHandler(apiPickSideBuffList))
	http.Handle("/pickSide/buffDel", lwutil.ReqHandler(apiPickSideBuffDel))
	http.Handle("/pickSide/buffMod", lwutil.ReqHandler(apiPickSideBuffMod))
	http.Handle("/pickSide/list", lwutil.ReqHandler(apiPickSideList))
	http.Handle("/pickSide/mod", lwutil.ReqHandler(apiPickSideMod))
	// http.Handle("/pickSide/setPublish", lwutil.ReqHandler(apiPickSideSetPublish))
	// http.Handle("/pickSide/getPublish", lwutil.ReqHandler(apiPickSideGetPublish))
}
