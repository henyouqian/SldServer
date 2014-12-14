package main

import (
	"./ssdb"
	"encoding/json"
	"fmt"
	"github.com/golang/glog"
	"github.com/henyouqian/lwutil"
	"github.com/qiniu/api/rs"
	"math"
	"net/http"
	"strconv"
)

const (
	H_PLAYER_INFO              = "H_PLAYER_INFO"     //key:H_PLAYER_INFO/<userId> subkey:property
	H_APP_PLAYER_RATE          = "H_APP_PLAYER_RATE" //subkey:appName/userId value:1
	USER_UPLOAD_BUCKET         = "pintuuserupload"
	ADS_PERCENT_DEFAUT         = 0.5
	H_PLAYER_PRIZE_RECORD      = "H_PLAYER_PRIZE_RECORD" //key:H_PLAYER_PRIZE_RECORD subkey:prizeRecordId value prizeRecordJson
	Z_PLAYER_PRIZE_RECORD      = "Z_PLAYER_PRIZE_RECORD" //key:Z_PLAYER_PRIZE_RECORD/userId subkey:prizeRecordId score:timeUnix
	SEREAL_PLAYER_PRIZE_RECORD = "SEREAL_PLAYER_PRIZE_RECORD"
	BATTLE_HEART_TOTAL         = 10
	BATTLE_HEART_ADD_SEC       = 60 * 5
)

type PlayerInfo struct {
	UserId              int64
	NickName            string
	TeamName            string
	Gender              int
	CustomAvatarKey     string
	GravatarKey         string
	Email               string
	GoldCoin            int
	Prize               int
	PrizeCache          int
	TotalPrize          int
	Secret              string
	BattlePoint         int
	BattleWinStreak     int
	BattleWinStreakMax  int
	BattleHeartZeroTime int64
}

//player property
const (
	PLAYER_GOLD_COIN              = "GoldCoin"
	PLAYER_PRIZE                  = "Prize"
	PLAYER_PRIZE_CACHE            = "PrizeCache"
	PLAYER_TOTAL_PRIZE            = "TotalPrize"
	PLAYER_IAP_SECRET             = "IapSecret"
	PLAYER_BATTLE_POINT           = "BattlePoint"
	PLAYER_BATTLE_WIN_STREAK      = "BattleWinStreak"
	PLAYER_BATTLE_WIN_STREAK_MAX  = "BattleWinStreakMax"
	PLAYER_BATTLE_HEART_ZERO_TIME = "BattleHeartZeroTime"
)

type PlayerBattleLevel struct {
	Level      int
	Title      string
	StartPoint int
}

const (
	BATTLE_HELP_TEXT = `✦ 赢了加分加金币，输了不扣分扣金币。

✦ 赢了的话，连胜几场就加几分。

✦ 在使用金币的场次下（除了第一个免费场外），终结了对手连胜纪录的话，额外获得等于对手连胜场次的积分。

✦ 所以想要得高分就尽可能保持连胜，或者凭运气碰到高连胜对手并取得胜利（金币场次）。

✦ 第一个免费场不会赢也不会输掉金币，但每玩一次会消耗一颗心。

✦ 积分与等级的对应关系如下：
`
)

var (
	PLAYER_BATTLE_LEVELS = []PlayerBattleLevel{
		// {1, "🚶", 0},
		// {2, "🚣", 10},
		// {3, "🚲", 30},
		// {4, "🚜", 60},
		// {5, "🚛", 100},
		// {6, "🚚", 150},
		// {7, "🚗", 200},
		// {8, "🚙", 250},
		// {9, "🚌", 300},
		// {10, "🚃", 400},
		// {11, "🚤", 500},
		// {12, "🚈", 600},
		// {13, "🚄", 700},
		// {14, "🚁", 800},
		// {15, "✈️", 900},
		// {16, "🚀", 1000},
		// {17, "🐭", 1100},
		// {18, "🐮", 1200},
		// {19, "🐯", 1300},
		// {20, "🐰", 1400},
		// {21, "🐲", 1500},
		// {22, "🐍", 1600},
		// {23, "🐴", 1700},
		// {24, "🐑", 1800},
		// {25, "🐵", 1900},
		// {26, "🐔", 2000},
		// {27, "🐶", 2100},
		// {28, "🐷", 2200},
		{1, "🐭", 0},
		{2, "🐮", 10},
		{3, "🐯", 30},
		{4, "🐰", 60},
		{5, "🐲", 100},
		{6, "🐍", 150},
		{7, "🐴", 200},
		{8, "🐑", 250},
		{9, "🐵", 300},
		{10, "🐔", 350},
		{11, "🐶", 400},
		{12, "🐷", 450},
		{13, "🚶", 500},
		{14, "🚣", 600},
		{15, "🚲", 700},
		{16, "🚜", 800},
		{17, "🚛", 900},
		{18, "🚚", 1000},
		{19, "🚗", 1100},
		{20, "🚙", 1200},
		{21, "🚌", 1300},
		{22, "🚃", 1400},
		{23, "🚤", 1500},
		{24, "🚈", 1600},
		{25, "🚄", 1700},
		{26, "🚁", 1800},
		{27, "✈️", 1900},
		{28, "🚀", 2000},
	}
)

const (
	PRIZE_REASON_RANK  = "排名奖励"
	PRIZE_REASON_LUCK  = "幸运大奖"
	PRIZE_REASON_OWNER = "发布分成"
)

type PrizeRecord struct {
	Id      int64
	MatchId int64
	Thumb   string
	Reason  string
	Prize   int
	Rank    int
}

func makeZPlayerPrizeRecordKey(userId int64) string {
	return fmt.Sprintf("%s, %d", Z_PLAYER_PRIZE_RECORD, userId)
}

func addPrizeToCache(ssdbc *ssdb.Client, userId int64, matchId int64, matchThumb string, prize int, reason string, rank int) {
	if prize <= 0 {
		return
	}
	playerKey := makePlayerInfoKey(userId)
	addPrizeCache(ssdbc, playerKey, prize)

	var record PrizeRecord
	record.Id = GenSerial(ssdbc, SEREAL_PLAYER_PRIZE_RECORD)
	record.Thumb = matchThumb
	record.Reason = reason
	record.MatchId = matchId
	record.Prize = prize
	record.Rank = rank
	js, err := json.Marshal(record)
	lwutil.CheckError(err, "")

	resp, err := ssdbc.Do("hset", H_PLAYER_PRIZE_RECORD, record.Id, js)
	lwutil.CheckSsdbError(resp, err)

	zkey := makeZPlayerPrizeRecordKey(userId)
	resp, err = ssdbc.Do("zset", zkey, record.Id, record.Id)
	lwutil.CheckSsdbError(resp, err)
}

func init() {
	glog.Info("")

	//check player battle levels
	currLv := 1
	currPt := -1
	for _, v := range PLAYER_BATTLE_LEVELS {
		if v.Level != currLv {
			panic("v.Level != currLv")
		}
		if v.StartPoint <= currPt {
			glog.Error(v.StartPoint, currPt)
			panic("v.StartPoint <= currPt")
		}
		currLv++
		currPt = v.StartPoint
	}
}

func makePlayerInfoKey(userId int64) string {
	return fmt.Sprintf("%s/%d", H_PLAYER_INFO, userId)
}

func makeAppPlayerRateSubkey(appName string, userId int64) string {
	return fmt.Sprintf("%s/%d", appName, userId)
}

func getPlayerInfo(ssdb *ssdb.Client, userId int64) (*PlayerInfo, error) {
	key := makePlayerInfoKey(userId)

	var playerInfo PlayerInfo
	err := ssdb.HGetStruct(key, &playerInfo)
	if err != nil {
		return nil, err
	}

	playerInfo.UserId = userId
	return &playerInfo, err
}

func addPlayerGoldCoin(ssc *ssdb.Client, playerKey string, addNum int) (rNum int) {
	resp, err := ssc.Do("hincr", playerKey, PLAYER_GOLD_COIN, addNum)
	lwutil.CheckSsdbError(resp, err)
	num, err := strconv.Atoi(resp[1])
	lwutil.CheckError(err, "")
	return num
}
func getPlayerGoldCoin(ssc *ssdb.Client, playerKey string) (rNum int) {
	resp, err := ssc.Do("hget", playerKey, PLAYER_GOLD_COIN)
	lwutil.CheckError(err, "")
	if resp[0] == ssdb.NOT_FOUND {
		return 0
	}

	num, err := strconv.Atoi(resp[1])
	lwutil.CheckError(err, "")
	return num
}

func getPrize(ssc *ssdb.Client, playerKey string) int {
	var prize int
	err := ssc.HGet(playerKey, PLAYER_PRIZE, &prize)
	lwutil.CheckError(err, "")
	return prize
}

func addPrize(ssc *ssdb.Client, playerKey string, prize int) (rPrize int) {
	resp, err := ssc.Do("hincr", playerKey, PLAYER_PRIZE, prize)
	lwutil.CheckSsdbError(resp, err)
	num, err := strconv.Atoi(resp[1])
	lwutil.CheckError(err, "")
	return num
}

func getPrizeCache(ssc *ssdb.Client, playerKey string) int {
	var prizeCache int
	err := ssc.HGet(playerKey, PLAYER_PRIZE_CACHE, &prizeCache)
	if err != nil {
		return 0
	}
	return prizeCache
}

func addPrizeCache(ssc *ssdb.Client, playerKey string, prize int) (rPrize int) {
	resp, err := ssc.Do("hincr", playerKey, PLAYER_PRIZE_CACHE, prize)
	lwutil.CheckSsdbError(resp, err)
	num, err := strconv.Atoi(resp[1])
	lwutil.CheckError(err, "")
	return num
}

func getPrizeTotal(ssc *ssdb.Client, playerKey string) int {
	var prize int
	err := ssc.HGet(playerKey, PLAYER_TOTAL_PRIZE, &prize)
	lwutil.CheckError(err, "")
	return prize
}

func addPrizeTotal(ssc *ssdb.Client, playerKey string, prize int) (rPrize int) {
	resp, err := ssc.Do("hincr", playerKey, PLAYER_TOTAL_PRIZE, prize)
	lwutil.CheckSsdbError(resp, err)
	num, err := strconv.Atoi(resp[1])
	lwutil.CheckError(err, "")
	return num
}

func apiGetPlayerInfo(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//get info
	playerInfo, err := getPlayerInfo(ssdb, session.Userid)
	if err != nil {
		w.WriteHeader(500)
		return
	}

	//out
	out := struct {
		*PlayerInfo
		AdsConf              AdsConf
		ClientConf           map[string]string
		OwnerPrizeProportion float32
		BattleLevels         []PlayerBattleLevel
		BattleHelpText       string
		BattleHeartAddSec    int
	}{
		playerInfo,
		_adsConf,
		_clientConf,
		MATCH_OWNER_PRIZE_PROPORTION,
		PLAYER_BATTLE_LEVELS,
		BATTLE_HELP_TEXT,
		BATTLE_HEART_ADD_SEC,
	}
	lwutil.WriteResponse(w, out)
}

func getBattleHeartNum(playerInfo *PlayerInfo) int {
	dt := lwutil.GetRedisTimeUnix() - playerInfo.BattleHeartZeroTime
	heartNum := int(dt) / BATTLE_HEART_ADD_SEC
	if heartNum > BATTLE_HEART_TOTAL {
		heartNum = BATTLE_HEART_TOTAL
	}
	return heartNum
}

func apiSetPlayerInfo(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var in struct {
		NickName        string
		GravatarKey     string
		CustomAvatarKey string
		TeamName        string
		Email           string
		Gender          int
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")
	if in.Gender > 2 {
		lwutil.SendError("err_gender", "")
	}

	stringLimit(&in.NickName, 40)
	stringLimit(&in.GravatarKey, 20)
	stringLimit(&in.CustomAvatarKey, 40)
	stringLimit(&in.TeamName, 40)
	stringLimit(&in.Email, 40)

	//check playerInfo
	if in.NickName == "" || in.TeamName == "" || (in.GravatarKey == "" && in.CustomAvatarKey == "") {
		lwutil.SendError("err_info_incomplete", "")
	}

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//
	playerKey := makePlayerInfoKey(session.Userid)

	//check playerInfo exist
	resp, err := ssdb.Do("hexists", playerKey, PLAYER_GOLD_COIN)
	lwutil.CheckError(err, "")
	if resp[1] != "1" {
		var player PlayerInfo
		err = ssdb.HSetStruct(playerKey, player)
		lwutil.CheckError(err, "")
	}

	//save
	err = ssdb.HSetStruct(playerKey, in)
	lwutil.CheckError(err, "")

	//get player info
	playerInfo, err := getPlayerInfo(ssdb, session.Userid)
	lwutil.CheckError(err, "")

	//out
	out := struct {
		PlayerInfo
	}{
		*playerInfo,
	}
	lwutil.WriteResponse(w, out)
}

func apiAddPrizeFromCache(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//
	key := makePlayerInfoKey(session.Userid)
	prizeCache := getPrizeCache(ssdb, key)
	lwutil.CheckError(err, "")
	if prizeCache > 0 {
		addPrize(ssdb, key, prizeCache)
		addPrizeTotal(ssdb, key, prizeCache)
		addPrizeCache(ssdb, key, -prizeCache)
	}

	playerInfo, err := getPlayerInfo(ssdb, session.Userid)
	lwutil.CheckError(err, "")

	//out
	out := map[string]interface{}{
		"Prize":      playerInfo.Prize,
		"TotalPrize": playerInfo.TotalPrize,
	}
	lwutil.WriteResponse(w, out)
}

func apiGetUptokens(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")

	//in
	var in []string
	err := lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	inLen := len(in)
	type outElem struct {
		Key   string
		Token string
	}
	out := make([]outElem, inLen, inLen)
	for i, v := range in {
		scope := fmt.Sprintf("%s:%s", USER_UPLOAD_BUCKET, v)
		putPolicy := rs.PutPolicy{
			Scope: scope,
		}
		out[i] = outElem{
			in[i],
			putPolicy.Token(nil),
		}
	}

	//out
	lwutil.WriteResponse(w, &out)
}

func apiGetUptoken(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")

	//session
	_, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//
	scope := fmt.Sprintf("%s", USER_UPLOAD_BUCKET)
	putPolicy := rs.PutPolicy{
		Scope: scope,
	}

	//out
	out := struct {
		Token string
	}{
		putPolicy.Token(nil),
	}

	lwutil.WriteResponse(w, &out)
}

func apiListMyEcard(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//in
	var in struct {
		StartId   int64
		LastScore int64
		Limit     int
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if in.StartId <= 0 {
		in.StartId = math.MaxInt64
	}
	if in.LastScore <= 0 {
		in.LastScore = math.MaxInt64
	}

	if in.Limit > 50 {
		in.Limit = 50
	}

	//
	out := struct {
		Ecards    []OutEcard
		LastScore int64
	}{}
	out.Ecards = make([]OutEcard, 0)

	//
	playerEcardZKey := makePlayerEcardZsetKey(session.Userid)
	resp, err := ssdbc.Do("zrscan", playerEcardZKey, in.StartId, in.LastScore, "", in.Limit)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	itemNum := len(resp) / 2
	if itemNum == 0 {
		lwutil.WriteResponse(w, out)
		return
	}

	cmds := make([]interface{}, itemNum+2)
	cmds[0] = "multi_hget"
	cmds[1] = H_ECARD
	lastScore := int64(0)
	for i := 0; i < itemNum; i++ {
		cmds[i+2] = resp[i*2]
		lastScore, err = strconv.ParseInt(resp[i*2+1], 10, 64)
		lwutil.CheckError(err, "")
	}

	//
	resp, err = ssdbc.Do(cmds...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]
	num := len(resp) / 2

	//
	out.LastScore = lastScore
	out.Ecards = make([]OutEcard, num)
	for i := 0; i < num; i++ {
		err = json.Unmarshal([]byte(resp[i*2+1]), &(out.Ecards[i].ECard))
		out.Ecards[i].Provider = ECARD_PROVIDERS[out.Ecards[i].ECard.Provider]
		lwutil.CheckError(err, "")
	}

	lwutil.WriteResponse(w, out)
}

func apiListMyPrize(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//in
	var in struct {
		StartId int64
		Limit   int
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if in.StartId <= 0 {
		in.StartId = math.MaxInt32
	}

	if in.Limit > 50 {
		in.Limit = 50
	}

	//out
	out := []PrizeRecord{}

	//
	zkey := makeZPlayerPrizeRecordKey(session.Userid)
	resp, err := ssdbc.Do("zrscan", zkey, in.StartId, in.StartId, "", in.Limit)
	lwutil.CheckError(err, "")
	resp = resp[1:]
	if len(resp) == 0 {
		lwutil.WriteResponse(w, out)
		return
	}

	num := len(resp) / 2
	cmds := make([]interface{}, num+2)
	cmds[0] = "multi_hget"
	cmds[1] = H_PLAYER_PRIZE_RECORD
	for i := 0; i < num; i++ {
		cmds[i+2] = resp[i*2]
	}

	//
	resp, err = ssdbc.Do(cmds...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]
	num = len(resp) / 2
	for i := 0; i < num; i++ {
		js := resp[i*2+1]
		record := PrizeRecord{}
		err = json.Unmarshal([]byte(js), &record)
		lwutil.CheckError(err, "")
		out = append(out, record)
	}

	lwutil.WriteResponse(w, out)
}

func apiGetPrizeCache(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//
	key := makePlayerInfoKey(session.Userid)
	prizeCache := getPrizeCache(ssdbc, key)

	//out
	out := struct {
		PrizeCache int
	}{
		prizeCache,
	}
	lwutil.WriteResponse(w, out)
}

func regPlayer() {
	http.Handle("/player/getInfo", lwutil.ReqHandler(apiGetPlayerInfo))
	http.Handle("/player/setInfo", lwutil.ReqHandler(apiSetPlayerInfo))
	http.Handle("/player/addPrizeFromCache", lwutil.ReqHandler(apiAddPrizeFromCache))
	http.Handle("/player/getUptokens", lwutil.ReqHandler(apiGetUptokens))
	http.Handle("/player/getUptoken", lwutil.ReqHandler(apiGetUptoken))
	http.Handle("/player/listMyEcard", lwutil.ReqHandler(apiListMyEcard))
	http.Handle("/player/listMyPrize", lwutil.ReqHandler(apiListMyPrize))
	http.Handle("/player/getPrizeCache", lwutil.ReqHandler(apiGetPrizeCache))
}
