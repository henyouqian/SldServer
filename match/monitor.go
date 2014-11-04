package main

import (
	"./ssdb"
	"encoding/json"
	"fmt"
	"math"
	"net/http"

	"github.com/golang/glog"
	"github.com/henyouqian/lwutil"
)

const (
	H_ECO_RECORD        = "H_ECO_RECORD" //subkey:goldCoinRecordId value:GoldCoinRecord
	ECO_RECORD_SERIAL   = "ECO_RECORD_SERIAL"
	Z_ECO_PLAYER_MON    = "Z_ECO_PLAYER_MON"    //key:Z_ECO_PLAYER_MON/playerId subkey:goldCoinRecordId value:time
	Z_ECO_DAILY_MON     = "Z_ECO_DAILY_MON"     //key:Z_ECO_DAILY_MON/date subkey:goldCoinRecordId value:time
	H_ECO_DAILY_COUNTER = "H_ECO_DAILY_COUNTER" //key:H_ECO_DAILY_COUNTER/date subkey:whatCounter value:count

	ECO_FORWHAT_IAP           = "iap coin+"
	ECO_FORWHAT_MATCHBEGIN    = "match begin coin-"
	ECO_FORWHAT_MATCHREWARD   = "match reward coupon+"
	ECO_FORWHAT_PUBLISHREWARD = "publish reward coupon+"
	ECO_FORWHAT_BUYECARD      = "buy ecard coupon-"

	//whatCounter
	ECO_DAILY_COUNTER_IAP           = "ECO_DAILY_COUNTER_IAP"
	ECO_DAILY_COUNTER_MATCHBEGIN    = "ECO_DAILY_COUNTER_MATCHBEGIN"
	ECO_DAILY_COUNTER_MATCHREWARD   = "ECO_DAILY_COUNTER_MATCHREWARD"
	ECO_DAILY_COUNTER_PUBLISHREWARD = "ECO_DAILY_COUNTER_PUBLISHREWARD"
	ECO_DAILY_COUNTER_BUYECARD      = "ECO_DAILY_COUNTER_BUYECARD"
)

type EcoRecord struct {
	UserId  int64
	Count   float32
	ForWhat string
	Time    int64
}

func monitorGlog() {
	glog.Info("")
}

func makeEcoDailyKey(date string) string {
	return fmt.Sprintf("%s/%s", Z_ECO_DAILY_MON, date)
}

func makeEcoDailyCountKey(date string) string {
	return fmt.Sprintf("%s/%s", H_ECO_DAILY_COUNTER, date)
}

func addEcoRecord(ssdbc *ssdb.Client, userId int64, count float32, forWhat string) (err error) {
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
	key := fmt.Sprintf("%s/%d", Z_ECO_PLAYER_MON, userId)
	_, err = ssdbc.Do("zset", key, id, id)
	if err != nil {
		return err
	}

	//add to daily zset
	dateStr := now.Format("2006-01-02")
	key = makeEcoDailyKey(dateStr)
	_, err = ssdbc.Do("zset", key, id, id)
	if err != nil {
		return err
	}

	//counter
	key = makeEcoDailyCountKey(dateStr)
	if forWhat == ECO_FORWHAT_IAP {
		_, err = ssdbc.Do("hincr", key, ECO_DAILY_COUNTER_IAP, count)
	} else if forWhat == ECO_FORWHAT_MATCHBEGIN {
		_, err = ssdbc.Do("hincr", key, ECO_DAILY_COUNTER_MATCHBEGIN, count)
	} else if forWhat == ECO_FORWHAT_MATCHREWARD {
		_, err = ssdbc.Do("hincr", key, ECO_DAILY_COUNTER_MATCHREWARD, count)
	} else if forWhat == ECO_FORWHAT_PUBLISHREWARD {
		_, err = ssdbc.Do("hincr", key, ECO_DAILY_COUNTER_PUBLISHREWARD, count)
	} else if forWhat == ECO_FORWHAT_BUYECARD {
		_, err = ssdbc.Do("hincr", key, ECO_DAILY_COUNTER_BUYECARD, count)
	}

	return err
}

func apiEcoDailyList(w http.ResponseWriter, r *http.Request) {
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
		Date    string
		StartId int64
		Limit   int
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if in.Limit < 0 {
		in.Limit = 0
	} else if in.Limit > 50 {
		in.Limit = 50
	}

	//zrscan
	if in.StartId == 0 {
		in.StartId = math.MaxInt64
	}
	key := makeEcoDailyKey(in.Date)
	resp, err := ssdbc.Do("zrscan", key, in.StartId, in.StartId, "", in.Limit)
	lwutil.CheckSsdbError(resp, err)

	resp = resp[1:]
	if len(resp) == 0 {
		w.Write([]byte("[]"))
		return
	}

	//out
	out := make([]EcoRecord, 0)

	//multi_hget
	cmds := make([]interface{}, 2)
	cmds[0] = "multi_hget"
	cmds[1] = H_ECO_RECORD
	for _, v := range resp {
		cmds = append(cmds, v)
	}

	resp, err = ssdbc.Do(cmds...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	num := len(resp) / 2
	for i := 0; i < num; i++ {
		valStr := resp[i*2+1]

		var record EcoRecord
		err = json.Unmarshal([]byte(valStr), &record)
		lwutil.CheckError(err, "")

		out = append(out, record)
	}

	lwutil.WriteResponse(w, out)
}

func regMonitor() {
	http.Handle("/monitor/ecoDailyList", lwutil.ReqHandler(apiEcoDailyList))
}
