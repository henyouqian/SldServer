package main

import (
	"./ssdb"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	// "github.com/garyburd/redigo/redis"
	"github.com/golang/glog"
	"github.com/henyouqian/lwutil"
	"math"
	"math/rand"
	"net/http"
	"strconv"
)

const (
	CHALLENGE_SERIAL = "CHALLENGE_SERIAL"
	H_CHALLENGE      = "H_CHALLENGE"
	H_CHALLENGE_PLAY = "H_CHALLENGE_PLAY" //subkey:<challengeId>/<userId>
	Z_CHALLENGE      = "Z_CHALLENGE"
)

type Challenge struct {
	Id               int64
	PackId           int64
	PackTitle        string
	Thumb            string
	SliderNum        int
	ChallengeSecs    []int
	ChallengeRewards []int
}

type ChallengePlay struct {
	ChallengeId int64
	UserId      int64
	HighScore   int
	CupType     int
}

func _challenge_glog() {
	glog.Info("")
}

func makeChallengePlaySubkey(challengeId int64, userId int64) string {
	key := fmt.Sprintf("%d/%d", challengeId, userId)
	return key
}

func getChallengePlay(ssdbc *ssdb.Client, challengeId int64, userId int64) *ChallengePlay {
	playKey := makeChallengePlaySubkey(challengeId, userId)
	resp, err := ssdbc.Do("hget", H_CHALLENGE_PLAY, playKey)
	lwutil.CheckError(err, "")

	play := ChallengePlay{}
	if len(resp) < 2 {
		play.ChallengeId = challengeId
		play.UserId = userId
	} else {
		err = json.Unmarshal([]byte(resp[1]), &play)
		lwutil.CheckError(err, "")
	}
	return &play
}

func saveChallengePlay(ssdbc *ssdb.Client, play *ChallengePlay) {
	key := makeChallengePlaySubkey(play.ChallengeId, play.UserId)
	jsRecord, err := json.Marshal(play)
	resp, err := ssdbc.Do("hset", H_CHALLENGE_PLAY, key, jsRecord)
	lwutil.CheckSsdbError(resp, err)
}

func apiChallengeCount(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//
	out := struct {
		Count int
	}{
		0,
	}

	//
	resp, err := ssdbc.Do("zrrange", Z_CHALLENGE, 0, 1)
	if len(resp) < 2 {
		lwutil.WriteResponse(w, out)
		return
	}
	lwutil.CheckSsdbError(resp, err)

	//out
	count, err := strconv.ParseInt(resp[1], 10, 32)
	lwutil.CheckError(err, "")

	out.Count = int(count)

	lwutil.WriteResponse(w, out)
}

func apiChallengeList(w http.ResponseWriter, r *http.Request) {
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
		Offset int
		Limit  int
	}

	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")
	if in.Limit > 50 {
		in.Limit = 50
	}

	//multi_hget
	cmds := make([]interface{}, in.Limit+2)
	cmds[0] = "multi_hget"
	cmds[1] = H_CHALLENGE
	challengeId := in.Offset + 1
	for i := 0; i < in.Limit; i++ {
		cmds[2+i] = challengeId
		challengeId++
	}
	resp, err := ssdb.Do(cmds...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	if len(resp) == 0 {
		w.Write([]byte("[]"))
		return
	}

	//out
	type OutChallenge struct {
		Challenge
		CupType int
	}

	num := len(resp) / 2
	out := make([]OutChallenge, num)
	for i := 0; i < num; i++ {
		err = json.Unmarshal([]byte(resp[i*2+1]), &out[i])
		lwutil.CheckError(err, "")
	}

	//cup type
	cmds = make([]interface{}, 0, num+2)
	cmds = append(cmds, "multi_hget")
	cmds = append(cmds, H_CHALLENGE_PLAY)

	//index map:
	//map[playKey:string]indexInOut:int
	idxMap := map[string]int{}
	for i := 0; i < num; i++ {
		playKey := makeChallengePlaySubkey(out[i].Id, session.Userid)
		idxMap[playKey] = i
		cmds = append(cmds, playKey)
	}

	//get challenge play
	resp, err = ssdb.Do(cmds...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	var play ChallengePlay
	playNum := len(resp) / 2
	for i := 0; i < playNum; i++ {
		playKey := resp[i*2]
		err = json.Unmarshal([]byte(resp[i*2+1]), &play)
		lwutil.CheckError(err, "")

		out[idxMap[playKey]].CupType = play.CupType
	}

	//challengeRewards
	for i := 0; i < num; i++ {
		if out[i].ChallengeRewards == nil {
			out[i].ChallengeRewards = _conf.ChallengeRewards
		}
	}

	lwutil.WriteResponse(w, out)
}

func apiChallengeDel(w http.ResponseWriter, r *http.Request) {
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
		ChallengeId int
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//check exist
	resp, err := ssdb.Do("hexists", H_CHALLENGE, in.ChallengeId)
	lwutil.CheckError(err, "")
	if resp[1] != "1" {
		lwutil.SendError("err_exist", fmt.Sprintf("challenge not exist:id=", in.ChallengeId))
	}

	//del
	resp, err = ssdb.Do("hdel", H_CHALLENGE, in.ChallengeId)
	lwutil.CheckSsdbError(resp, err)

	resp, err = ssdb.Do("zdel", Z_CHALLENGE, in.ChallengeId)
	lwutil.CheckSsdbError(resp, err)

	lwutil.WriteResponse(w, in)
}

func apiChallengeMod(w http.ResponseWriter, r *http.Request) {
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
	var in Challenge
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//get challange
	resp, err := ssdb.Do("hexists", H_CHALLENGE, in.Id)
	lwutil.CheckError(err, "")
	if resp[1] != "1" {
		lwutil.SendError("err_exist", fmt.Sprintf("challenge not exist:id=", in.Id))
	}

	//get pack
	pack, err := getPack(ssdb, in.PackId)
	lwutil.CheckError(err, "")

	in.PackTitle = pack.Title
	in.Thumb = pack.Thumb

	//set
	js, err := json.Marshal(in)
	lwutil.CheckError(err, "")
	resp, err = ssdb.Do("hset", H_CHALLENGE, in.Id, js)
	lwutil.CheckSsdbError(resp, err)

	lwutil.WriteResponse(w, in)
}

func apiChallengeGetPlay(w http.ResponseWriter, r *http.Request) {
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
		ChallengeId int64
	}

	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//
	play := getChallengePlay(ssdb, in.ChallengeId, session.Userid)
	lwutil.WriteResponse(w, play)
}

func apiChallengeSubmitScore(w http.ResponseWriter, r *http.Request) {
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
		ChallengeId int64
		Score       int
		Checksum    string
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	playerKey := makePlayerInfoKey(session.Userid)

	//check challengeId
	var currChallengeId int64
	err = ssdb.HGet(playerKey, playerCurrChallengeId, &currChallengeId)
	lwutil.CheckError(err, "")
	if in.ChallengeId > currChallengeId {
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

	//check challenge play
	play := getChallengePlay(ssdb, in.ChallengeId, session.Userid)

	//update score
	scoreUpdate := false
	reward := 0
	oldScore := play.HighScore
	if play.HighScore == 0 {
		play.HighScore = in.Score
		scoreUpdate = true
	} else {
		if in.Score > play.HighScore {
			play.HighScore = in.Score
			scoreUpdate = true
		}
	}

	//
	if scoreUpdate {
		if oldScore == 0 {
			oldScore = math.MinInt32
		}

		//get challenge
		resp, err := ssdb.Do("hget", H_CHALLENGE, in.ChallengeId)
		lwutil.CheckSsdbError(resp, err)

		challenge := Challenge{}
		err = json.Unmarshal([]byte(resp[1]), &challenge)
		lwutil.CheckError(err, "")

		play.CupType = 0
		challengeRewards := challenge.ChallengeRewards
		if challengeRewards == nil {
			challengeRewards = _conf.ChallengeRewards
		}
		for i, sec := range challenge.ChallengeSecs {
			refScore := -int(sec * 1000)
			if refScore > oldScore && refScore <= in.Score {
				reward += challengeRewards[i]
			}
			if play.CupType == 0 && in.Score >= refScore {
				play.CupType = i + 1
			}
		}
	}

	//save play
	if scoreUpdate {
		saveChallengePlay(ssdb, play)
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
	if currChallengeId == in.ChallengeId && play.CupType > 0 {
		currChallengeId++
		resp, err := ssdb.Do("hincr", playerKey, playerCurrChallengeId, 1)
		lwutil.CheckSsdbError(resp, err)
	}

	//out
	out := struct {
		Reward          int
		Money           int64
		TotalReward     int64
		CupType         int
		CurrChallengeId int64
	}{
		reward,
		newMoney,
		totalReward,
		play.CupType,
		currChallengeId,
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
		ChallengeId int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	playerKey := makePlayerInfoKey(session.Userid)

	//check challengeId
	var currChallengeId int64
	err = ssdb.HGet(playerKey, playerCurrChallengeId, &currChallengeId)
	lwutil.CheckError(err, "")
	if in.ChallengeId != currChallengeId {
		lwutil.SendError("err_invalid_challenge", "")
	}

	//challenge must not exist
	resp, err := ssdb.Do("hget", H_CHALLENGE, in.ChallengeId)
	lwutil.CheckError(err, "")
	if resp[0] != "not_found" {
		lwutil.SendError("err_challenge_exist", "")
	}

	//add chanllengeId
	resp, err = ssdb.Do("hincr", playerKey, playerCurrChallengeId, 1)
	lwutil.CheckSsdbError(resp, err)
	currChallengeId++

	//add money
	addMoney := 100 + rand.Int()%400
	resp, err = ssdb.Do("hincr", playerKey, playerMoney, addMoney)
	lwutil.CheckSsdbError(resp, err)
	money, err := strconv.ParseInt(resp[1], 10, 64)
	lwutil.CheckError(err, "")

	//out
	out := struct {
		AddMoney        int
		Money           int64
		CurrChallengeId int64
	}{
		addMoney,
		money,
		currChallengeId,
	}
	lwutil.WriteResponse(w, out)
}

func regChallenge() {
	http.Handle("/challenge/count", lwutil.ReqHandler(apiChallengeCount))
	http.Handle("/challenge/list", lwutil.ReqHandler(apiChallengeList))
	//http.Handle("/challenge/del", lwutil.ReqHandler(apiChallengeDel))
	http.Handle("/challenge/mod", lwutil.ReqHandler(apiChallengeMod))
	http.Handle("/challenge/getPlay", lwutil.ReqHandler(apiChallengeGetPlay))
	http.Handle("/challenge/submitScore", lwutil.ReqHandler(apiChallengeSubmitScore))
	http.Handle("/challenge/passMissingChallenge", lwutil.ReqHandler(apiPassMissingChallenge))
}
