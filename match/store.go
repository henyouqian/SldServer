package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/golang/glog"
	"github.com/henyouqian/lwutil"
	"net/http"
	"strconv"
)

const (
	H_COUPON_ITEM_TYPE = "H_COUPON_ITEM_TYPE" //subkey:typeKey value:CouponItemType
	H_COUPON_ITEM      = "H_COUPON_ITEM"      //subkey:itemId, value:couponItemJson
	Q_COUPON_ITEM      = "Q_COUPON_ITEM"
	H_COUPON_CODE      = "H_COUPON_CODE"      //subkey:provider/code value:couponItemId
	Z_PLAYER_COUPON    = "Z_PLAYER_COUPON"    //key:Z_PLAYER_COUPON/playerId, subkey:couponItemId, score:time
	Z_COUPON_PURCHASED = "Z_COUPON_PURCHASED" //subkey:couponItemId, score:time
)

var (
	COUPON_ITME_PROVIDERS = map[string]bool{
		"amazon": true,
		"jd":     true,
	}
)

type GameCoinPack struct {
	Price   int
	CoinNum int
}

type CouponItemType struct {
	Key         string
	Name        string
	Provider    string
	RmbPrice    int
	CouponPrice int
	Num         int
}

type CouponItem struct {
	Id          int64
	TypeKey     string
	CouponCode  string
	ExpireDate  string
	GenDate     string
	UserGetDate string
}

var (
	gameCoinPacks = []GameCoinPack{
		{50, 1},
		{100, 3},
		{150, 5},
		{250, 10},
	}

	iapProducts = map[string]int{
	// "com.liwei.pin.coin6":   600,
	// "com.liwei.pin.coin30":  3000 + 300,
	// "com.liwei.pin.coin68":  6800 + 1000,
	// "com.liwei.pin.coin128": 12800 + 2500,
	// "com.liwei.pin.coin328": 32800 + 10000,
	// "com.liwei.pin.coin588": 58800 + 25000,
	}
)

func makeCouponItemQueueKey(typeKey string) string {
	return fmt.Sprintf("Q_COUPON_ITEM/%s", typeKey)
}

func makeCouponCodeSubkey(provider string, code string) string {
	return fmt.Sprintf("%s/%s", provider, code)
}

func makePlayerCouponZsetKey(playerId int64) string {
	return fmt.Sprintf("%s/%d", Z_PLAYER_COUPON, playerId)
}

func glogStore() {
	glog.Info("")
}

func apiListGameCoinPack(w http.ResponseWriter, r *http.Request) {
	out := struct {
		GameCoinPacks []GameCoinPack
	}{
		GameCoinPacks: gameCoinPacks,
	}
	lwutil.WriteResponse(w, out)
}

func apiBuyGameCoin(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//in
	var in struct {
		EventId        int64
		GameCoinPackId int
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if in.GameCoinPackId < 0 || in.GameCoinPackId >= len(gameCoinPacks) {
		lwutil.SendError("err_game_coin_id", "")
	}

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//
	var gameCoinPack GameCoinPack
	gameCoinPack = gameCoinPacks[in.GameCoinPackId]

	//get event info
	resp, err := ssdb.Do("hget", H_EVENT, in.EventId)
	if resp[0] == "not_found" {
		lwutil.SendError("err_event_id", "")
	}
	lwutil.CheckSsdbError(resp, err)

	event := Event{}
	err = json.Unmarshal([]byte(resp[1]), &event)

	now := lwutil.GetRedisTimeUnix()
	if event.HasResult || now >= event.EndTime {
		lwutil.SendError("err_event_closed", "")
	}

	//check money
	var money int64
	playerKey := makePlayerInfoKey(session.Userid)
	ssdb.HGet(playerKey, PLAYER_MONEY, &money)

	if int(money) < gameCoinPack.Price {
		lwutil.SendError("err_money", "")
	}

	//get record
	recordKey := makeEventPlayerRecordSubkey(in.EventId, session.Userid)
	resp, err = ssdb.Do("hget", H_EVENT_PLAYER_RECORD, recordKey)
	lwutil.CheckSsdbError(resp, err)

	record := EventPlayerRecord{}
	err = json.Unmarshal([]byte(resp[1]), &record)
	lwutil.CheckError(err, "")

	//set game coin number
	record.GameCoinNum += gameCoinPack.CoinNum

	//save game coin
	js, err := json.Marshal(record)
	lwutil.CheckError(err, "")
	resp, err = ssdb.Do("hset", H_EVENT_PLAYER_RECORD, recordKey, js)
	lwutil.CheckSsdbError(resp, err)

	//spend money
	money -= int64(gameCoinPack.Price)
	resp, err = ssdb.Do("hincr", playerKey, PLAYER_MONEY, -gameCoinPack.Price)

	//out
	out := struct {
		Money       int64
		GameCoinNum int
	}{
		money,
		record.GameCoinNum,
	}
	lwutil.WriteResponse(w, out)
}

func apiListIapProductId(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")

	out := make([]string, len(iapProducts))
	i := 0
	for k, _ := range iapProducts {
		out[i] = k
		i++
	}

	lwutil.WriteResponse(w, out)
}

func apiGetIapSecret(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//player
	secret := lwutil.GenUUID()
	playerKey := makePlayerInfoKey(session.Userid)
	err = ssdb.HSet(playerKey, PLAYER_IAP_SECRET, secret)
	lwutil.CheckError(err, "")

	//out
	out := map[string]string{
		"Secret": secret,
	}
	lwutil.WriteResponse(w, out)
}

func apiBuyIap(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//in
	var in struct {
		ProductId string
		Checksum  string
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	addMoney, exist := iapProducts[in.ProductId]
	if !exist {
		lwutil.SendError("err_product_id", "")
	}

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//get iap secret
	playerKey := makePlayerInfoKey(session.Userid)
	var secret string
	err = ssdb.HGet(playerKey, PLAYER_IAP_SECRET, &secret)
	lwutil.CheckError(err, "")

	//check checksum
	if secret == "" {
		lwutil.SendError("err_secret", "")
	}
	checksum := fmt.Sprintf("%s%d%s,", secret, session.Userid, session.Username)
	hasher := sha1.New()
	hasher.Write([]byte(checksum))
	checksum = hex.EncodeToString(hasher.Sum(nil))
	if in.Checksum != checksum {
		lwutil.SendError("err_checksum", checksum)
	}

	//set money
	resp, err := ssdb.Do("hincr", playerKey, PLAYER_MONEY, addMoney)
	lwutil.CheckSsdbError(resp, err)
	money, err := strconv.ParseInt(resp[1], 10, 64)

	//update secret
	err = ssdb.HSet(playerKey, PLAYER_IAP_SECRET, "")
	lwutil.CheckError(err, "")

	//out
	out := map[string]int64{
		"AddMoney": int64(addMoney),
		"Money":    money,
	}
	lwutil.WriteResponse(w, out)
}

func apiAddCouponItemType(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//check admin
	checkAdmin(session)

	//in
	var in CouponItemType
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")
	in.Num = 0

	//check exist
	resp, err := ssdbc.Do("hexists", H_COUPON_ITEM_TYPE, in.Key)
	lwutil.CheckSsdbError(resp, err)
	if ssdbCheckExists(resp) {
		lwutil.SendError("err_exist", "key exist")
	}

	//check provider
	if !COUPON_ITME_PROVIDERS[in.Provider] {
		lwutil.SendError("err_provider", "invalid provider")
	}

	//check price
	if in.RmbPrice*10 != in.CouponPrice {
		lwutil.SendError("err_price", "in.RmbPrice * 10 != in.CouponPrice")
	}

	//json
	js, err := json.Marshal(in)
	lwutil.CheckError(err, "")

	//ssdb hset
	resp, err = ssdbc.Do("hset", H_COUPON_ITEM_TYPE, in.Key, js)
	lwutil.CheckSsdbError(resp, err)

	//out
	lwutil.WriteResponse(w, in)
}

func apiDelCouponItemType(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//check admin
	checkAdmin(session)

	//in
	var in struct {
		Key string
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//check queue empty
	qkey := makeCouponItemQueueKey(in.Key)
	resp, err := ssdbc.Do("qsize", qkey)
	lwutil.CheckSsdbError(resp, err)

	qsize, err := strconv.Atoi(resp[1])
	lwutil.CheckError(err, "")
	if qsize > 0 {
		lwutil.SendError("not_empty", "queue is not empty")
	}

	//ssdb hdel
	resp, err = ssdbc.Do("hdel", H_COUPON_ITEM_TYPE, in.Key)
	lwutil.CheckSsdbError(resp, err)

	resp, err = ssdbc.Do("hdel", H_COUPON_ITEM_TYPE, in.Key)
	lwutil.CheckSsdbError(resp, err)

	//out
	lwutil.WriteResponse(w, in)
}

func apiListCouponItemType(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//session
	_, err = findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//ssdb hgetall
	resp, err := ssdbc.Do("hgetall", H_COUPON_ITEM_TYPE)
	lwutil.CheckSsdbError(resp, err)

	resp = resp[1:]
	num := len(resp) / 2
	ciTypes := make([]CouponItemType, 0, num)
	for i := 0; i < num; i++ {
		js := resp[i*2+1]
		var ciType CouponItemType
		err = json.Unmarshal([]byte(js), &ciType)
		lwutil.CheckError(err, "")
		ciTypes = append(ciTypes, ciType)
	}

	//out
	lwutil.WriteResponse(w, ciTypes)
}

func apiAddCouponItem(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//check admin
	checkAdmin(session)

	//in
	var in CouponItem
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if len(in.CouponCode) == 0 {
		lwutil.SendError("err_code", "need coupon code")
	}

	//check type key
	resp, err := ssdbc.Do("hget", H_COUPON_ITEM_TYPE, in.TypeKey)
	lwutil.CheckSsdbError(resp, err)
	if len(resp) < 2 {
		lwutil.SendError("err_not_exist", "key not exist")
	}
	var itemType CouponItemType
	err = json.Unmarshal([]byte(resp[1]), &itemType)
	lwutil.CheckError(err, "")

	//check code exist
	codeSubkey := makeCouponCodeSubkey(itemType.Provider, in.CouponCode)
	resp, err = ssdbc.Do("hexists", H_COUPON_CODE, codeSubkey)
	lwutil.CheckSsdbError(resp, err)
	if ssdbCheckExists(resp) {
		lwutil.SendError("err_exists", "code exist")
	}

	//hset
	in.Id = GenSerial(ssdbc, "COUPON_ITEM_SERIAL")
	js, err := json.Marshal(in)
	lwutil.CheckError(err, "")
	resp, err = ssdbc.Do("hset", H_COUPON_ITEM, in.Id, js)
	lwutil.CheckSsdbError(resp, err)

	//qpush_back
	qkey := makeCouponItemQueueKey(in.TypeKey)
	resp, err = ssdbc.Do("qpush_back", qkey, in.Id)
	lwutil.CheckSsdbError(resp, err)

	//add to H_COUPON_CODE
	resp, err = ssdbc.Do("hset", H_COUPON_CODE, codeSubkey, in.Id)
	lwutil.CheckSsdbError(resp, err)

	//update num
	resp, err = ssdbc.Do("qsize", qkey)
	lwutil.CheckSsdbError(resp, err)
	num, err := strconv.Atoi(resp[1])
	lwutil.CheckError(err, "")
	itemType.Num = num
	js, err = json.Marshal(itemType)
	lwutil.CheckError(err, "")
	resp, err = ssdbc.Do("hset", H_COUPON_ITEM_TYPE, in.TypeKey, js)
	lwutil.CheckSsdbError(resp, err)

	//out
	lwutil.WriteResponse(w, in)
}

func apiBuyCouponItem(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var in struct {
		TypeKey string
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//check item type
	resp, err := ssdbc.Do("hget", H_COUPON_ITEM_TYPE, in.TypeKey)
	lwutil.CheckSsdbError(resp, err)
	if len(resp) < 2 {
		lwutil.SendError("err_not_exist", "key not exist")
	}
	var itemType CouponItemType
	err = json.Unmarshal([]byte(resp[1]), &itemType)
	lwutil.CheckError(err, "")

	if itemType.Num == 0 {
		lwutil.SendError("err_zero", "item count = 0")
	}

	//check player coupon
	playerKey := makePlayerInfoKey(session.Userid)
	resp, err = ssdbc.Do("hget", playerKey, PLAYER_COUPON)
	lwutil.CheckSsdbError(resp, err)
	playerCoupon, err := strconv.Atoi(resp[1])
	lwutil.CheckError(err, "")
	if playerCoupon < itemType.CouponPrice {
		lwutil.SendError("err_not_enough", "not enough coupon")
	}

	//buy, pop from coupon item queue
	couponItemQueueKey := makeCouponItemQueueKey(in.TypeKey)
	resp, err = ssdbc.Do("qpop_front", couponItemQueueKey)
	lwutil.CheckSsdbError(resp, err)
	itemId, err := strconv.Atoi(resp[1])
	lwutil.CheckError(err, "")

	//buy, add to player coupon zset
	playerCouponZKey := makePlayerCouponZsetKey(session.Userid)
	score := lwutil.GetRedisTimeUnix()
	resp, err = ssdbc.Do("zset", playerCouponZKey, itemId, score)
	lwutil.CheckSsdbError(resp, err)

	//buy, sub player coupon num
	resp, err = ssdbc.Do("hincr", playerKey, PLAYER_COUPON, -itemType.CouponPrice)
	lwutil.CheckSsdbError(resp, err)

	//update coupon type's coupon num
	resp, err = ssdbc.Do("qsize", couponItemQueueKey)
	lwutil.CheckSsdbError(resp, err)
	num, err := strconv.Atoi(resp[1])
	lwutil.CheckError(err, "")
	itemType.Num = num
	js, err := json.Marshal(itemType)
	lwutil.CheckError(err, "")
	resp, err = ssdbc.Do("hset", H_COUPON_ITEM_TYPE, in.TypeKey, js)
	lwutil.CheckSsdbError(resp, err)

	//add to purchased list
	resp, err = ssdbc.Do("zset", Z_COUPON_PURCHASED, itemId, score)
	lwutil.CheckSsdbError(resp, err)

	//out
	out := map[string]interface{}{
		"ItemId":       itemId,
		"PlayerCoupon": playerCoupon - itemType.CouponPrice,
	}
	lwutil.WriteResponse(w, out)
}

func regStore() {
	http.Handle("/store/listGameCoinPack", lwutil.ReqHandler(apiListGameCoinPack))
	http.Handle("/store/buyGameCoin", lwutil.ReqHandler(apiBuyGameCoin))
	http.Handle("/store/listIapProductId", lwutil.ReqHandler(apiListIapProductId))
	http.Handle("/store/getIapSecret", lwutil.ReqHandler(apiGetIapSecret))
	http.Handle("/store/buyIap", lwutil.ReqHandler(apiBuyIap))

	http.Handle("/store/addCouponItemType", lwutil.ReqHandler(apiAddCouponItemType))
	http.Handle("/store/delCouponItemType", lwutil.ReqHandler(apiDelCouponItemType))
	http.Handle("/store/listCouponItemType", lwutil.ReqHandler(apiListCouponItemType))

	http.Handle("/store/addCouponItem", lwutil.ReqHandler(apiAddCouponItem))
	http.Handle("/store/buyCouponItem", lwutil.ReqHandler(apiBuyCouponItem))
}
