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
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"time"
)

const (
	COUPON_SUM_MAX              = 1000
	H_EVENT                     = "H_EVENT"
	Z_EVENT                     = "Z_EVENT"
	H_EVENT_BUFF                = "H_EVENT_BUFF"
	Z_EVENT_BUFF                = "Z_EVENT_BUFF"
	H_EVENT_PLAYER_RECORD       = "H_EVENT_PLAYER_RECORD" //subkey:<eventId>/<userId>
	Z_EVENT_PLAYER_RECORD       = "Z_EVENT_PLAYER_RECORD" //key:Z_EVENT_PLAYER_RECORD/<(int64)userId> subkey:(int)eventId score:(int)eventId
	RDS_Z_EVENT_LEADERBOARD_PRE = "RDS_Z_EVENT_LEADERBOARD_PRE"
	H_EVENT_RANK                = "H_EVENT_RANK" //subkey:(uint32)rank value:(uint64)userId
	EVENT_SERIAL                = "EVENT_SERIAL"
	BUFF_EVENT_SERIAL           = "BUFF_EVENT_SERIAL"
	TRY_COST_MONEY              = 100
	TRY_EXPIRE_SECONDS          = 600
	TEAM_CHAMPIONSHIP_ROUND_NUM = 6
	H_EVENT_TEAM_SCORE          = "H_EVENT_TEAM_SCORE"   //subkey:eventId value:map[(string)teamName](int)score
	H_EVENT_BETTING_POOL        = "H_EVENT_BETTING_POOL" //key:H_EVENT_BETTING_POOL/eventId subkey:(string)teamName value:(int64)money
	INIT_GAME_COIN_NUM          = 3
	BET_CLOSE_BEFORE_END_SEC    = 60 * 60
	// BET_CLOSE_BEFORE_END_SEC = 0
	H_EVENT_TEAM_PLAYER_BET = "H_EVENT_TEAM_PLAYER_BET" //key:H_EVENT_TEAM_PLAYER_BET/eventId/teamName subKey:playerId value:betMoney
	TIME_FORMAT             = "2006-01-02T15:04"
	TEAM_SCORE_RANK_MAX     = 100
	K_EVENT_PUBLISH         = "K_EVENT_PUBLISH"
)

var (
	_eventPublishInfoes []EventPublishInfo

	TEAM_NAMES              = []string{"安徽", "澳门", "北京", "重庆", "福建", "甘肃", "广东", "广西", "贵州", "海南", "河北", "黑龙江", "河南", "湖北", "湖南", "江苏", "江西", "吉林", "辽宁", "内蒙古", "宁夏", "青海", "陕西", "山东", "上海", "山西", "四川", "台湾", "天津", "香港", "新疆", "西藏", "云南", "浙江"}
	EVENT_INIT_BETTING_POOL = map[string]interface{}{}
	INIT_BET_MONEY          = int64(10000)
)

type Event struct {
	Id              int64
	PackId          int64
	PackTitle       string
	Thumb           string
	BeginTime       int64
	EndTime         int64
	BeginTimeString string
	EndTimeString   string
	BetEndTime      int64
	HasResult       bool
	SliderNum       int
	ChallengeSecs   []int
}

type BuffEvent struct {
	Id            int64
	PackId        int64
	PackTitle     string
	SliderNum     int
	ChallengeSecs []int
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

// func checkCouponSetting(event *Event) bool {
// 	couponSetting := event.CouponSetting
// 	if len(couponSetting) == 0 {
// 		return true
// 	}

// 	if couponSetting[0][0] != 1 {
// 		glog.Error("couponSetting[0][0] != 1")
// 		return false
// 	}

// 	rank := 0
// 	couponNum := int(math.MaxInt32)
// 	couponSum := 0
// 	for i := 0; i < len(couponSetting); i++ {
// 		if couponSetting[i][0] <= rank {
// 			glog.Error("couponSetting'rank must be asc")
// 			return false
// 		}
// 		if couponSetting[i][1] >= couponNum {
// 			glog.Error("couponSetting's couponNum must be desc")
// 			return false
// 		}
// 		rank = couponSetting[i][0]
// 		couponNum = couponSetting[i][1]
// 		couponSum += couponNum
// 	}

// 	if couponSetting[len(couponSetting)-1][1] != 0 {
// 		glog.Error("last couponNum must be 0")
// 		return false
// 	}

// 	if couponSum > COUPON_SUM_MAX {
// 		glog.Errorf("couponSum > COUPON_SUM_MAX, couponSum=%d, COUPON_SUM_MAX=%d", couponSum, COUPON_SUM_MAX)
// 		return false
// 	}

// 	event.CouponSum = couponSum

// 	return true
// }

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

func initEvent() {
	//ssdb
	ssdbc, err := ssdbPool.Get()
	checkError(err)
	defer ssdbc.Close()

	//
	for _, teamName := range TEAM_NAMES {
		EVENT_INIT_BETTING_POOL[teamName] = INIT_BET_MONEY
	}

	//cron
	_cron.AddFunc("0 * * * * *", eventPublish)

	//eventPublishInfoes
	resp, err := ssdbc.Do("get", K_EVENT_PUBLISH)
	if err != nil || len(resp) <= 1 {
		_eventPublishInfoes = _conf.EventPublishInfoes
	} else {
		err = json.Unmarshal([]byte(resp[1]), &_eventPublishInfoes)
		checkError(err)
	}
}

func eventPublish() {
	eventPublishTask()
	pickSidePublishTask()
}

func eventPublishTask() {
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
	for _, pubInfo := range _eventPublishInfoes {
		if pubInfo.PublishTime[0] == now.Hour() && pubInfo.PublishTime[1] == now.Minute() {
			//pop from Z_EVENT_BUFF and push to Z_EVENT
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

				var event Event
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

				//init betting pool
				key := makeEventBettingPoolKey(event.Id)
				err = ssdbc.HSetMap(key, EVENT_INIT_BETTING_POOL)
				lwutil.CheckError(err, "")

				//get pack
				pack, err := getPack(ssdbc, event.PackId)
				lwutil.CheckError(err, fmt.Sprintf("packId:%d", event.PackId))

				event.Thumb = pack.Thumb
				event.PackTitle = pack.Title

				//save event
				bts, err := json.Marshal(event)
				checkError(err)
				resp, err = ssdbc.Do("hset", H_EVENT, event.Id, bts)
				checkSsdbError(resp, err)

				//push to Z_EVENT
				resp, err = ssdbc.Do("zset", Z_EVENT, event.Id, event.Id)
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

				//del front event
				resp, err = ssdbc.Do("zdel", Z_EVENT_BUFF, buffEventId)
				checkSsdbError(resp, err)
				resp, err = ssdbc.Do("hdel", H_EVENT_BUFF, buffEventId)
				checkSsdbError(resp, err)

				//
				glog.Infof("Add event and challenge ok:id=%d", event.Id)
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

func initPlayerRecord(record *EventPlayerRecord, ssdbc *ssdb.Client, eventId int64, userId int64) {
	playerInfo, err := getPlayerInfo(ssdbc, userId)
	lwutil.CheckError(err, "")

	record.EventId = eventId
	record.Trys = 0
	record.PlayerName = playerInfo.NickName
	record.TeamName = ""
	record.GravatarKey = playerInfo.GravatarKey
	record.CustomAvartarKey = playerInfo.CustomAvatarKey
	record.GameCoinNum = INIT_GAME_COIN_NUM

	//PackThumbKey
	event := getEvent(ssdbc, eventId)
	pack, err := getPack(ssdbc, event.PackId)
	lwutil.CheckError(err, "")
	record.PackThumbKey = pack.Thumb
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
		initPlayerRecord(&record, ssdb, eventId, userId)
		return &record
	}
}

func saveEventPlayerRecord(ssdb *ssdb.Client, eventId int64, userId int64, record *EventPlayerRecord) {
	key := makeEventPlayerRecordSubkey(eventId, userId)
	js, err := json.Marshal(record)
	lwutil.CheckError(err, "")
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

	//check
	if event.BeginTime >= event.EndTime {
		lwutil.SendError("err_time", "event.BeginTime >= event.EndTime")
	}

	//BetEndTime
	if event.BetEndTime == 0 {
		event.BetEndTime = event.EndTime - BET_CLOSE_BEFORE_END_SEC
	}
}

func apiEventNew(w http.ResponseWriter, r *http.Request) {
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
	event.HasResult = false

	//
	calcEventTimes(&event.Event)

	//sliderNum
	if event.SliderNum == 0 {
		event.SliderNum = 6
	} else if event.SliderNum > 10 {
		event.SliderNum = 10
	}

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
	event.PackTitle = pack.Title

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

func apiEventMod(w http.ResponseWriter, r *http.Request) {
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

	//get pack
	resp, err := ssdb.Do("hget", H_PACK, event.PackId)
	lwutil.CheckSsdbError(resp, err)
	var pack Pack
	err = json.Unmarshal([]byte(resp[1]), &pack)
	lwutil.CheckError(err, "")
	event.Thumb = pack.Thumb
	event.PackTitle = pack.Title

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

func apiEventDel(w http.ResponseWriter, r *http.Request) {
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

func apiEventList(w http.ResponseWriter, r *http.Request) {
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
	resp, err := ssdb.Do("zrscan", Z_EVENT, startId, startId, "", in.Limit)
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
	cmds[1] = H_EVENT
	for i := 0; i < keyNum; i++ {
		cmds[2+i] = resp[i*2]
	}
	resp, err = ssdb.Do(cmds...)
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

func apiEventGet(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

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
	out := struct {
		Event
		Pack *Pack
	}{}
	err = json.Unmarshal([]byte(resp[1]), &out.Event)
	lwutil.CheckError(err, "")

	//pack
	pack, err := getPack(ssdb, out.Event.Id)
	lwutil.CheckError(err, "")

	out.Pack = pack

	lwutil.WriteResponse(w, out)
}

func apiEventBuffAdd(w http.ResponseWriter, r *http.Request) {
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
	var buffEvent BuffEvent
	err = lwutil.DecodeRequestBody(r, &buffEvent)
	lwutil.CheckError(err, "err_decode_body")

	//sliderNum
	if buffEvent.SliderNum <= 0 {
		buffEvent.SliderNum = 5
	} else if buffEvent.SliderNum > 10 {
		buffEvent.SliderNum = 10
	}

	//gen serial
	buffEvent.Id = GenSerial(ssdb, BUFF_EVENT_SERIAL)

	//get pack
	pack, err := getPack(ssdb, buffEvent.PackId)
	lwutil.CheckError(err, "")

	//
	buffEvent.PackTitle = pack.Title

	//save to ssdb
	js, err := json.Marshal(buffEvent)
	lwutil.CheckError(err, "")
	resp, err := ssdb.Do("hset", H_EVENT_BUFF, buffEvent.Id, js)
	lwutil.CheckSsdbError(resp, err)

	resp, err = ssdb.Do("zset", Z_EVENT_BUFF, buffEvent.Id, buffEvent.Id)
	lwutil.CheckSsdbError(resp, err)

	//out
	lwutil.WriteResponse(w, buffEvent)
}

func apiEventBuffList(w http.ResponseWriter, r *http.Request) {
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
	out := make([]BuffEvent, eventNum)
	for i := 0; i < eventNum; i++ {
		err = json.Unmarshal([]byte(resp[i*2+1]), &out[i])
		lwutil.CheckError(err, "")
	}

	lwutil.WriteResponse(w, out)
}

func apiEventBuffDel(w http.ResponseWriter, r *http.Request) {
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

	//check exist
	resp, err := ssdb.Do("hexists", H_EVENT_BUFF, in.EventId)
	lwutil.CheckError(err, "")
	if resp[1] != "1" {
		lwutil.SendError("err_exist", fmt.Sprintf("buffEvent not exist:id=", in.EventId))
	}

	//del
	resp, err = ssdb.Do("zdel", Z_EVENT_BUFF, in.EventId)
	lwutil.CheckSsdbError(resp, err)
	resp, err = ssdb.Do("hdel", H_EVENT_BUFF, in.EventId)
	lwutil.CheckSsdbError(resp, err)

	lwutil.WriteResponse(w, in)
}

func apiEventBuffMod(w http.ResponseWriter, r *http.Request) {
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
	var event BuffEvent
	err = lwutil.DecodeRequestBody(r, &event)
	lwutil.CheckError(err, "err_decode_body")

	//check exist
	resp, err := ssdb.Do("hget", H_EVENT_BUFF, event.Id)
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
	record.Trys++

	if record.TeamName == "" {
		playerInfo, err := getPlayerInfo(ssdb, session.Userid)
		lwutil.CheckError(err, "")
		record.TeamName = playerInfo.TeamName
	}

	if record.GameCoinNum <= 0 {
		lwutil.SendError("err_game_coin", "")
	}
	record.GameCoinNum--

	//gen secret
	record.Secret = lwutil.GenUUID()
	record.SecretExpire = lwutil.GetRedisTimeUnix() + TRY_EXPIRE_SECONDS

	//update record
	saveEventPlayerRecord(ssdb, in.EventId, session.Userid, record)

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
	if event.HasResult {
		lwutil.SendError("err_event_has_result", "")
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
	resp, err := ssdb.Do("hincr", playerKey, PLAYER_MONEY, -in.Money)
	lwutil.CheckSsdbError(resp, err)
	money -= in.Money

	// add to H_EVENT_TEAM_PLAYER_BET
	key := makeEventTeamPlayerBetKey(in.EventId, in.TeamName)
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

func apiGetEventPublish(w http.ResponseWriter, r *http.Request) {
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
	lwutil.WriteResponse(w, _eventPublishInfoes)
}

func apiSetEventPublish(w http.ResponseWriter, r *http.Request) {
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

	_eventPublishInfoes = in

	//save
	js, err := json.Marshal(_eventPublishInfoes)
	lwutil.CheckError(err, "")
	resp, err := ssdb.Do("set", K_EVENT_PUBLISH, js)
	lwutil.CheckSsdbError(resp, err)

	//out
	lwutil.WriteResponse(w, in)
}

func regMatch() {
	http.Handle("/event/new", lwutil.ReqHandler(apiEventNew))
	http.Handle("/event/del", lwutil.ReqHandler(apiEventDel))
	http.Handle("/event/mod", lwutil.ReqHandler(apiEventMod))
	http.Handle("/event/list", lwutil.ReqHandler(apiEventList))
	http.Handle("/event/get", lwutil.ReqHandler(apiEventGet))
	http.Handle("/event/buffAdd", lwutil.ReqHandler(apiEventBuffAdd))
	http.Handle("/event/buffList", lwutil.ReqHandler(apiEventBuffList))
	http.Handle("/event/buffDel", lwutil.ReqHandler(apiEventBuffDel))
	http.Handle("/event/buffMod", lwutil.ReqHandler(apiEventBuffMod))
	http.Handle("/event/getUserPlay", lwutil.ReqHandler(apiGetUserPlay))
	http.Handle("/event/playBegin", lwutil.ReqHandler(apiPlayBegin))
	http.Handle("/event/playEnd", lwutil.ReqHandler(apiPlayEnd))
	http.Handle("/event/getRanks", lwutil.ReqHandler(apiGetRanks))
	http.Handle("/event/getBettingPool", lwutil.ReqHandler(apiGetBettingPool))
	http.Handle("/event/bet", lwutil.ReqHandler(apiBet))
	http.Handle("/event/listPlayResult", lwutil.ReqHandler(apiListPlayResult))
	http.Handle("/event/checkNew", lwutil.ReqHandler(apiCheckNewEvent))
	http.Handle("/event/getPublish", lwutil.ReqHandler(apiGetEventPublish))
	http.Handle("/event/setPublish", lwutil.ReqHandler(apiSetEventPublish))
}
