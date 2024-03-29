package main

import (
	"./ssdb"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/golang/glog"
	"github.com/henyouqian/lwutil"
)

const (
	H_ECO_RECORD        = "H_ECO_RECORD" //subkey:goldCoinRecordId value:GoldCoinRecord
	ECO_RECORD_SERIAL   = "ECO_RECORD_SERIAL"
	Z_ECO_PLAYER_MON    = "Z_ECO_PLAYER_MON"    //key:Z_ECO_PLAYER_MON/playerId subkey:goldCoinRecordId score:time
	Z_ECO_DAILY_MON     = "Z_ECO_DAILY_MON"     //key:Z_ECO_DAILY_MON/date subkey:goldCoinRecordId score:time
	H_ECO_DAILY_COUNTER = "H_ECO_DAILY_COUNTER" //key:H_ECO_DAILY_COUNTER/date subkey:whatCounter value:count

	ECO_FORWHAT_IAP          = "iap coin+"
	ECO_FORWHAT_MATCHBEGIN   = "match begin coin-"
	ECO_FORWHAT_MATCHPRIZE   = "match prize+"
	ECO_FORWHAT_PUBLISHPRIZE = "publish prize+"
	ECO_FORWHAT_BUYECARD     = "buy ecard prize-"
	ECO_FORWHAT_ADMIN_COIN   = "admin coin+"
	ECO_FORWHAT_ADMIN_PRIZE  = "admin prize+"

	//whatCounter
	ECO_DAILY_COUNTER_IAP          = "ECO_DAILY_COUNTER_IAP"          //count:goldCoin
	ECO_DAILY_COUNTER_MATCHBEGIN   = "ECO_DAILY_COUNTER_MATCHBEGIN"   //count:goldCoin
	ECO_DAILY_COUNTER_MATCHPRIZE   = "ECO_DAILY_COUNTER_MATCHPRIZE"   //count:prize
	ECO_DAILY_COUNTER_PUBLISHPRIZE = "ECO_DAILY_COUNTER_PUBLISHPRIZE" //count:prize
	ECO_DAILY_COUNTER_BUYECARD     = "ECO_DAILY_COUNTER_BUYECARD"     //count:prize
	ECO_DAILY_COUNTER_ADMIN_COIN   = "ECO_DAILY_COUNTER_ADMIN_COIN"   //count:goldCoin
	ECO_DAILY_COUNTER_ADMIN_PRIZE  = "ECO_DAILY_COUNTER_ADMIN_PRIZE"  //count:prize
)

type EcoRecord struct {
	UserId  int64
	Count   int
	ForWhat string
	Time    int64
}

type OutEcoRecord struct {
	Id      int64
	ForWhat string
	Count   int
	Time    string
}

func monitorGlog() {
	glog.Info("")
}

func makeEcoPlayerKey(userId int64) string {
	return fmt.Sprintf("%s/%d", Z_ECO_PLAYER_MON, userId)
}

func makeEcoDailyKey(date string) string {
	return fmt.Sprintf("%s/%s", Z_ECO_DAILY_MON, date)
}

func makeEcoDailyCountKey(date string) string {
	return fmt.Sprintf("%s/%s", H_ECO_DAILY_COUNTER, date)
}

func addEcoRecord(ssdbc *ssdb.Client, userId int64, count int, forWhat string) (err error) {
	id := GenSerial(ssdbc, ECO_RECORD_SERIAL)
	now := lwutil.GetRedisTime()

	var record EcoRecord
	record.UserId = userId
	record.Count = count
	record.ForWhat = forWhat
	record.Time = now.Unix()

	js, err := json.Marshal(record)
	if err != nil {
		return err
	}

	//add to hash
	_, err = ssdbc.Do("hset", H_ECO_RECORD, id, js)
	if err != nil {
		return err
	}

	//add to player zset
	key := makeEcoPlayerKey(userId)
	_, err = ssdbc.Do("zset", key, id, record.Time)
	if err != nil {
		return err
	}

	dateStr := now.Format("2006-01-02")

	// //add to daily zset
	// key = makeEcoDailyKey(dateStr)
	// _, err = ssdbc.Do("zset", key, id, id)
	// if err != nil {
	// 	return err
	// }

	//counter
	key = makeEcoDailyCountKey(dateStr)
	count *= 100
	if forWhat == ECO_FORWHAT_IAP {
		_, err = ssdbc.Do("hincr", key, ECO_DAILY_COUNTER_IAP, count)
	} else if forWhat == ECO_FORWHAT_MATCHBEGIN {
		_, err = ssdbc.Do("hincr", key, ECO_DAILY_COUNTER_MATCHBEGIN, count)
	} else if forWhat == ECO_FORWHAT_MATCHPRIZE {
		_, err = ssdbc.Do("hincr", key, ECO_DAILY_COUNTER_MATCHPRIZE, count)
	} else if forWhat == ECO_FORWHAT_PUBLISHPRIZE {
		_, err = ssdbc.Do("hincr", key, ECO_DAILY_COUNTER_PUBLISHPRIZE, count)
	} else if forWhat == ECO_FORWHAT_BUYECARD {
		_, err = ssdbc.Do("hincr", key, ECO_DAILY_COUNTER_BUYECARD, count)
	} else if forWhat == ECO_FORWHAT_ADMIN_COIN {
		_, err = ssdbc.Do("hincr", key, ECO_DAILY_COUNTER_ADMIN_COIN, count)
	} else if forWhat == ECO_FORWHAT_ADMIN_PRIZE {
		_, err = ssdbc.Do("hincr", key, ECO_DAILY_COUNTER_ADMIN_PRIZE, count)
	}

	return err
}

func apiEcoPlayerList(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	checkAdmin(session)

	//in
	var in struct {
		UserId  int64
		StartId int64
		Limit   int
		Time    string
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if in.StartId == 0 {
		in.StartId = math.MaxInt64
	}
	if in.Limit < 0 {
		in.Limit = 0
	} else if in.Limit > 50 {
		in.Limit = 50
	}

	tUnix := int64(math.MaxInt64)
	if in.Time != "" {
		t, err := time.ParseInLocation("2006-01-02T15:04:05", in.Time, time.Local)
		lwutil.CheckError(err, "")
		tUnix = t.Unix()

		glog.Info(t)
	}

	//
	zkey := makeEcoPlayerKey(in.UserId)
	vals, err := zrscanGet(ssdbc, zkey, in.StartId, tUnix, in.Limit, H_ECO_RECORD)
	lwutil.CheckError(err, "")
	glog.Info(vals)

	out := make([]OutEcoRecord, 0, 16)
	num := len(vals) / 2
	for i := 0; i < num; i++ {
		k := vals[i*2]
		v := vals[i*2+1]

		var rec EcoRecord
		err := json.Unmarshal([]byte(v), &rec)
		lwutil.CheckError(err, "")

		outRecord := OutEcoRecord{}
		outRecord.ForWhat = rec.ForWhat
		outRecord.Count = rec.Count

		outRecord.Id, err = strconv.ParseInt(k, 10, 64)
		lwutil.CheckError(err, "")

		t := time.Unix(rec.Time, 0)
		outRecord.Time = t.Format("2006-01-02T15:04:05")
		out = append(out, outRecord)
	}
	lwutil.WriteResponse(w, out)
}

func apiEcoDailyCount(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	checkAdmin(session)

	//in
	var in struct {
		Date string
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//
	key := makeEcoDailyCountKey(in.Date)
	resp, err := ssdbc.Do("hgetall", key)
	lwutil.CheckError(err, "")
	if resp[0] == ssdb.NOT_FOUND {
		w.Write([]byte("{}"))
	}
	resp = resp[1:]

	//out
	out := make(map[string]float32)

	num := len(resp) / 2
	for i := 0; i < num; i++ {
		f, _ := strconv.ParseFloat(resp[i*2+1], 32)
		out[resp[i*2]] = float32(f * 0.01)
	}

	lwutil.WriteResponse(w, out)
}

func regEcoMonitor() {
	http.Handle("/ecoMonitor/ecoPlayerList", lwutil.ReqHandler(apiEcoPlayerList))
	http.Handle("/ecoMonitor/ecoDailyCount", lwutil.ReqHandler(apiEcoDailyCount))
}
