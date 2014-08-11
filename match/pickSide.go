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
	BUFF_PICK_SIDE_SERIAL     = "BUFF_PICK_SIDE_SERIAL"
	H_PICK_SIDE_EVENT         = "H_PICK_SIDE_EVENT"
	Z_PICK_SIDE_EVENT         = "H_PICK_SIDE_EVENT"
	K_PICK_SIDE_PUBLISH       = "K_PICK_SIDE_PUBLISH"
	Z_PICK_SIDE_QUESTION      = "Z_PICK_SIDE_QUESTION"
	H_PICK_SIDE_QUESTION      = "H_PICK_SIDE_QUESTION"
	PICK_SIDE_QUESTION_SERIAL = "PICK_SIDE_QUESTION_SERIAL"
)

type PickSideQuestion struct {
	Id       int64
	Question string
	Sides    []string
}

type PickSideEvent struct {
	Event
	Question string
	Sides    []string
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

	//pickSidePublishInfoes
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
			//pop from buff and push to pickSideEvents
			for i := 0; i < pubInfo.EventNum; i++ {
				//get front event
				resp, err := ssdbc.Do("zkeys", Z_EVENT_BUFF, "", "", "", 1)
				checkSsdbError(resp, err)
				if len(resp) <= 1 {
					glog.Error("Z_EVENT_BUFF empty!!!!")
					return
				}
				buffEventId, err := strconv.ParseInt(resp[1], 10, 64)
				checkError(err)

				//get event
				resp, err = ssdbc.Do("hget", H_EVENT_BUFF, buffEventId)
				checkSsdbError(resp, err)

				var event PickSideEvent
				err = json.Unmarshal([]byte(resp[1]), &event)
				checkError(err)

				//get question
				resp, err = ssdbc.Do("zkeys", Z_PICK_SIDE_QUESTION, "", "", "", 1)
				checkSsdbError(resp, err)
				if len(resp) <= 1 {
					glog.Error("Z_PICK_SIDE_QUESTION empty!!!!")
					return
				}
				questionId, err := strconv.ParseInt(resp[1], 10, 64)
				checkError(err)

				resp, err = ssdbc.Do("hget", H_PICK_SIDE_QUESTION, questionId)
				checkSsdbError(resp, err)

				var question PickSideQuestion
				err = json.Unmarshal([]byte(resp[1]), &question)
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
				resp, err = ssdbc.Do("zrscan", Z_PICK_SIDE_EVENT, "", "", "", 1)
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

				//set question
				event.Question = question.Question
				event.Sides = question.Sides

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

				//del eventBuff
				resp, err = ssdbc.Do("zdel", Z_EVENT_BUFF, buffEventId)
				checkSsdbError(resp, err)
				resp, err = ssdbc.Do("hdel", H_EVENT_BUFF, buffEventId)
				checkSsdbError(resp, err)

				//del question
				resp, err = ssdbc.Do("zdel", Z_PICK_SIDE_QUESTION, questionId)
				checkSsdbError(resp, err)
				resp, err = ssdbc.Do("hdel", H_PICK_SIDE_QUESTION, questionId)
				checkSsdbError(resp, err)

				//
				glog.Infof("Add event and challenge ok:id=%d", event.Id)
			}
		}
	}
}

func apiPickSideQuestionAdd(w http.ResponseWriter, r *http.Request) {
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
	var question PickSideQuestion
	err = lwutil.DecodeRequestBody(r, &question)
	lwutil.CheckError(err, "err_decode_body")

	//check input
	if len(question.Sides) < 2 {
		lwutil.SendError("err_input", "len(question.Sides < 2)")
	}

	//gen serial
	question.Id = GenSerial(ssdb, PICK_SIDE_QUESTION_SERIAL)

	//save to ssdb
	js, err := json.Marshal(question)
	lwutil.CheckError(err, "")
	resp, err := ssdb.Do("hset", H_PICK_SIDE_QUESTION, question.Id, js)
	lwutil.CheckSsdbError(resp, err)

	resp, err = ssdb.Do("zset", Z_PICK_SIDE_QUESTION, question.Id, question.Id)
	lwutil.CheckSsdbError(resp, err)

	//out
	lwutil.WriteResponse(w, question)
}

func apiPickSideQuestionList(w http.ResponseWriter, r *http.Request) {
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
	resp, err := ssdbc.Do("zkeys", Z_PICK_SIDE_QUESTION, "", "", "", 100)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]
	if len(resp) == 0 {
		lwutil.SendError("err_not_found", "")
	}

	//multi_hget
	keyNum := len(resp)
	cmds := make([]interface{}, keyNum+2)
	cmds[0] = "multi_hget"
	cmds[1] = H_PICK_SIDE_QUESTION
	for i := 0; i < keyNum; i++ {
		cmds[2+i] = resp[i]
	}
	resp, err = ssdbc.Do(cmds...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	//out
	num := len(resp) / 2
	out := make([]PickSideQuestion, num)
	for i := 0; i < num; i++ {
		err = json.Unmarshal([]byte(resp[i*2+1]), &out[i])
		lwutil.CheckError(err, "")
	}

	lwutil.WriteResponse(w, out)
}

func apiPickSideQuestionDel(w http.ResponseWriter, r *http.Request) {
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
	resp, err := ssdb.Do("hexists", H_PICK_SIDE_QUESTION, in.Id)
	lwutil.CheckError(err, "")
	if resp[1] != "1" {
		lwutil.SendError("err_exist", fmt.Sprintf("question not exist:id=", in.Id))
	}

	//del
	resp, err = ssdb.Do("zdel", Z_PICK_SIDE_QUESTION, in.Id)
	lwutil.CheckSsdbError(resp, err)
	resp, err = ssdb.Do("hdel", H_PICK_SIDE_QUESTION, in.Id)
	lwutil.CheckSsdbError(resp, err)

	lwutil.WriteResponse(w, in)
}

func apiPickSideQuestionMod(w http.ResponseWriter, r *http.Request) {
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
	var question PickSideQuestion
	err = lwutil.DecodeRequestBody(r, &question)
	lwutil.CheckError(err, "err_decode_body")

	//check exist
	resp, err := ssdb.Do("hget", H_PICK_SIDE_QUESTION, question.Id)
	if resp[0] == "not_found" {
		lwutil.SendError("err_not_found", "question not found from H_PICK_SIDE_QUESTION")
	}
	lwutil.CheckSsdbError(resp, err)

	//save to ssdb
	js, err := json.Marshal(question)
	lwutil.CheckError(err, "")
	resp, err = ssdb.Do("hset", H_PICK_SIDE_QUESTION, question.Id, js)
	lwutil.CheckSsdbError(resp, err)

	//out
	lwutil.WriteResponse(w, question)
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

func apiPickSideGetPublish(w http.ResponseWriter, r *http.Request) {
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

	//out
	lwutil.WriteResponse(w, _pickSidePublishInfoes)
}

func apiPickSideSetPublish(w http.ResponseWriter, r *http.Request) {
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
	var in []EventPublishInfo
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	for _, v := range in {
		if v.EventNum < 1 || v.EventNum > 3 {
			lwutil.SendError("err_input", "EventNum must between [1, 3]")
		}
	}

	_pickSidePublishInfoes = in

	//save
	js, err := json.Marshal(_pickSidePublishInfoes)
	lwutil.CheckError(err, "")
	resp, err := ssdb.Do("set", K_EVENT_PUBLISH, js)
	lwutil.CheckSsdbError(resp, err)

	//out
	lwutil.WriteResponse(w, in)
}

func apiPickSideAddQuestion(w http.ResponseWriter, r *http.Request) {
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
}

func apiPickSideListQuestion(w http.ResponseWriter, r *http.Request) {
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
}

func apiPickSideDelQuestion(w http.ResponseWriter, r *http.Request) {
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
}

func apiPickSideModQuestion(w http.ResponseWriter, r *http.Request) {
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
}

func regPickSide() {
	http.Handle("/pickSide/questionAdd", lwutil.ReqHandler(apiPickSideQuestionAdd))
	http.Handle("/pickSide/questionList", lwutil.ReqHandler(apiPickSideQuestionList))
	http.Handle("/pickSide/questionDel", lwutil.ReqHandler(apiPickSideQuestionDel))
	http.Handle("/pickSide/questionMod", lwutil.ReqHandler(apiPickSideQuestionMod))
	http.Handle("/pickSide/list", lwutil.ReqHandler(apiPickSideList))
	http.Handle("/pickSide/mod", lwutil.ReqHandler(apiPickSideMod))
	http.Handle("/pickSide/getPublish", lwutil.ReqHandler(apiPickSideGetPublish))
	http.Handle("/pickSide/setPublish", lwutil.ReqHandler(apiPickSideSetPublish))
	http.Handle("/pickSide/addQuestion", lwutil.ReqHandler(apiPickSideAddQuestion))
	http.Handle("/pickSide/listQuestion", lwutil.ReqHandler(apiPickSideListQuestion))
	http.Handle("/pickSide/delQuestion", lwutil.ReqHandler(apiPickSideDelQuestion))
	http.Handle("/pickSide/modQuestion", lwutil.ReqHandler(apiPickSideModQuestion))
}
