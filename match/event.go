package main

import (
	"./ssdb"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/garyburd/redigo/redis"
	"github.com/golang/glog"
	"github.com/henyouqian/lwutil"
	"github.com/robfig/cron"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"time"
)

const (
	H_EVENT                     = "H_EVENT"
	H_EVENT_BUFF                = "H_EVENT_BUFF"
	H_EVENT_PLAYER_RECORD       = "H_EVENT_PLAYER_RECORD" //subkey:(int)eventId
	Z_EVENT_PLAYER_RECORD       = "Z_EVENT_PLAYER_RECORD" //key:Z_EVENT_PLAYER_RECORD/<(int64)userId> subkey:(int)eventId score:(int)eventId
	RDS_Z_EVENT_LEADERBOARD_PRE = "RDS_Z_EVENT_LEADERBOARD_PRE"
	H_EVENT_RANK                = "H_EVENT_RANK" //subkey:(uint32)rank value:(uint64)userId
	EVENT_SERIAL                = "EVENT_SERIAL"
	TRY_COST_MONEY              = 100
	TRY_EXPIRE_SECONDS          = 600
	TEAM_CHAMPIONSHIP_ROUND_NUM = 6
	H_EVENT_TEAM_SCORE          = "H_EVENT_TEAM_SCORE"   //subkey:eventId value:map[(string)teamName](int)score
	H_EVENT_BETTING_POOL        = "H_EVENT_BETTING_POOL" //key:H_EVENT_BETTING_POOL/eventId subkey:(string)teamName value:(int64)money
	INIT_GAME_COIN_NUM          = 3
	BET_CLOSE_BEFORE_END_SEC    = 60 * 60
	H_EVENT_TEAM_PLAYER_BET     = "H_EVENT_TEAM_PLAYER_BET" //key:H_EVENT_TEAM_PLAYER_BET/eventId/teamName subKey:playerId value:betMoney
	TIME_FORMAT                 = "2006-01-02T15:04"
	TEAM_SCORE_RANK_MAX         = 100
)

var (
	Z_EVENT            = "Z_EVENT"
	Z_EVENT_BUFF       = "Z_EVENT_BUFF"
	EventPublishInfoes []EventPublishInfo

	DEFAULT_CHALLENGE_REWARDS = []int{100, 50, 50} //gold, silver, bronze. additive.

	EVENT_TYPES = map[string]int{
		"PERSONAL_RANK":     1,
		"TEAM_CHAMPIONSHIP": 1,
	}

	TEAM_NAMES              = []string{"安徽", "澳门", "北京", "重庆", "福建", "甘肃", "广东", "广西", "贵州", "海南", "河北", "黑龙江", "河南", "湖北", "湖南", "江苏", "江西", "吉林", "辽宁", "内蒙古", "宁夏", "青海", "陕西", "山东", "上海", "山西", "四川", "台湾", "天津", "香港", "新疆", "西藏", "云南", "浙江"}
	EVENT_INIT_BETTING_POOL = map[string]interface{}{}
	INIT_BET_MONEY          = int64(10000)

	_cron cron.Cron
)

type Event struct {
	Type             string //"PERSONAL_RANK", "TEAM_CHAMPIONSHIP"
	Id               int64
	PackId           int64
	PackTimeUnix     int64
	Thumb            string
	BeginTime        int64
	EndTime          int64
	BeginTimeString  string
	EndTimeString    string
	BetEndTime       int64
	HasResult        bool
	SliderNum        int
	ChallengeSecs    []int
	ChallengeRewards []int
}

type EventPlayerRecord struct {
	EventId            int64
	PlayerName         string
	TeamName           string
	Secret             string
	SecretExpire       int64
	Trys               int
	HighScore          int
	HighScoreTime      int64
	FinalRank          int
	GravatarKey        string
	CustomAvartarKey   string
	Gender             int
	GameCoinNum        int
	ChallengeHighScore int
	CupType            int
	MatchReward        int64
	BetReward          int64
	Bet                map[string]int64 //[teamName]betMoney
	BetMoneySum        int64
	PackThumbKey       string
}

type EventPublishInfo struct {
	PublishTime [2]int
	BeginTime   [2]int
	EndTime     [2]int
	EventNum    int
}

func makeRedisLeaderboardKey(evnetId int64) string {
	return fmt.Sprintf("%s/%d", RDS_Z_EVENT_LEADERBOARD_PRE, evnetId)
}

func makeHashEventRankKey(eventId int64) string {
	return fmt.Sprintf("%s/%d", H_EVENT_RANK, eventId)
}

func makeEventPlayerRecordSubkey(eventId int64, userId int64) string {
	key := fmt.Sprintf("%d/%d", eventId, userId)
	return key
}

func makeEventBettingPoolKey(eventId int64) string {
	return fmt.Sprintf("%s/%d", H_EVENT_BETTING_POOL, eventId)
}

func makeEventTeamPlayerBetKey(eventId int64, teamName string) string {
	return fmt.Sprintf("%s/%d/%s", H_EVENT_TEAM_PLAYER_BET, eventId, teamName)
}

func init() {
	glog.Info("match init")
	for _, teamName := range TEAM_NAMES {
		EVENT_INIT_BETTING_POOL[teamName] = INIT_BET_MONEY
	}

	//cron
	_cron.AddFunc("0 * * * * *", eventPublishTask)
	_cron.Start()
}

func eventPublishTask() {
	defer handleError()

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//
	now := time.Now()
	for _, pubInfo := range EventPublishInfoes {
		if pubInfo.PublishTime[0] == now.Hour() && pubInfo.PublishTime[1] == now.Minute() {
			//pop from Z_EVENT_BUFF and push to Z_EVENT
			for i := 0; i < pubInfo.EventNum; i++ {
				//get front event
				resp, err := ssdbc.Do("zkeys", Z_EVENT_BUFF, "", "", "", 1)
				if err != nil || resp[0] != ssdb.OK || len(resp) <= 1 {
					glog.Errorln(err, resp)
					return
				}
				eventId, err := strconv.ParseInt(resp[1], 10, 64)
				if err != nil {
					glog.Errorln(err)
					return
				}

				//del front event
				resp, err = ssdbc.Do("zdel", Z_EVENT_BUFF, eventId)
				if err != nil || resp[0] != ssdb.OK {
					glog.Errorln(err, resp[0])
					return
				}

				//get event
				resp, err = ssdbc.Do("hget", H_EVENT_BUFF, eventId)
				if err != nil || resp[0] != ssdb.OK {
					glog.Errorln(err, resp[0])
					return
				}

				var event Event
				err = json.Unmarshal([]byte(resp[1]), &event)
				if err != nil {
					glog.Errorln(err)
					return
				}

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
				lwutil.CheckError(err, "")
				if resp[0] == "not_found" {
					event.Id = 1
				} else {
					maxId, err := strconv.ParseInt(resp[1], 10, 64)
					lwutil.CheckError(err, "")
					event.Id = maxId + 1
				}

				//get pack
				pack := getPack(ssdbc, event.PackId)

				event.Thumb = pack.Thumb
				event.PackTimeUnix = pack.TimeUnix

				//add event id to pack
				pack.EventIds[fmt.Sprintf("%d", event.Id)] = true

				//save pack
				savePack(ssdbc, pack)

				//save event
				bts, err := json.Marshal(event)
				if err != nil {
					glog.Errorln(err)
					return
				}
				resp, err = ssdbc.Do("hset", H_EVENT, eventId, bts)
				if err != nil || resp[0] != ssdb.OK {
					glog.Errorln(err, resp[0])
					return
				}

				//push to Z_EVENT
				resp, err = ssdbc.Do("zset", Z_EVENT, eventId, eventId)
				if err != nil || resp[0] != ssdb.OK {
					glog.Errorln(err)
					return
				}
			}
		}
	}
}

func getEvent(ssdb *ssdb.Client, eventId int64) *Event {
	resp, err := ssdb.Do("hget", H_EVENT, eventId)
	lwutil.CheckSsdbError(resp, err)

	event := Event{}
	err = json.Unmarshal([]byte(resp[1]), &event)
	lwutil.CheckError(err, "")
	return &event
}

func saveEvent(ssdb *ssdb.Client, event *Event) {
	js, err := json.Marshal(event)
	lwutil.CheckError(err, "")

	resp, err := ssdb.Do("hset", H_EVENT, event.Id, js)
	lwutil.CheckSsdbError(resp, err)
}

func getEventFromBuff(ssdb *ssdb.Client, eventId int64) *Event {
	resp, err := ssdb.Do("hget", H_EVENT_BUFF, eventId)
	lwutil.CheckSsdbError(resp, err)

	event := Event{}
	err = json.Unmarshal([]byte(resp[1]), &event)
	lwutil.CheckError(err, "")
	return &event
}

func isEventRunning(event *Event) bool {
	if event.HasResult {
		return true
	}
	now := lwutil.GetRedisTimeUnix()
	if now >= event.BeginTime && now < event.EndTime {
		return true
	}
	return false
}

func getEventPlayerRecord(ssdb *ssdb.Client, eventId int64, userId int64) *EventPlayerRecord {
	key := makeEventPlayerRecordSubkey(eventId, userId)
	resp, err := ssdb.Do("hget", H_EVENT_PLAYER_RECORD, key)
	lwutil.CheckError(err, "")
	var record EventPlayerRecord
	if resp[0] == "ok" {
		err = json.Unmarshal([]byte(resp[1]), &record)
		lwutil.CheckError(err, "")
		return &record
	} else { //create record
		playerInfo, err := getPlayerInfo(ssdb, userId)
		lwutil.CheckError(err, "")

		record.EventId = eventId
		record.Trys = 0
		record.PlayerName = playerInfo.NickName
		record.TeamName = playerInfo.TeamName
		record.GravatarKey = playerInfo.GravatarKey
		record.CustomAvartarKey = playerInfo.CustomAvatarKey
		record.GameCoinNum = INIT_GAME_COIN_NUM

		//get event
		event := getEvent(ssdb, eventId)
		pack := getPack(ssdb, event.PackId)
		record.PackThumbKey = pack.Thumb

		js, err := json.Marshal(record)
		resp, err = ssdb.Do("hset", H_EVENT_PLAYER_RECORD, key, js)
		lwutil.CheckSsdbError(resp, err)
		return &record
	}
}

func saveEventPlayerRecord(ssdb *ssdb.Client, eventId int64, userId int64, record *EventPlayerRecord) {
	key := makeEventPlayerRecordSubkey(eventId, userId)
	js, err := json.Marshal(record)
	resp, err := ssdb.Do("hset", H_EVENT_PLAYER_RECORD, key, js)
	lwutil.CheckSsdbError(resp, err)
}

func shuffleArray(src []uint32) []uint32 {
	dest := make([]uint32, len(src))
	rand.Seed(time.Now().UTC().UnixNano())
	perm := rand.Perm(len(src))
	for i, v := range perm {
		dest[v] = src[i]
	}
	return dest
}

func calcEventTimes(event *Event) {
	now := lwutil.GetRedisTimeUnix()

	t, err := time.ParseInLocation(TIME_FORMAT, event.BeginTimeString, time.Local)
	lwutil.CheckError(err, "")
	event.BeginTime = t.Unix()
	if event.BeginTime < now {
		//lwutil.SendError("err_time", "BeginTime must larger than now")
	}

	t, err = time.ParseInLocation(TIME_FORMAT, event.EndTimeString, time.Local)
	lwutil.CheckError(err, "")
	event.EndTime = t.Unix()
	if event.EndTime < now {
		//lwutil.SendError("err_time", "EndTime must larger than now")
	}

	//check timePotins
	if event.BeginTime >= event.EndTime {
		lwutil.SendError("err_time", "event.BeginTime >= event.EndTime")
	}

	//BetEndTime
	if event.BetEndTime == 0 {
		event.BetEndTime = event.EndTime - BET_CLOSE_BEFORE_END_SEC
	}
}

func apiNewEvent(w http.ResponseWriter, r *http.Request) {
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
	var event struct {
		Event
		Force bool
	}
	err = lwutil.DecodeRequestBody(r, &event)
	lwutil.CheckError(err, "err_decode_body")
	event.Type = "PERSONAL_RANK"
	if _, ok := EVENT_TYPES[event.Type]; ok == false {
		lwutil.SendError("err_match_type", "")
	}
	event.HasResult = false

	//
	calcEventTimes(&event.Event)

	//sliderNum
	if event.SliderNum == 0 {
		event.SliderNum = 6
	} else if event.SliderNum > 10 {
		event.SliderNum = 10
	}

	//challengeRewards
	event.ChallengeRewards = _conf.ChallengeRewards

	//gen serial
	if event.Force {
		resp, err := ssdb.Do("hget", H_EVENT, event.Id)
		if resp[0] != "not_found" {
			lwutil.SendError("err_exist", "Force == true, but event exist")
		}
		lwutil.CheckError(err, "")
	} else {
		//event.Id = GenSerial(ssdb, EVENT_SERIAL)
		resp, err := ssdb.Do("zrscan", Z_EVENT, "", "", "", 1)
		lwutil.CheckError(err, "")
		if resp[0] == "not_found" {
			event.Id = 1
		} else {
			maxId, err := strconv.ParseInt(resp[1], 10, 64)
			lwutil.CheckError(err, "")
			event.Id = maxId + 1
		}
	}

	//get pack
	resp, err := ssdb.Do("hget", H_PACK, event.PackId)
	if resp[0] == "not_found" {
		lwutil.SendError("err_pack_not_found", "")
	}
	lwutil.CheckSsdbError(resp, err)
	var pack Pack
	err = json.Unmarshal([]byte(resp[1]), &pack)
	lwutil.CheckError(err, "")
	event.Thumb = pack.Thumb
	event.PackTimeUnix = pack.TimeUnix

	//add event id to pack
	pack.EventIds[fmt.Sprintf("%d", event.Id)] = true

	//save pack
	savePack(ssdb, &pack)

	//save to ssdb
	js, err := json.Marshal(event)
	lwutil.CheckError(err, "")
	resp, err = ssdb.Do("hset", H_EVENT, event.Id, js)
	lwutil.CheckSsdbError(resp, err)

	resp, err = ssdb.Do("zset", Z_EVENT, event.Id, event.Id)
	lwutil.CheckSsdbError(resp, err)

	//init betting pool
	key := makeEventBettingPoolKey(event.Id)
	err = ssdb.HSetMap(key, EVENT_INIT_BETTING_POOL)
	lwutil.CheckError(err, "")

	//out
	lwutil.WriteResponse(w, event)
}

func apiModEvent(w http.ResponseWriter, r *http.Request) {
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
	var event Event
	err = lwutil.DecodeRequestBody(r, &event)
	lwutil.CheckError(err, "err_decode_body")
	if _, ok := EVENT_TYPES[event.Type]; ok == false {
		lwutil.SendError("err_match_type", "")
	}

	//get pack
	resp, err := ssdb.Do("hget", H_PACK, event.PackId)
	lwutil.CheckSsdbError(resp, err)
	var pack Pack
	err = json.Unmarshal([]byte(resp[1]), &pack)
	lwutil.CheckError(err, "")
	event.Thumb = pack.Thumb
	event.PackTimeUnix = pack.TimeUnix

	//check exist
	resp, err = ssdb.Do("hget", H_EVENT, event.Id)
	if resp[0] == "not_found" {
		lwutil.SendError("err_not_found", "event not found from H_EVENT")
	}
	lwutil.CheckSsdbError(resp, err)

	//
	calcEventTimes(&event)

	//save to ssdb
	js, err := json.Marshal(event)
	lwutil.CheckError(err, "")
	resp, err = ssdb.Do("hset", H_EVENT, event.Id, js)
	lwutil.CheckSsdbError(resp, err)

	//out
	lwutil.WriteResponse(w, event)
}

func apiDelEvent(w http.ResponseWriter, r *http.Request) {
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
		EventId int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//del
	resp, err := ssdb.Do("zdel", Z_EVENT, in.EventId)
	lwutil.CheckSsdbError(resp, err)
	resp, err = ssdb.Do("hdel", H_EVENT, in.EventId)
	lwutil.CheckSsdbError(resp, err)

	lwutil.WriteResponse(w, in)
}

func apiListEvent(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//session
	session, err := findSession(w, r, nil)
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
	resp, err := ssdb.Do("zrscan", Z_EVENT, startId, startId, "", in.Limit)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]
	if len(resp) == 0 {
		glog.Errorf("startId=%d", startId)
		lwutil.SendError("err_not_found", fmt.Sprintf("startId=%d", startId))
	}

	//multi_hget
	keyNum := len(resp) / 2
	cmds := make([]interface{}, keyNum+2)
	cmds[0] = "multi_hget"
	cmds[1] = H_EVENT
	for i := 0; i < keyNum; i++ {
		cmds[2+i] = resp[i*2]
	}
	resp, err = ssdb.Do(cmds...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	//out
	type OutEvent struct {
		Event
		CupType int
	}

	eventNum := len(resp) / 2
	out := make([]OutEvent, eventNum)
	for i := 0; i < eventNum; i++ {
		err = json.Unmarshal([]byte(resp[i*2+1]), &out[i])
		lwutil.CheckError(err, "")
	}

	//event index map:
	//map[recordKey:string]eventIndexInOut:int
	idxMap := map[string]int{}
	for i := 0; i < eventNum; i++ {
		recordKey := makeEventPlayerRecordSubkey(out[i].Id, session.Userid)
		idxMap[recordKey] = i
	}

	//cup type
	cmds = make([]interface{}, 0, eventNum+2)
	cmds = append(cmds, "multi_hget")
	cmds = append(cmds, H_EVENT_PLAYER_RECORD)

	for i := 0; i < eventNum; i++ {
		recordKey := makeEventPlayerRecordSubkey(out[i].Id, session.Userid)
		cmds = append(cmds, recordKey)
	}

	//get event player record
	resp, err = ssdb.Do(cmds...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	var record EventPlayerRecord
	recordNum := len(resp) / 2
	for i := 0; i < recordNum; i++ {
		recordKey := resp[i*2]
		err = json.Unmarshal([]byte(resp[i*2+1]), &record)
		lwutil.CheckError(err, "")

		out[idxMap[recordKey]].CupType = record.CupType
	}

	//challengeRewards
	for i := 0; i < eventNum; i++ {
		if out[i].ChallengeRewards == nil {
			out[i].ChallengeRewards = DEFAULT_CHALLENGE_REWARDS
		}
	}

	lwutil.WriteResponse(w, out)
}

func apiRevListEvent(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//session
	session, err := findSession(w, r, nil)
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
	resp, err := ssdb.Do("zscan", Z_EVENT, startId, startId, "", in.Limit)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]
	if len(resp) == 0 {
		lwutil.SendError("err_not_found", "")
	}

	//multi_hget
	keyNum := len(resp) / 2
	cmds := make([]interface{}, keyNum+2)
	cmds[0] = "multi_hget"
	cmds[1] = H_EVENT
	for i := 0; i < keyNum; i++ {
		cmds[2+i] = resp[i*2]
	}
	resp, err = ssdb.Do(cmds...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	//out
	type OutEvent struct {
		Event
		CupType int
	}

	eventNum := len(resp) / 2
	out := make([]OutEvent, eventNum)
	for i := 0; i < eventNum; i++ {
		err = json.Unmarshal([]byte(resp[i*2+1]), &out[i])
		lwutil.CheckError(err, "")
	}

	//event index map:
	//map[recordKey:string]eventIndexInOut:int
	idxMap := map[string]int{}
	for i := 0; i < eventNum; i++ {
		recordKey := makeEventPlayerRecordSubkey(out[i].Id, session.Userid)
		idxMap[recordKey] = i
	}

	//cup type
	cmds = make([]interface{}, 0, eventNum+2)
	cmds = append(cmds, "multi_hget")
	cmds = append(cmds, H_EVENT_PLAYER_RECORD)

	for i := 0; i < eventNum; i++ {
		recordKey := makeEventPlayerRecordSubkey(out[i].Id, session.Userid)
		cmds = append(cmds, recordKey)
	}

	//get event player record
	resp, err = ssdb.Do(cmds...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	var record EventPlayerRecord
	recordNum := len(resp) / 2
	for i := 0; i < recordNum; i++ {
		recordKey := resp[i*2]
		err = json.Unmarshal([]byte(resp[i*2+1]), &record)
		lwutil.CheckError(err, "")

		out[idxMap[recordKey]].CupType = record.CupType
	}

	//challengeRewards
	for i := 0; i < eventNum; i++ {
		if out[i].ChallengeRewards == nil {
			out[i].ChallengeRewards = DEFAULT_CHALLENGE_REWARDS
		}
	}

	lwutil.WriteResponse(w, out)
}

func apiGetEvent(w http.ResponseWriter, r *http.Request) {
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
		EventId uint32
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//hget
	resp, err := ssdb.Do("hget", H_EVENT, in.EventId)
	lwutil.CheckSsdbError(resp, err)

	//out
	out := Event{}
	err = json.Unmarshal([]byte(resp[1]), &out)
	lwutil.CheckError(err, "")

	lwutil.WriteResponse(w, out)
}

func apiAddEventToBuff(w http.ResponseWriter, r *http.Request) {
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
	var event Event
	err = lwutil.DecodeRequestBody(r, &event)
	lwutil.CheckError(err, "err_decode_body")
	event.Type = "PERSONAL_RANK"
	if _, ok := EVENT_TYPES[event.Type]; ok == false {
		lwutil.SendError("err_match_type", "")
	}
	event.HasResult = false

	//sliderNum
	if event.SliderNum <= 0 {
		event.SliderNum = 5
	} else if event.SliderNum > 10 {
		event.SliderNum = 10
	}

	//gen serial
	event.Id = GenSerial(ssdb, EVENT_SERIAL)

	//save to ssdb
	js, err := json.Marshal(event)
	lwutil.CheckError(err, "")
	resp, err := ssdb.Do("hset", H_EVENT_BUFF, event.Id, js)
	lwutil.CheckSsdbError(resp, err)

	resp, err = ssdb.Do("zset", Z_EVENT_BUFF, event.Id, event.Id)
	lwutil.CheckSsdbError(resp, err)

	//init betting pool
	key := makeEventBettingPoolKey(event.Id)
	err = ssdb.HSetMap(key, EVENT_INIT_BETTING_POOL)
	lwutil.CheckError(err, "")

	//out
	lwutil.WriteResponse(w, event)
}

func apiListEventBuff(w http.ResponseWriter, r *http.Request) {
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
	resp, err := ssdbc.Do("zkeys", Z_EVENT_BUFF, "", "", "", 100)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]
	if len(resp) == 0 {
		lwutil.SendError("err_not_found", "")
	}

	//multi_hget
	keyNum := len(resp)
	cmds := make([]interface{}, keyNum+2)
	cmds[0] = "multi_hget"
	cmds[1] = H_EVENT_BUFF
	for i := 0; i < keyNum; i++ {
		cmds[2+i] = resp[i]
	}
	resp, err = ssdbc.Do(cmds...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	//out
	eventNum := len(resp) / 2
	out := make([]Event, eventNum)
	for i := 0; i < eventNum; i++ {
		err = json.Unmarshal([]byte(resp[i*2+1]), &out[i])
		lwutil.CheckError(err, "")
	}

	lwutil.WriteResponse(w, out)
}

func apiDelEventFromBuff(w http.ResponseWriter, r *http.Request) {
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
		EventId int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//del
	resp, err := ssdb.Do("zdel", Z_EVENT_BUFF, in.EventId)
	lwutil.CheckSsdbError(resp, err)
	resp, err = ssdb.Do("hdel", H_EVENT_BUFF, in.EventId)
	lwutil.CheckSsdbError(resp, err)

	lwutil.WriteResponse(w, in)
}

func apiModEventInBuff(w http.ResponseWriter, r *http.Request) {
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
	var event Event
	err = lwutil.DecodeRequestBody(r, &event)
	lwutil.CheckError(err, "err_decode_body")
	if _, ok := EVENT_TYPES[event.Type]; ok == false {
		lwutil.SendError("err_match_type", "")
	}

	//get pack
	resp, err := ssdb.Do("hget", H_PACK, event.PackId)
	lwutil.CheckSsdbError(resp, err)
	var pack Pack
	err = json.Unmarshal([]byte(resp[1]), &pack)
	lwutil.CheckError(err, "")
	event.Thumb = pack.Thumb
	event.PackTimeUnix = pack.TimeUnix

	//check exist
	resp, err = ssdb.Do("hget", H_EVENT_BUFF, event.Id)
	if resp[0] == "not_found" {
		lwutil.SendError("err_not_found", "event not found from H_EVENT_BUFF")
	}
	lwutil.CheckSsdbError(resp, err)

	//save to ssdb
	js, err := json.Marshal(event)
	lwutil.CheckError(err, "")
	resp, err = ssdb.Do("hset", H_EVENT_BUFF, event.Id, js)
	lwutil.CheckSsdbError(resp, err)

	//out
	lwutil.WriteResponse(w, event)
}

func apiGetUserPlay(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var in struct {
		EventId int64
		UserId  int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if in.UserId == 0 {
		in.UserId = session.Userid
	}

	//get event info
	resp, err := ssdb.Do("hget", H_EVENT, in.EventId)
	lwutil.CheckSsdbError(resp, err)
	event := Event{}
	err = json.Unmarshal([]byte(resp[1]), &event)
	lwutil.CheckError(err, "")

	//Out
	type Out struct {
		HighScore          int
		Trys               int
		Rank               int
		RankNum            int
		TeamName           string
		GameCoinNum        int
		ChallengeHighScore int
		CupType            int
		MatchReward        int64
		BetReward          int64
		Bet                map[string]int64 //[teamName]betMoney
		BetMoneySum        int64
	}

	//event play record
	record := getEventPlayerRecord(ssdb, in.EventId, in.UserId)

	//rank and rankNum
	rank := 0
	rankNum := 0

	if event.HasResult {
		rank = record.FinalRank
		//rankNum
		hRankKey := makeHashEventRankKey(in.EventId)
		resp, err = ssdb.Do("hsize", hRankKey)
		lwutil.CheckSsdbError(resp, err)
		rankNum, err = strconv.Atoi(resp[1])
		lwutil.CheckError(err, "")
	} else {
		//redis
		rc := redisPool.Get()
		defer rc.Close()

		//get rank
		eventLbLey := makeRedisLeaderboardKey(in.EventId)
		rc.Send("ZREVRANK", eventLbLey, in.UserId)
		rc.Send("ZCARD", eventLbLey)
		err = rc.Flush()
		lwutil.CheckError(err, "")
		rank, err = _redisInt(rc.Receive())
		rank += 1
		lwutil.CheckError(err, "")
		rankNum, err = _redisInt(rc.Receive())
		lwutil.CheckError(err, "")
	}

	// if record.Bet == nil {
	// 	record.Bet = map[string]int64{}
	// }

	//out
	out := Out{
		record.HighScore,
		record.Trys,
		rank,
		rankNum,
		record.TeamName,
		record.GameCoinNum,
		record.ChallengeHighScore,
		record.CupType,
		record.MatchReward,
		record.BetReward,
		record.Bet,
		record.BetMoneySum,
	}

	//out
	lwutil.WriteResponse(w, out)
}

func apiPlayBegin(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var in struct {
		EventId int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//get event
	resp, err := ssdb.Do("hget", H_EVENT, in.EventId)
	lwutil.CheckSsdbError(resp, err)
	var event Event
	err = json.Unmarshal([]byte(resp[1]), &event)
	lwutil.CheckError(err, "")
	now := lwutil.GetRedisTimeUnix()

	if now < event.BeginTime || now >= event.EndTime || event.HasResult {
		lwutil.SendError("err_time", "event not running")
	}

	//get event player record
	record := getEventPlayerRecord(ssdb, in.EventId, session.Userid)

	// key := makeEventPlayerRecordSubkey(in.EventId, session.Userid)
	// resp, err = ssdb.Do("hget", H_EVENT_PLAYER_RECORD, key)
	// lwutil.CheckSsdbError(resp, err)
	// record := EventPlayerRecord{}

	// err = json.Unmarshal([]byte(resp[1]), &record)
	// lwutil.CheckError(err, "")

	//
	record.Trys++

	if record.GameCoinNum <= 0 {
		lwutil.SendError("err_game_coin", "")
	}
	record.GameCoinNum--

	//gen secret
	record.Secret = lwutil.GenUUID()
	record.SecretExpire = lwutil.GetRedisTimeUnix() + TRY_EXPIRE_SECONDS

	//update record
	saveEventPlayerRecord(ssdb, in.EventId, session.Userid, record)
	// js, err := json.Marshal(record)
	// resp, err = ssdb.Do("hset", H_EVENT_PLAYER_RECORD, key, js)
	// lwutil.CheckSsdbError(resp, err)

	//out
	lwutil.WriteResponse(w, record)
}

func apiPlayEnd(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var in struct {
		EventId  int64
		Secret   string
		Score    int
		Checksum string
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//checksum
	checksum := fmt.Sprintf("%s+%d9d7a", in.Secret, in.Score+8703)
	hasher := sha1.New()
	hasher.Write([]byte(checksum))
	checksum = hex.EncodeToString(hasher.Sum(nil))
	if in.Checksum != checksum {
		lwutil.SendError("err_checksum", "")
	}

	//check event record
	now := lwutil.GetRedisTimeUnix()
	recordKey := makeEventPlayerRecordSubkey(in.EventId, session.Userid)
	resp, err := ssdb.Do("hget", H_EVENT_PLAYER_RECORD, recordKey)
	lwutil.CheckError(err, "")
	if resp[0] != "ok" {
		lwutil.SendError("err_not_found", "event record not found")
	}

	record := EventPlayerRecord{}
	err = json.Unmarshal([]byte(resp[1]), &record)
	lwutil.CheckError(err, "")
	if record.Secret != in.Secret {
		lwutil.SendError("err_not_match", "Secret not match")
	}
	if now > record.SecretExpire {
		lwutil.SendError("err_expired", "secret expired")
	}

	//clear secret
	record.SecretExpire = 0

	//update score
	scoreUpdate := false
	if record.Trys == 1 || record.HighScore == 0 {
		record.HighScore = in.Score
		record.HighScoreTime = now
		scoreUpdate = true
	} else {
		if in.Score > record.HighScore {
			record.HighScore = in.Score
			scoreUpdate = true
		}
	}

	//save record
	jsRecord, err := json.Marshal(record)
	resp, err = ssdb.Do("hset", H_EVENT_PLAYER_RECORD, recordKey, jsRecord)
	lwutil.CheckSsdbError(resp, err)

	//redis
	rc := redisPool.Get()
	defer rc.Close()

	//event leaderboard
	eventLbLey := makeRedisLeaderboardKey(in.EventId)
	if scoreUpdate {
		_, err = rc.Do("ZADD", eventLbLey, record.HighScore, session.Userid)
		lwutil.CheckError(err, "")
	}

	//get rank
	rc.Send("ZREVRANK", eventLbLey, session.Userid)
	rc.Send("ZCARD", eventLbLey)
	err = rc.Flush()
	lwutil.CheckError(err, "")
	rank, err := redis.Int(rc.Receive())
	lwutil.CheckError(err, "")
	rankNum, err := redis.Int(rc.Receive())
	lwutil.CheckError(err, "")

	//recaculate team score
	if scoreUpdate && rank <= TEAM_SCORE_RANK_MAX {
		recaculateTeamScore(ssdb, rc, in.EventId)
	}

	//out
	out := struct {
		Rank    uint32
		RankNum uint32
	}{
		uint32(rank + 1),
		uint32(rankNum),
	}

	//out
	lwutil.WriteResponse(w, out)
}

func recaculateTeamScore(ssdb *ssdb.Client, rc redis.Conn, eventId int64) map[string]int {
	resp, err := ssdb.Do("hget", H_EVENT, eventId)
	lwutil.CheckSsdbError(resp, err)
	var event Event
	err = json.Unmarshal([]byte(resp[1]), &event)
	lwutil.CheckError(err, "")
	if event.HasResult == false {
		//get ranks from redis
		eventLbLey := makeRedisLeaderboardKey(eventId)
		values, err := redis.Values(rc.Do("ZREVRANGE", eventLbLey, 0, TEAM_SCORE_RANK_MAX-1))
		lwutil.CheckError(err, "")

		num := len(values)
		userIds := make([]int64, 0, TEAM_SCORE_RANK_MAX)
		if num > 0 {
			cmds := make([]interface{}, 0, num+2)
			cmds = append(cmds, "multi_hget")
			cmds = append(cmds, H_EVENT_PLAYER_RECORD)

			for i := 0; i < num; i++ {
				userId, err := redis.Int64(values[i], nil)
				lwutil.CheckError(err, "")
				userIds = append(userIds, userId)
				recordKey := makeEventPlayerRecordSubkey(eventId, userId)
				cmds = append(cmds, recordKey)
			}

			//get event player record
			resp, err = ssdb.Do(cmds...)
			lwutil.CheckSsdbError(resp, err)
			resp = resp[1:]

			if num*2 != len(resp) {
				lwutil.SendError("err_data_missing", "")
			}
			var record EventPlayerRecord
			scoreMap := make(map[string]int)
			for i := range userIds {
				err = json.Unmarshal([]byte(resp[i*2+1]), &record)
				lwutil.CheckError(err, "")
				score := scoreMap[record.TeamName]
				score += 100 - i
				if i == 0 {
					score += 50
				}
				scoreMap[record.TeamName] = score
			}
			// glog.Info(scoreMap)

			js, err := json.Marshal(scoreMap)
			lwutil.CheckError(err, "")

			resp, err := ssdb.Do("hset", H_EVENT_TEAM_SCORE, eventId, js)
			// glog.Info(string(js))
			lwutil.CheckSsdbError(resp, err)

			return scoreMap
		}
	}
	return nil
}

func _redisInt(reply interface{}, err error) (int, error) {
	v, err := redis.Int(reply, err)
	if err == redis.ErrNil {
		return -1, nil
	} else {
		return v, err
	}
}

func apiGetRanks(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var in struct {
		EventId int64
		Offset  int
		Limit   int
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if in.Limit > 30 {
		in.Limit = 30
	}

	//check event id
	resp, err := ssdb.Do("hget", H_EVENT, in.EventId)
	lwutil.CheckSsdbError(resp, err)
	var event Event
	err = json.Unmarshal([]byte(resp[1]), &event)
	lwutil.CheckError(err, "")

	//RankInfo
	type RankInfo struct {
		Rank            int
		UserId          int64
		NickName        string
		TeamName        string
		GravatarKey     string
		CustomAvatarKey string
		Score           int
		Time            int64
		Trys            int
	}

	type Out struct {
		EventId int64
		MyRank  int
		Ranks   []RankInfo
		RankNum int
	}

	//get ranks
	var ranks []RankInfo
	myRank := 0
	rankNum := 0

	if event.HasResult {
		cmds := make([]interface{}, in.Limit+2)
		cmds[0] = "multi_hget"
		cmds[1] = makeHashEventRankKey(event.Id)
		hRankKey := cmds[1]
		for i := 0; i < in.Limit; i++ {
			rank := i + in.Offset + 1
			cmds[i+2] = rank
		}

		resp, err := ssdb.Do(cmds...)
		lwutil.CheckSsdbError(resp, err)
		resp = resp[1:]

		num := len(resp) / 2
		ranks = make([]RankInfo, num)

		for i := 0; i < num; i++ {
			rank, err := strconv.ParseUint(resp[i*2], 10, 32)
			lwutil.CheckError(err, "")
			ranks[i].Rank = int(rank)
			ranks[i].UserId, err = strconv.ParseInt(resp[i*2+1], 10, 64)
			lwutil.CheckError(err, "")
		}

		//my rank
		recordKey := makeEventPlayerRecordSubkey(in.EventId, session.Userid)
		resp, err = ssdb.Do("hget", H_EVENT_PLAYER_RECORD, recordKey)
		lwutil.CheckError(err, "")

		record := EventPlayerRecord{}
		if resp[0] == "ok" {
			err = json.Unmarshal([]byte(resp[1]), &record)
			lwutil.CheckError(err, "")
			myRank = record.FinalRank
		}

		//rankNum
		resp, err = ssdb.Do("hsize", hRankKey)
		lwutil.CheckSsdbError(resp, err)
		rankNum, err = strconv.Atoi(resp[1])
		lwutil.CheckError(err, "")
	} else {
		//redis
		rc := redisPool.Get()
		defer rc.Close()

		eventLbLey := makeRedisLeaderboardKey(in.EventId)

		//get ranks from redis
		values, err := redis.Values(rc.Do("ZREVRANGE", eventLbLey, in.Offset, in.Offset+in.Limit-1))
		lwutil.CheckError(err, "")

		num := len(values)
		if num > 0 {
			ranks = make([]RankInfo, num)

			currRank := in.Offset + 1
			for i := 0; i < num; i++ {
				ranks[i].Rank = currRank
				currRank++
				ranks[i].UserId, err = redisInt64(values[i], nil)
				lwutil.CheckError(err, "")
			}
		}

		//get my rank
		rc.Send("ZREVRANK", eventLbLey, session.Userid)
		rc.Send("ZCARD", eventLbLey)
		err = rc.Flush()
		lwutil.CheckError(err, "")
		myRank, err = redis.Int(rc.Receive())
		if err == nil {
			myRank += 1
		} else {
			myRank = 0
		}
		rankNum, err = redis.Int(rc.Receive())
		if err != nil {
			rankNum = 0
		}
	}

	num := len(ranks)
	if num == 0 {
		out := Out{
			in.EventId,
			myRank,
			[]RankInfo{},
			rankNum,
		}
		lwutil.WriteResponse(w, out)
		return
	}

	//get event player record
	cmds := make([]interface{}, 0, num+2)
	cmds = append(cmds, "multi_hget")
	cmds = append(cmds, H_EVENT_PLAYER_RECORD)
	for _, rank := range ranks {
		recordKey := makeEventPlayerRecordSubkey(in.EventId, rank.UserId)
		cmds = append(cmds, recordKey)
	}
	resp, err = ssdb.Do(cmds...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	if num*2 != len(resp) {
		lwutil.SendError("err_data_missing", "")
	}
	var record EventPlayerRecord
	for i := range ranks {
		err = json.Unmarshal([]byte(resp[i*2+1]), &record)
		lwutil.CheckError(err, "")
		ranks[i].Score = record.HighScore
		ranks[i].NickName = record.PlayerName
		ranks[i].Time = record.HighScoreTime
		ranks[i].Trys = record.Trys
		ranks[i].TeamName = record.TeamName
		ranks[i].GravatarKey = record.GravatarKey
		ranks[i].CustomAvatarKey = record.CustomAvartarKey
	}

	//out
	out := Out{
		in.EventId,
		myRank,
		ranks,
		rankNum,
	}

	lwutil.WriteResponse(w, out)
}

func apiSubmitChallengeScore(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var in struct {
		EventId  int64
		Score    int
		Checksum string
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	playerKey := makePlayerInfoKey(session.Userid)

	//check eventId
	var challengeEventId int64
	err = ssdb.HGet(playerKey, playerChallengeEventId, &challengeEventId)
	lwutil.CheckError(err, "")
	if in.EventId > challengeEventId {
		lwutil.SendError("err_invalid_event", "")
	}

	//checksum
	checksum := fmt.Sprintf("zzzz%d9d7a", in.Score+8703)
	hasher := sha1.New()
	hasher.Write([]byte(checksum))
	checksumHex := hex.EncodeToString(hasher.Sum(nil))

	if in.Checksum != checksumHex {
		lwutil.SendError("err_checksum", "")
	}

	//check event record
	recordKey := makeEventPlayerRecordSubkey(in.EventId, session.Userid)
	resp, err := ssdb.Do("hget", H_EVENT_PLAYER_RECORD, recordKey)
	lwutil.CheckSsdbError(resp, err)

	record := EventPlayerRecord{}
	err = json.Unmarshal([]byte(resp[1]), &record)
	lwutil.CheckError(err, "")

	//update score
	scoreUpdate := false
	reward := 0
	oldScore := record.ChallengeHighScore
	if record.ChallengeHighScore == 0 {
		record.ChallengeHighScore = in.Score
		scoreUpdate = true
	} else {
		if in.Score > record.ChallengeHighScore {
			record.ChallengeHighScore = in.Score
			scoreUpdate = true
		}
	}

	//
	if scoreUpdate {
		if oldScore == 0 {
			oldScore = math.MinInt32
		}

		//get event
		resp, err := ssdb.Do("hget", H_EVENT, in.EventId)
		lwutil.CheckSsdbError(resp, err)

		event := Event{}
		err = json.Unmarshal([]byte(resp[1]), &event)
		lwutil.CheckError(err, "")

		record.CupType = 0
		challengeRewards := event.ChallengeRewards
		if challengeRewards == nil {
			challengeRewards = DEFAULT_CHALLENGE_REWARDS
		}
		for i, sec := range event.ChallengeSecs {
			refScore := -int(sec * 1000)
			if refScore > oldScore && refScore <= in.Score {
				reward += challengeRewards[i]
			}
			if record.CupType == 0 && in.Score >= refScore {
				record.CupType = i + 1
			}
		}
	}

	//save record
	if scoreUpdate {
		jsRecord, err := json.Marshal(record)
		resp, err = ssdb.Do("hset", H_EVENT_PLAYER_RECORD, recordKey, jsRecord)
		lwutil.CheckSsdbError(resp, err)
	}

	//add money
	newMoney := int64(0)
	totalReward := int64(0)
	if reward > 0 {
		resp, err := ssdb.Do("hincr", playerKey, playerMoney, reward)
		lwutil.CheckSsdbError(resp, err)
		newMoney, err = strconv.ParseInt(resp[1], 10, 64)
		lwutil.CheckError(err, "")

		resp, err = ssdb.Do("hincr", playerKey, playerTotalReward, reward)
		lwutil.CheckSsdbError(resp, err)
		totalReward, err = strconv.ParseInt(resp[1], 10, 64)
		lwutil.CheckError(err, "")
	}

	//update ChallangeEventId
	if challengeEventId == in.EventId && record.CupType > 0 {
		challengeEventId++
		resp, err = ssdb.Do("hset", playerKey, playerChallengeEventId, challengeEventId)
		lwutil.CheckSsdbError(resp, err)
	}

	//out
	out := struct {
		Reward           int
		Money            int64
		TotalReward      int64
		CupType          int
		ChallengeEventId int64
	}{
		reward,
		newMoney,
		totalReward,
		record.CupType,
		challengeEventId,
	}

	//out
	lwutil.WriteResponse(w, out)
}

func apiPassMissingChallenge(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var in struct {
		EventId int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	playerKey := makePlayerInfoKey(session.Userid)

	//check eventId
	var challengeEventId int64
	err = ssdb.HGet(playerKey, playerChallengeEventId, &challengeEventId)
	lwutil.CheckError(err, "")
	if in.EventId != challengeEventId {
		lwutil.SendError("err_invalid_event", "")
	}

	//event must not exist
	resp, err := ssdb.Do("hget", H_EVENT, in.EventId)
	lwutil.CheckError(err, "")
	if resp[0] != "not_found" {
		lwutil.SendError("err_event_exist", "")
	}

	//add chanllengeEventId
	resp, err = ssdb.Do("hincr", playerKey, playerChallengeEventId, 1)
	lwutil.CheckSsdbError(resp, err)
	challengeEventId++

	//add money
	addMoney := 100 + rand.Int()%400
	resp, err = ssdb.Do("hincr", playerKey, playerMoney, addMoney)
	lwutil.CheckSsdbError(resp, err)
	money, err := strconv.ParseInt(resp[1], 10, 64)
	lwutil.CheckError(err, "")

	//out
	out := struct {
		AddMoney         int
		Money            int64
		ChallengeEventId int64
	}{
		addMoney,
		money,
		challengeEventId,
	}
	lwutil.WriteResponse(w, out)
}

func apiGetBettingPool(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//in
	var in struct {
		EventId int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//betting pool
	key := makeEventBettingPoolKey(in.EventId)
	bettingPool := map[string]int64{}
	err = ssdb.HGetMapAll(key, bettingPool)
	lwutil.CheckError(err, "")

	//team score
	resp, err := ssdb.Do("hget", H_EVENT_TEAM_SCORE, in.EventId)
	teamScores := map[string]int{}
	if resp[0] == "ok" {
		err = json.Unmarshal([]byte(resp[1]), &teamScores)
		lwutil.CheckError(err, "")
	}

	//out
	out := map[string]interface{}{
		"BettingPool": bettingPool,
		"TeamScores":  teamScores,
	}
	lwutil.WriteResponse(w, out)
}

func apiBet(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//in
	var in struct {
		EventId  int64
		TeamName string
		Money    int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	userId := session.Userid

	//check event
	event := getEvent(ssdb, in.EventId)
	if !isEventRunning(event) {
		lwutil.SendError("err_event_not_running", "")
	}
	now := lwutil.GetRedisTimeUnix()

	if event.BetEndTime == 0 {
		event.BetEndTime = event.EndTime - BET_CLOSE_BEFORE_END_SEC
	}
	if now >= event.BetEndTime {
		lwutil.SendError("err_bet_close", "")
	}

	//check money
	playerInfo, err := getPlayerInfo(ssdb, userId)
	lwutil.CheckError(err, "")
	if in.Money > playerInfo.Money {
		lwutil.SendError("err_money", "")
	}
	money := playerInfo.Money

	//update bet
	record := getEventPlayerRecord(ssdb, in.EventId, userId)
	if record.Bet == nil {
		record.Bet = map[string]int64{}
	}
	record.Bet[in.TeamName] += in.Money
	record.BetMoneySum += in.Money
	saveEventPlayerRecord(ssdb, in.EventId, userId, record)

	//update money
	playerKey := makePlayerInfoKey(userId)
	resp, err := ssdb.Do("hincr", playerKey, playerMoney, -in.Money)
	lwutil.CheckSsdbError(resp, err)
	money -= in.Money

	// add to H_EVENT_TEAM_PLAYER_BET
	key := makeEventTeamPlayerBetKey(in.EventId, playerInfo.TeamName)
	resp, err = ssdb.Do("hincr", key, userId, in.Money)
	lwutil.CheckSsdbError(resp, err)

	//add to betting pool
	bettingPoolKey := makeEventBettingPoolKey(in.EventId)
	resp, err = ssdb.Do("hincr", bettingPoolKey, in.TeamName, in.Money)

	//out
	out := map[string]interface{}{
		"TeamName":    in.TeamName,
		"BetMoney":    record.Bet[in.TeamName],
		"BetMoneySum": record.BetMoneySum,
		"UserMoney":   money,
	}
	lwutil.WriteResponse(w, out)
}

func apiListPlayResult(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//in
	var in struct {
		StartEventId int
		Limit        int
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if in.StartEventId <= 0 {
		in.StartEventId = math.MaxInt32
	}

	if in.Limit < 0 || in.Limit > 20 {
		in.Limit = 50
	}

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//zrscan
	key := fmt.Sprintf("Z_EVENT_PLAYER_RECORD/%d", session.Userid)
	resp, err := ssdb.Do("zrscan", key, in.StartEventId, in.StartEventId, "", in.Limit)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]
	if len(resp) == 0 {
		records := []EventPlayerRecord{}
		lwutil.WriteResponse(w, records)
		return
	}

	//multi_hget
	keyNum := len(resp) / 2
	cmds := make([]interface{}, keyNum+2)
	cmds[0] = "multi_hget"
	cmds[1] = H_EVENT_PLAYER_RECORD
	for i := 0; i < keyNum; i++ {
		eventIdStr := resp[i*2]
		key := fmt.Sprintf("%s/%d", eventIdStr, session.Userid)
		cmds[2+i] = key
	}
	resp, err = ssdb.Do(cmds...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	//out
	resultNum := len(resp) / 2
	records := make([]EventPlayerRecord, resultNum)
	for i := 0; i < resultNum; i++ {
		err = json.Unmarshal([]byte(resp[i*2+1]), &records[i])
		lwutil.CheckError(err, "")
	}

	lwutil.WriteResponse(w, records)
}

func apiCheckNewEvent(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//get newest event id
	//zrscan
	resp, err := ssdb.Do("zrscan", Z_EVENT, "", "", "", 1)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]
	if len(resp) == 0 {
		lwutil.SendError("err_not_found", "")
	}
	eventId, err := strconv.ParseInt(resp[0], 10, 64)
	lwutil.CheckError(err, "")

	//
	playerInfo, err := getPlayerInfo(ssdb, session.Userid)
	lwutil.CheckError(err, "")

	//out
	out := struct {
		EventId     int64
		RewardCache int64
	}{
		eventId,
		playerInfo.RewardCache,
	}
	lwutil.WriteResponse(w, out)
}

func regMatch() {
	http.Handle("/event/new", lwutil.ReqHandler(apiNewEvent))
	http.Handle("/event/del", lwutil.ReqHandler(apiDelEvent))
	http.Handle("/event/mod", lwutil.ReqHandler(apiModEvent))
	http.Handle("/event/list", lwutil.ReqHandler(apiListEvent))
	http.Handle("/event/revList", lwutil.ReqHandler(apiRevListEvent))
	http.Handle("/event/get", lwutil.ReqHandler(apiGetEvent))
	http.Handle("/event/addToBuff", lwutil.ReqHandler(apiAddEventToBuff))
	http.Handle("/event/listBuff", lwutil.ReqHandler(apiListEventBuff))
	http.Handle("/event/delFromBuff", lwutil.ReqHandler(apiDelEventFromBuff))
	http.Handle("/event/getUserPlay", lwutil.ReqHandler(apiGetUserPlay))
	http.Handle("/event/playBegin", lwutil.ReqHandler(apiPlayBegin))
	http.Handle("/event/playEnd", lwutil.ReqHandler(apiPlayEnd))
	http.Handle("/event/getRanks", lwutil.ReqHandler(apiGetRanks))
	http.Handle("/event/submitChallengeScore", lwutil.ReqHandler(apiSubmitChallengeScore))
	http.Handle("/event/passMissingChallenge", lwutil.ReqHandler(apiPassMissingChallenge))
	http.Handle("/event/getBettingPool", lwutil.ReqHandler(apiGetBettingPool))
	http.Handle("/event/bet", lwutil.ReqHandler(apiBet))
	http.Handle("/event/listPlayResult", lwutil.ReqHandler(apiListPlayResult))
	http.Handle("/event/checkNew", lwutil.ReqHandler(apiCheckNewEvent))
}
