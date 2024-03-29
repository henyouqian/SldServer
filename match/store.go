package main

import (
	"./ssdb"
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
	H_ECARD_TYPE          = "H_ECARD_TYPE" //subkey:typeKey value:ecardType
	H_ECARD               = "H_ECARD"      //subkey:id, value:ecardJson
	Q_ECARD               = "Q_ECARD"
	H_ECARD_CODE          = "H_ECARD_CODE"      //subkey:provider/code value:ecardId
	Z_PLAYER_ECARD        = "Z_PLAYER_ECARD"    //key:Z_PLAYER_ECARD/playerId, subkey:ecardId, score:time
	Z_ECARD_PURCHASED     = "Z_ECARD_PURCHASED" //subkey:ecardId, score:time
	KEY_ECARD_STORE_CLOSE = "KEY_ECARD_STORE_CLOSE"
)

type Provider struct {
	RechargeUrl string
	HelpText    string
}

const (
	AMAZON_HELP = `1.	点击“充值到我的账户”按钮进入充值页面。
2.	已有亚马逊账号的用户请直接登录，否则请先注册亚马逊账号后再登录。
3.	按照提示输入充值码（可直接粘贴，充值码已自动拷贝至剪贴板）。
4.	在结算过程中，礼品卡金额将被自动用于支付有效订单。
5.	当您的礼品卡余额不足以支付订单时，您需要同时选择其它支付方式支付订单的差额部分。
您也可以在结算过程中按照提示输入您的礼品卡充值码。选择“一键下单”服务时，您需要先将礼品卡充值至我的账户。`

	PROVIDER_KEY_AMAZON = "amazon"
)

var (
	ECARD_PROVIDERS = map[string]Provider{
		PROVIDER_KEY_AMAZON: Provider{"https://www.amazon.cn/gp/css/gc/payment/ref=gc_lp_cc", AMAZON_HELP},
	}
	_ecardStoreClose = false
)

type GameCoinPack struct {
	Price   int
	CoinNum int
}

type ECardType struct {
	Key       string
	Name      string
	Provider  string
	Thumb     string
	RmbPrice  int
	NeedPrize int
	Num       int
}

type ECard struct {
	Id          int64
	TypeKey     string
	Code        string
	ExpireDate  string
	GenDate     string
	UserGetDate string
	OwnerId     int64
	Title       string
	Provider    string
	RmbPrice    int
}

type OutEcard struct {
	ECard
	Provider
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
		// "com.lw.pin.goldcoin6":    300,
		// "com.lw.pin.goldcoin18":   900,
		// "com.lw.pin.goldcoin30":   1500,
		// "com.lw.pin.goldcoin60":   3000,
		// "com.lw.pin.goldcoin188":  10000,
		// "com.lw.pin.goldcoin388":  20000,
		// "com.lw.pin.goldcoin588":  30000,
		// "com.lw.pin.goldcoin998":  50000,
		// "com.lw.pin.goldcoin2998": 150000,
		// "com.lw.pin.goldcoin5898": 300000,

		// "com.lw.pin.gc6":    30,
		// "com.lw.pin.gc18":   90,
		// "com.lw.pin.gc25":   125,
		// "com.lw.pin.gc60":   300,
		// "com.lw.pin.gc188":  1000,
		// "com.lw.pin.gc388":  2000,
		// "com.lw.pin.gc588":  3000,
		// "com.lw.pin.gc998":  5000,
		// "com.lw.pin.gc2998": 15000,
		// "com.lw.pin.gc5898": 30000,

		"com.lw.mpin.goldcoin6":   30,
		"com.lw.mpin.goldcoin12":  60,
		"com.lw.mpin.goldcoin25":  125,
		"com.lw.mpin.goldcoin50":  250,
		"com.lw.mpin.goldcoin98":  500,
		"com.lw.mpin.goldcoin198": 1000,
		"com.lw.mpin.goldcoin298": 1500,
		"com.lw.mpin.goldcoin388": 2000,
		"com.lw.mpin.goldcoin488": 2500,
		"com.lw.mpin.goldcoin588": 3000,
	}
)

func makeEcardQueueKey(typeKey string) string {
	return fmt.Sprintf("Q_ECARD_ITEM/%s", typeKey)
}

func makeEcardCodeSubkey(provider string, code string) string {
	return fmt.Sprintf("%s/%s", provider, code)
}

func makeEcardTypeKey(provider string, rmbPrice int) string {
	return fmt.Sprintf("%s/%d", provider, rmbPrice)
}

func makePlayerEcardZsetKey(playerId int64) string {
	return fmt.Sprintf("%s/%d", Z_PLAYER_ECARD, playerId)
}

func glogStore() {
	glog.Info("")
}

func initStore() {
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	resp, err := ssdbc.Do("get", KEY_ECARD_STORE_CLOSE)
	lwutil.CheckError(err, "")
	if resp[0] == ssdb.NOT_FOUND {
		resp, err := ssdbc.Do("set", KEY_ECARD_STORE_CLOSE, 0)
		lwutil.CheckSsdbError(resp, err)
	} else {
		if resp[1] == "1" {
			_ecardStoreClose = true
		} else {
			_ecardStoreClose = false
		}
	}
}

func apiListGameCoinPack(w http.ResponseWriter, r *http.Request) {
	out := struct {
		GameCoinPacks []GameCoinPack
	}{
		GameCoinPacks: gameCoinPacks,
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

	addGoldCoin, exist := iapProducts[in.ProductId]
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

	//set goldCoin
	resp, err := ssdb.Do("hincr", playerKey, PLAYER_GOLD_COIN, addGoldCoin)
	lwutil.CheckSsdbError(resp, err)
	goldCoin, err := strconv.ParseInt(resp[1], 10, 64)

	//
	err = addEcoRecord(ssdb, session.Userid, addGoldCoin, ECO_FORWHAT_IAP)
	lwutil.CheckError(err, "")

	//update secret
	err = ssdb.HSet(playerKey, PLAYER_IAP_SECRET, "")
	lwutil.CheckError(err, "")

	//out
	out := map[string]int64{
		"AddGoldCoin": int64(addGoldCoin),
		"GoldCoin":    goldCoin,
	}
	lwutil.WriteResponse(w, out)
}

func apiAddEcardType(w http.ResponseWriter, r *http.Request) {
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
	var in ECardType
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")
	in.Num = 0

	//gen key
	in.Key = makeEcardTypeKey(in.Provider, in.RmbPrice)

	//check exist
	resp, err := ssdbc.Do("hexists", H_ECARD_TYPE, in.Key)
	lwutil.CheckSsdbError(resp, err)
	if ssdbCheckExists(resp) {
		lwutil.SendError("err_exist", "key exist")
	}

	//check provider
	_, ok := ECARD_PROVIDERS[in.Provider]
	if !ok {
		lwutil.SendError("err_provider", "invalid provider")
	}

	//check price
	if in.RmbPrice*1000 != in.NeedPrize {
		lwutil.SendError("err_price", "in.RmbPrice * 1000 != in.NeedPrize")
	}

	//check thumb
	if len(in.Thumb) == 0 {
		lwutil.SendError("err_thumb", "need thumb")
	}

	//json
	js, err := json.Marshal(in)
	lwutil.CheckError(err, "")

	//ssdb hset
	resp, err = ssdbc.Do("hset", H_ECARD_TYPE, in.Key, js)
	lwutil.CheckSsdbError(resp, err)

	//out
	lwutil.WriteResponse(w, in)
}

func apiEditEcardType(w http.ResponseWriter, r *http.Request) {
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
		Key   string
		Name  string
		Thumb string
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//check exist
	resp, err := ssdbc.Do("hget", H_ECARD_TYPE, in.Key)
	lwutil.CheckSsdbError(resp, err)
	var eCardType ECardType
	err = json.Unmarshal([]byte(resp[1]), &eCardType)
	lwutil.CheckError(err, "")

	//update
	eCardType.Name = in.Name
	eCardType.Thumb = in.Thumb

	//json
	js, err := json.Marshal(eCardType)
	lwutil.CheckError(err, "")

	//ssdb hset
	resp, err = ssdbc.Do("hset", H_ECARD_TYPE, in.Key, js)
	lwutil.CheckSsdbError(resp, err)

	//out
	lwutil.WriteResponse(w, eCardType)
}

func apiDelEcardType(w http.ResponseWriter, r *http.Request) {
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
	qkey := makeEcardQueueKey(in.Key)
	resp, err := ssdbc.Do("qsize", qkey)
	lwutil.CheckSsdbError(resp, err)

	qsize, err := strconv.Atoi(resp[1])
	lwutil.CheckError(err, "")
	if qsize > 0 {
		lwutil.SendError("not_empty", "queue is not empty")
	}

	//ssdb hdel
	resp, err = ssdbc.Do("hdel", H_ECARD_TYPE, in.Key)
	lwutil.CheckSsdbError(resp, err)

	//out
	lwutil.WriteResponse(w, in)
}

func apiListEcardType(w http.ResponseWriter, r *http.Request) {
	if _ecardStoreClose {
		w.Write([]byte("[]"))
		return
	}

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
	resp, err := ssdbc.Do("hgetall", H_ECARD_TYPE)
	lwutil.CheckSsdbError(resp, err)

	resp = resp[1:]
	num := len(resp) / 2
	ciTypes := make([]ECardType, 0, num)
	for i := 0; i < num; i++ {
		js := resp[i*2+1]
		var ciType ECardType
		err = json.Unmarshal([]byte(js), &ciType)
		lwutil.CheckError(err, "")
		ciTypes = append(ciTypes, ciType)
	}

	//out
	lwutil.WriteResponse(w, ciTypes)
}

func apiAddEcard(w http.ResponseWriter, r *http.Request) {
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
	var in ECard
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if len(in.Code) == 0 {
		lwutil.SendError("err_code", "need code")
	}

	in.TypeKey = makeEcardTypeKey(in.Provider, in.RmbPrice)

	//check type key
	resp, err := ssdbc.Do("hget", H_ECARD_TYPE, in.TypeKey)
	lwutil.CheckError(err, "")
	if resp[0] == ssdb.NOT_FOUND {
		lwutil.SendError("err_type", "type not found")
	}

	if len(resp) < 2 {
		lwutil.SendError("err_not_exist", "key not exist")
	}
	var cardType ECardType
	err = json.Unmarshal([]byte(resp[1]), &cardType)
	lwutil.CheckError(err, "")

	//check code exist
	codeSubkey := makeEcardCodeSubkey(cardType.Provider, in.Code)
	resp, err = ssdbc.Do("hexists", H_ECARD_CODE, codeSubkey)
	lwutil.CheckSsdbError(resp, err)
	if ssdbCheckExists(resp) {
		lwutil.SendError("err_exists", "code exist")
	}

	//set gen time
	now := lwutil.GetRedisTime()
	in.GenDate = now.Format("2006-01-02 15:04:05")

	//set title
	in.Title = cardType.Name
	in.Provider = cardType.Provider

	//hset
	in.Id = GenSerial(ssdbc, "ECARD_SERIAL")
	js, err := json.Marshal(in)
	lwutil.CheckError(err, "")
	resp, err = ssdbc.Do("hset", H_ECARD, in.Id, js)
	lwutil.CheckSsdbError(resp, err)

	//qpush_back
	qkey := makeEcardQueueKey(in.TypeKey)
	resp, err = ssdbc.Do("qpush_back", qkey, in.Id)
	lwutil.CheckSsdbError(resp, err)

	//add to H_ECARD_CODE
	resp, err = ssdbc.Do("hset", H_ECARD_CODE, codeSubkey, in.Id)
	lwutil.CheckSsdbError(resp, err)

	//update num
	resp, err = ssdbc.Do("qsize", qkey)
	lwutil.CheckSsdbError(resp, err)
	num, err := strconv.Atoi(resp[1])
	lwutil.CheckError(err, "")
	cardType.Num = num
	js, err = json.Marshal(cardType)
	lwutil.CheckError(err, "")
	resp, err = ssdbc.Do("hset", H_ECARD_TYPE, in.TypeKey, js)
	lwutil.CheckSsdbError(resp, err)

	//out
	lwutil.WriteResponse(w, in)
}

func checkEcardCode(provider string, code string) bool {
	if provider == PROVIDER_KEY_AMAZON {
		if len(code) != 16 || code[4] != '-' || code[11] != '-' {
			return false
		}
	}
	return true
}

func apiAddEcards(w http.ResponseWriter, r *http.Request) {
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
	var in []ECard
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	for _, ecard := range in {
		if len(ecard.Code) == 0 {
			lwutil.SendError("err_code", "need code")
		}

		//check ecard code
		if isReleaseServer() {
			if !checkEcardCode(ecard.Provider, ecard.Code) {
				lwutil.SendError("err_code", fmt.Sprintf("code format error:code=%s", ecard.Code))
			}
		}

		ecard.TypeKey = makeEcardTypeKey(ecard.Provider, ecard.RmbPrice)

		//check type key
		resp, err := ssdbc.Do("hget", H_ECARD_TYPE, ecard.TypeKey)
		lwutil.CheckError(err, "")
		if resp[0] == ssdb.NOT_FOUND {
			lwutil.SendError("err_type", "type not found")
		}

		if len(resp) < 2 {
			lwutil.SendError("err_not_exist", "key not exist")
		}
		var cardType ECardType
		err = json.Unmarshal([]byte(resp[1]), &cardType)
		lwutil.CheckError(err, "")

		//check code exist
		codeSubkey := makeEcardCodeSubkey(cardType.Provider, ecard.Code)
		resp, err = ssdbc.Do("hexists", H_ECARD_CODE, codeSubkey)
		lwutil.CheckSsdbError(resp, err)
		if ssdbCheckExists(resp) {
			lwutil.SendError("err_exists", fmt.Sprintf("code exist:", ecard.Code))
		}

		//set gen time
		now := lwutil.GetRedisTime()
		ecard.GenDate = now.Format("2006-01-02 15:04:05")

		//set title
		ecard.Title = cardType.Name
		ecard.Provider = cardType.Provider

		//hset
		ecard.Id = GenSerial(ssdbc, "ECARD_SERIAL")
		js, err := json.Marshal(ecard)
		lwutil.CheckError(err, "")
		resp, err = ssdbc.Do("hset", H_ECARD, ecard.Id, js)
		lwutil.CheckSsdbError(resp, err)

		//qpush_back
		qkey := makeEcardQueueKey(ecard.TypeKey)
		resp, err = ssdbc.Do("qpush_back", qkey, ecard.Id)
		lwutil.CheckSsdbError(resp, err)

		//add to H_ECARD_CODE
		resp, err = ssdbc.Do("hset", H_ECARD_CODE, codeSubkey, ecard.Id)
		lwutil.CheckSsdbError(resp, err)

		//update num
		resp, err = ssdbc.Do("qsize", qkey)
		lwutil.CheckSsdbError(resp, err)
		num, err := strconv.Atoi(resp[1])
		lwutil.CheckError(err, "")
		cardType.Num = num
		js, err = json.Marshal(cardType)
		lwutil.CheckError(err, "")
		resp, err = ssdbc.Do("hset", H_ECARD_TYPE, ecard.TypeKey, js)
		lwutil.CheckSsdbError(resp, err)
	}

	//out
	lwutil.WriteResponse(w, in)
}

func apiBuyEcard(w http.ResponseWriter, r *http.Request) {
	if _ecardStoreClose {
		lwutil.SendError("err_store_close", "暂停兑换，请稍候再来")
	}

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
	resp, err := ssdbc.Do("hget", H_ECARD_TYPE, in.TypeKey)
	lwutil.CheckSsdbError(resp, err)
	if len(resp) < 2 {
		lwutil.SendError("err_not_exist", "key not exist")
	}
	var cardType ECardType
	err = json.Unmarshal([]byte(resp[1]), &cardType)
	lwutil.CheckError(err, "")

	if cardType.Num == 0 {
		sendErrorNoLog(w, "err_zero", "item count = 0")
		return
	}

	//check player prize
	playerKey := makePlayerInfoKey(session.Userid)
	playerPrize := getPrize(ssdbc, playerKey)
	if playerPrize < cardType.NeedPrize {
		lwutil.SendError("err_not_enough", "not enough prize")
	}

	//buy, pop from ecard queue
	ecardQueueKey := makeEcardQueueKey(in.TypeKey)
	resp, err = ssdbc.Do("qpop_front", ecardQueueKey)
	lwutil.CheckSsdbError(resp, err)
	itemId, err := strconv.Atoi(resp[1])
	lwutil.CheckError(err, "")

	//buy, add to player ecard zset
	playerEcardZKey := makePlayerEcardZsetKey(session.Userid)
	score := lwutil.GetRedisTimeUnix()
	resp, err = ssdbc.Do("zset", playerEcardZKey, itemId, score)
	lwutil.CheckSsdbError(resp, err)

	//buy, sub player prize num
	addPrize(ssdbc, playerKey, -cardType.NeedPrize)

	//eco record
	err = addEcoRecord(ssdbc, session.Userid, cardType.NeedPrize, ECO_FORWHAT_BUYECARD)
	lwutil.CheckError(err, "")

	//update ecard type's ecard num
	resp, err = ssdbc.Do("qsize", ecardQueueKey)
	lwutil.CheckSsdbError(resp, err)
	num, err := strconv.Atoi(resp[1])
	lwutil.CheckError(err, "")
	cardType.Num = num
	js, err := json.Marshal(cardType)
	lwutil.CheckError(err, "")
	resp, err = ssdbc.Do("hset", H_ECARD_TYPE, in.TypeKey, js)
	lwutil.CheckSsdbError(resp, err)

	//add to purchased list
	resp, err = ssdbc.Do("zset", Z_ECARD_PURCHASED, itemId, score)
	lwutil.CheckSsdbError(resp, err)

	//get ecard
	resp, err = ssdbc.Do("hget", H_ECARD, itemId)
	lwutil.CheckSsdbError(resp, err)

	var ecard ECard
	err = json.Unmarshal([]byte(resp[1]), &ecard)
	lwutil.CheckError(err, "")

	//update ecard
	now := lwutil.GetRedisTime()
	ecard.UserGetDate = now.Format("2006-01-02 15:04:05")
	ecard.OwnerId = session.Userid

	js, err = json.Marshal(ecard)
	lwutil.CheckError(err, "")
	resp, err = ssdbc.Do("hset", H_ECARD, itemId, js)
	lwutil.CheckSsdbError(resp, err)

	//out
	var outEcard OutEcard
	outEcard.ECard = ecard
	outEcard.Provider = ECARD_PROVIDERS[cardType.Provider]

	out := map[string]interface{}{
		"Ecard":       outEcard,
		"PlayerPrize": playerPrize - cardType.NeedPrize,
	}
	lwutil.WriteResponse(w, out)
}

func apiSetEcardStoreClose(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//
	checkAdmin(session)

	//in
	var in struct {
		Close bool
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	_ecardStoreClose = in.Close
	resp, err := ssdbc.Do("set", KEY_ECARD_STORE_CLOSE, _ecardStoreClose)
	lwutil.CheckSsdbError(resp, err)

	//out
	lwutil.WriteResponse(w, in)
}

// func apiGetLastEcard(w http.ResponseWriter, r *http.Request) {
// 	var err error
// 	lwutil.CheckMathod(r, "POST")

// 	//ssdb
// 	ssdbc, err := ssdbPool.Get()
// 	lwutil.CheckError(err, "")
// 	defer ssdbc.Close()

// 	//session
// 	session, err := findSession(w, r, nil)
// 	lwutil.CheckError(err, "err_auth")

// 	//
// 	checkAdmin(session)

// 	//in
// 	var in struct {
// 		Provider string
// 		Price    int
// 	}
// 	err = lwutil.DecodeRequestBody(r, &in)
// 	lwutil.CheckError(err, "err_decode_body")

// 	typeKey := makeEcardTypeKey(in.Provider, in.Price)

// 	//get
// 	qkey := makeEcardQueueKey(typeKey)
// 	resp, err := ssdbc.Do("qback", qkey)
// 	lwutil.CheckSsdbError(resp, err)
// 	ecardId, err := strconv.ParseInt(resp[1], 10, 64)
// 	lwutil.CheckError(err, "")

// 	resp, err = ssdbc.Do("hget", H_ECARD, ecardId)
// 	lwutil.CheckSsdbError(resp, err)

// 	var ecard ECard
// 	err = json.Unmarshal([]byte(resp[1]), &ecard)
// 	lwutil.CheckError(err, "")

// 	//out
// 	out := struct {
// 		Code string
// 	}{
// 		ecard.Code,
// 	}

// 	lwutil.WriteResponse(w, out)
// }

func regStore() {
	http.Handle("/store/listGameCoinPack", lwutil.ReqHandler(apiListGameCoinPack))
	http.Handle("/store/listIapProductId", lwutil.ReqHandler(apiListIapProductId))
	http.Handle("/store/getIapSecret", lwutil.ReqHandler(apiGetIapSecret))
	http.Handle("/store/buyIap", lwutil.ReqHandler(apiBuyIap))

	http.Handle("/store/addEcardType", lwutil.ReqHandler(apiAddEcardType))
	http.Handle("/store/editEcardType", lwutil.ReqHandler(apiEditEcardType))
	http.Handle("/store/delEcardType", lwutil.ReqHandler(apiDelEcardType))
	http.Handle("/store/listEcardType", lwutil.ReqHandler(apiListEcardType))

	http.Handle("/store/addEcard", lwutil.ReqHandler(apiAddEcard))
	http.Handle("/store/addEcards", lwutil.ReqHandler(apiAddEcards))
	http.Handle("/store/buyEcard", lwutil.ReqHandler(apiBuyEcard))

	http.Handle("/store/setEcardStoreClose", lwutil.ReqHandler(apiSetEcardStoreClose))
	// http.Handle("/store/getLastEcard", lwutil.ReqHandler(apiGetLastEcard))
}
