package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	// "github.com/garyburd/redigo/redis"
	"./ssdb"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/mail"
	"net/smtp"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	qiniuio "github.com/qiniu/api/io"
	qiniurs "github.com/qiniu/api/rs"

	"github.com/golang/glog"
	"github.com/henyouqian/lwutil"
)

const (
	PASSWORD_SALT      = "liwei"
	SESSION_LIFE_SEC   = 60 * 60 * 24 * 7
	SESSION_UPDATE_SEC = 60 * 60
	H_ACCOUNT          = "H_ACCOUNT"        //key:userId, value:accountJson
	H_NAME_ACCONT      = "H_NAME_ACCONT"    //key:userName, value:userId
	H_SNS_ACCONT       = "H_SNS_ACCONT"     //key:snsKey, value:userId
	H_TMP_ACCOUNT      = "H_TMP_ACCOUNT"    //key:uuid, value:userId
	H_WEIBO_ACCOUNT    = "H_WEIBO_ACCOUNT"  //key:uid, value:userId
	H_SESSION          = "H_SESSION"        //key:token, value:session
	H_USER_TOKEN       = "H_USER_TOKEN"     //key:appid/userid, value:token
	K_RESET_PASSWORD   = "K_RESET_PASSWORD" //key:K_RESET_PASSWORD/<resetKey> value:accountEmail
	RESET_PASSWORD_TTL = 60 * 60
	NOTIFICATION       = ""
	ACCOUNT_SERIAL     = "account"
)

var (
	ADMIN_SET = map[string]bool{
		"henyouqian@gmail.com":                 true,
		"B421E075-A1D3-BED7-3D55-CD0613E14915": true,
	}
	TEAM_NAMES = []string{"安徽", "澳门", "北京", "重庆", "福建", "甘肃", "广东", "广西", "贵州", "海南", "河北", "黑龙江", "河南", "湖北", "湖南", "江苏", "江西", "吉林", "辽宁", "内蒙古", "宁夏", "青海", "陕西", "山东", "上海", "山西", "四川", "台湾", "天津", "香港", "新疆", "西藏", "云南", "浙江"}
)

type Session struct {
	Userid   int64
	Username string
	Born     time.Time
	Appid    int
}

type Account struct {
	Username     string
	Password     string
	RegisterTime string
}

func init() {
	glog.Infoln("auth init")
}

func newSession(w http.ResponseWriter, userid int64, username string, appid int, ssdb *ssdb.Client) (usertoken string) {
	var err error
	if ssdb == nil {
		ssdb, err = ssdbAuthPool.Get()
		lwutil.CheckError(err, "")
		defer ssdb.Close()
	}

	tokenKey := fmt.Sprintf("%s/%d/%d", H_USER_TOKEN, appid, userid)
	resp, err := ssdb.Do("get", tokenKey)
	if resp[0] == "ok" {
		sessionKey := fmt.Sprintf("%s/%s", H_SESSION, resp[1])
		ssdb.Do("del", tokenKey)
		ssdb.Do("del", sessionKey)
	}

	usertoken = lwutil.GenUUID()
	sessionKey := fmt.Sprintf("%s/%s", H_SESSION, usertoken)

	session := Session{userid, username, time.Now(), appid}
	js, err := json.Marshal(session)
	lwutil.CheckError(err, "")

	resp, err = ssdb.Do("setx", sessionKey, js, SESSION_LIFE_SEC)
	lwutil.CheckSsdbError(resp, err)
	resp, err = ssdb.Do("setx", tokenKey, usertoken, SESSION_LIFE_SEC)
	lwutil.CheckSsdbError(resp, err)

	// cookie
	http.SetCookie(w, &http.Cookie{Name: "usertoken", Value: usertoken, MaxAge: SESSION_LIFE_SEC, Path: "/"})

	return usertoken
}

func checkAdmin(session *Session) {
	if !ADMIN_SET[session.Username] {
		glog.Info(ADMIN_SET)
		glog.Info(session)
		lwutil.SendError("err_denied", "")
	}
}

func isAdmin(username string) bool {
	return ADMIN_SET[username]
}

func findSession(w http.ResponseWriter, r *http.Request, ssdb *ssdb.Client) (*Session, error) {
	var err error
	if ssdb == nil {
		ssdb, err = ssdbAuthPool.Get()
		lwutil.CheckError(err, "")
		defer ssdb.Close()
	}

	usertoken := ""
	usertokenCookie, err := r.Cookie("usertoken")
	if err != nil {
		usertoken = r.URL.Query().Get("usertoken")
	} else {
		usertoken = usertokenCookie.Value
	}
	if usertoken == "" {
		return nil, fmt.Errorf("no usertoken")
	}

	sessionKey := fmt.Sprintf("%s/%s", H_SESSION, usertoken)
	resp, err := ssdb.Do("get", sessionKey)
	if err != nil {
		return nil, lwutil.NewErr(err)
	}
	if resp[0] != "ok" {
		return nil, lwutil.NewErrStr(resp[0])
	}

	var session Session
	err = json.Unmarshal([]byte(resp[1]), &session)
	lwutil.CheckError(err, "")

	//update session
	dt := time.Now().Sub(session.Born)
	if dt > SESSION_UPDATE_SEC*time.Second {
		newSession(w, session.Userid, session.Username, session.Appid, ssdb)
	}

	return &session, nil
}

func apiAuthRegister(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdb, err := ssdbAuthPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	// in
	var in Account
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if in.Username == "" || in.Password == "" {
		lwutil.SendError("err_input", "")
	}

	in.Password = lwutil.Sha224(in.Password + PASSWORD_SALT)
	in.RegisterTime = time.Now().Format(time.RFC3339)

	//check exist
	rName, err := ssdb.Do("hexists", H_NAME_ACCONT, in.Username)
	lwutil.CheckError(err, "")
	if rName[1] == "1" {
		lwutil.SendError("err_exist", "account already exists")
	}

	//add account
	id := GenSerial(ssdb, ACCOUNT_SERIAL)
	js, err := json.Marshal(in)
	lwutil.CheckError(err, "")
	_, err = ssdb.Do("hset", H_ACCOUNT, id, js)
	lwutil.CheckError(err, "")

	_, err = ssdb.Do("hset", H_NAME_ACCONT, in.Username, id)
	lwutil.CheckError(err, "")

	// reply
	reply := struct {
		Userid int64
	}{id}
	lwutil.WriteResponse(w, reply)
}

func apiAuthLogin(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdb, err := ssdbAuthPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	// logout if already login
	// session, err := findSession(w, r, nil)
	// if err == nil {
	// 	usertokenCookie, err := r.Cookie("usertoken")
	// 	if err == nil {
	// 		usertoken := usertokenCookie.Value
	// 		rc.Send("del", fmt.Sprintf("sessions/%s", usertoken))
	// 		rc.Send("del", fmt.Sprintf("usertokens/%d+%d", session.Userid, session.Appid))
	// 		err = rc.Flush()
	// 		lwutil.CheckError(err, "")
	// 	}
	// }

	// input
	var in struct {
		Username  string
		Password  string
		Appsecret string
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if in.Username == "" || in.Password == "" {
		lwutil.SendError("err_input", "")
	}

	pwsha := lwutil.Sha224(in.Password + PASSWORD_SALT)

	// get userid
	resp, err := ssdb.Do("hget", H_NAME_ACCONT, in.Username)
	lwutil.CheckError(err, "")
	if resp[0] != "ok" {
		lwutil.SendError("err_not_match", "name and password not match")
	}
	userId, err := strconv.ParseInt(resp[1], 10, 64)
	lwutil.CheckError(err, "")

	resp, err = ssdb.Do("hget", H_ACCOUNT, userId)
	lwutil.CheckError(err, "")
	if resp[0] != "ok" {
		lwutil.SendError("err_internal", "account not exist")
	}
	var account Account
	err = json.Unmarshal([]byte(resp[1]), &account)
	lwutil.CheckError(err, "")

	if account.Password != pwsha {
		lwutil.SendError("err_not_match", "name and password not match")
	}

	// get appid
	appid := 0
	// if in.Appsecret != "" {
	// 	row = authDB.QueryRow("SELECT id FROM apps WHERE secret=?", in.Appsecret)
	// 	err = row.Scan(&appid)
	// 	lwutil.CheckError(err, "err_app_secret")
	// }

	// new session
	usertoken := newSession(w, userId, in.Username, appid, ssdb)

	// out
	out := struct {
		Token  string
		Now    int64
		UserId int64
	}{
		usertoken,
		lwutil.GetRedisTimeUnix(),
		userId,
	}
	lwutil.WriteResponse(w, out)
}

func apiAuthGetSnsSecret(w http.ResponseWriter, r *http.Request) {
	//ssdb
	ssdbc, err := ssdbAuthPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	k1 := lwutil.GenUUID()
	key := fmt.Sprintf("snsSecret/%s", k1)

	resp, err := ssdbc.Do("setx", key, 1, 60)
	lwutil.CheckSsdbError(resp, err)

	//out
	out := struct {
		Secret string
	}{
		k1,
	}
	lwutil.WriteResponse(w, out)
}

func apiAuthLoginSns(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdbc, err := ssdbAuthPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//in
	var in struct {
		Type     string
		SnsKey   string
		Secret   string
		Checksum string
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//check type
	snsTypes := map[string]bool{
		"weibo":  true,
		"qzone":  true,
		"douban": true,
		"uuid":   true,
	}

	if snsTypes[in.Type] == false {
		lwutil.SendError("err_type", "")
	}

	//check secret
	secret := fmt.Sprintf("snsSecret/%s", in.Secret)
	resp, err := ssdbc.Do("exists", secret)
	if resp[1] != "1" {
		lwutil.SendError("err_exist", "secret not exists")
	}

	//checksum
	checksum := fmt.Sprintf("%s+%sll46i", in.SnsKey, in.Secret)
	hasher := sha1.New()
	hasher.Write([]byte(checksum))
	checksum = hex.EncodeToString(hasher.Sum(nil))
	if in.Checksum != checksum {
		lwutil.SendError("err_checksum", "")
	}

	//get userId
	key := fmt.Sprintf("%s/%s", in.Type, in.SnsKey)
	resp, err = ssdbc.Do("hget", H_SNS_ACCONT, key)
	var userId int64
	if resp[0] == ssdb.NOT_FOUND {
		userId = GenSerial(ssdbc, ACCOUNT_SERIAL)

		resp, err = ssdbc.Do("hset", H_SNS_ACCONT, key, userId)
		lwutil.CheckSsdbError(resp, err)

		//set account
		var account Account
		account.RegisterTime = time.Now().Format(time.RFC3339)
		js, err := json.Marshal(account)
		lwutil.CheckError(err, "")
		_, err = ssdbc.Do("hset", H_ACCOUNT, userId, js)
		lwutil.CheckError(err, "")

		//set player
		playerKey := makePlayerInfoKey(userId)

		matchDb, err := ssdbPool.Get()
		lwutil.CheckError(err, "")
		defer matchDb.Close()

		addPlayerGoldCoin(matchDb, playerKey, 20)
	} else {
		lwutil.CheckSsdbError(resp, err)
		userId, err = strconv.ParseInt(resp[1], 10, 64)
	}

	userToken := newSession(w, userId, key, 0, ssdbc)

	// out
	out := struct {
		Token    string
		Now      int64
		UserId   int64
		UserName string
	}{
		userToken,
		lwutil.GetRedisTimeUnix(),
		userId,
		key,
	}
	lwutil.WriteResponse(w, out)
}

func apiAuthLogout(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdb, err := ssdbAuthPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//find user token
	usertokenCookie, err := r.Cookie("usertoken")
	lwutil.CheckError(err, "err_already_logout")
	usertoken := usertokenCookie.Value

	//get session
	sessionKey := fmt.Sprintf("%s/%s", H_SESSION, usertoken)
	resp, err := ssdb.Do("get", sessionKey)
	if err != nil || resp[0] != "ok" {
		lwutil.SendError("err_already_logout", "")
	}

	var session Session
	err = json.Unmarshal([]byte(resp[1]), &session)
	lwutil.CheckError(err, "")

	//del
	tokenKey := fmt.Sprintf("%s/%d/%d", H_USER_TOKEN, session.Appid, session.Userid)
	ssdb.Do("del", tokenKey)
	ssdb.Do("del", sessionKey)

	// reply
	lwutil.WriteResponse(w, "logout")
}

func apiAuthLoginInfo(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")

	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//
	usertokenCookie, err := r.Cookie("usertoken")
	usertoken := usertokenCookie.Value

	//
	reply := struct {
		Session   *Session
		UserToken string
	}{session, usertoken}

	lwutil.WriteResponse(w, reply)
}

// func authNewApp(w http.ResponseWriter, r *http.Request) {
// 	lwutil.CheckMathod(r, "POST")

// 	session, err := findSession(w, r, nil)
// 	lwutil.CheckError(err, "err_auth")
// 	checkAdmin(session)

// 	// input
// 	var input struct {
// 		Name string
// 	}
// 	err = lwutil.DecodeRequestBody(r, &input)
// 	lwutil.CheckError(err, "err_decode_body")

// 	if input.Name == "" {
// 		lwutil.SendError("err_input", "input.Name empty")
// 	}

// 	// db
// 	stmt, err := authDB.Prepare("INSERT INTO apps (name, secret) VALUES (?, ?)")
// 	lwutil.CheckError(err, "")

// 	secret := lwutil.GenUUID()
// 	_, err = stmt.Exec(input.Name, secret)
// 	lwutil.CheckError(err, "err_name_exists")

// 	// reply
// 	reply := struct {
// 		Name   string
// 		Secret string
// 	}{input.Name, secret}
// 	lwutil.WriteResponse(w, reply)
// }

// func authListApp(w http.ResponseWriter, r *http.Request) {
// 	lwutil.CheckMathod(r, "POST")

// 	session, err := findSession(w, r, nil)
// 	lwutil.CheckError(err, "err_auth")
// 	checkAdmin(session)

// 	// db
// 	rows, err := authDB.Query("SELECT name, secret FROM apps")
// 	lwutil.CheckError(err, "")

// 	type App struct {
// 		Name   string
// 		Secret string
// 	}

// 	apps := make([]App, 0, 16)
// 	var app App
// 	for rows.Next() {
// 		err = rows.Scan(&app.Name, &app.Secret)
// 		lwutil.CheckError(err, "")
// 		apps = append(apps, app)
// 	}

// 	lwutil.WriteResponse(w, apps)
// }

func apiForgotPassword(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//in
	var in struct {
		UserName string
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//ssdb
	ssdb, err := ssdbAuthPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	matchDb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer matchDb.Close()

	//check account exist
	resp, err := ssdb.Do("hget", H_NAME_ACCONT, in.UserName)
	lwutil.CheckError(err, "")
	if resp[0] != "ok" {
		lwutil.SendError("err_not_exist", "account not exist")
	}
	userId, err := strconv.ParseInt(resp[1], 10, 64)
	lwutil.CheckError(err, "")

	player, err := getPlayerInfo(matchDb, userId)
	lwutil.CheckError(err, fmt.Sprintf("Player info not exist, userId:%d", userId))
	toEmail := player.Email
	if len(toEmail) == 0 {
		lwutil.SendError("err_no_email", "")
	}

	//gen reset key
	resetKey := lwutil.GenUUID()
	key := fmt.Sprintf("K_RESET_PASSWORD/%s", resetKey)
	resp, err = ssdb.Do("setx", key, in.UserName, RESET_PASSWORD_TTL)
	lwutil.CheckSsdbError(resp, err)

	//
	body := fmt.Sprintf("请进入以下网址重设《蛮拼的》密码. \nhttp://sld.pintugame.com/www/resetpassword.html?key=%s", resetKey)

	//email
	b64 := base64.NewEncoding("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/")

	// host := "smtp.qq.com"
	// email := "103638667@qq.com"
	// password := "nmmgbnmmgb"
	host := "localhost"
	fromEmail := "resetpassword@pintugame.com"
	password := "Nmmgb808313"

	from := mail.Address{"蛮拼的", fromEmail}
	to := mail.Address{"亲爱的《蛮拼的》用户", toEmail}

	header := make(map[string]string)
	header["From"] = from.String()
	header["To"] = to.String()
	header["Subject"] = fmt.Sprintf("=?UTF-8?B?%s?=", b64.EncodeToString([]byte("《蛮拼的》密码重设")))
	header["MIME-Version"] = "1.0"
	header["Content-Type"] = "text/html; charset=UTF-8"
	header["Content-Transfer-Encoding"] = "base64"

	message := ""
	for k, v := range header {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	message += "\r\n" + b64.EncodeToString([]byte(body))

	auth := smtp.PlainAuth(
		"",
		fromEmail,
		password,
		host,
	)

	err = smtp.SendMail(
		host+":25",
		auth,
		fromEmail,
		[]string{to.Address},
		[]byte(message),
	)
	lwutil.CheckError(err, "")

	lwutil.WriteResponse(w, "ok")
}

func apiCheckVersion(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//in
	var in struct {
		Version string
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//out
	url := ""
	if in.Version < CLIENT_VERSION {
		url = APP_STORE_URL
	}
	out := struct {
		UpdateUrl    string
		Notification string
	}{
		url,
		NOTIFICATION,
	}
	lwutil.WriteResponse(w, out)
}

func apiResetPassword(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//in
	var in struct {
		ResetKey string
		Password string
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//ssdb
	ssdb, err := ssdbAuthPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//
	key := fmt.Sprintf("K_RESET_PASSWORD/%s", in.ResetKey)
	resp, err := ssdb.Do("get", key)
	if resp[0] == "not_found" {
		lwutil.SendError("err_key", "reset not found")
	}
	lwutil.CheckSsdbError(resp, err)
	username := resp[1]

	//password
	newPassword := lwutil.Sha224(in.Password + PASSWORD_SALT)

	//get account
	resp, err = ssdb.Do("hget", H_NAME_ACCONT, username)
	lwutil.CheckError(err, "")
	if resp[0] != "ok" {
		lwutil.SendError("err_not_match", "name and password not match")
	}
	userId, err := strconv.ParseInt(resp[1], 10, 64)
	lwutil.CheckError(err, "")

	resp, err = ssdb.Do("hget", H_ACCOUNT, userId)
	lwutil.CheckError(err, "")
	if resp[0] != "ok" {
		lwutil.SendError("err_internal", "account not exist")
	}
	var account Account
	err = json.Unmarshal([]byte(resp[1]), &account)
	lwutil.CheckError(err, "")

	//save
	account.Password = newPassword
	js, err := json.Marshal(account)
	lwutil.CheckError(err, "")
	resp, err = ssdb.Do("hset", H_ACCOUNT, userId, js)
	lwutil.CheckSsdbError(resp, err)

	//delete reset key
	resp, err = ssdb.Do("del", key)
	lwutil.CheckSsdbError(resp, err)

	lwutil.WriteResponse(w, "ok")
}

func apiSsdbTest(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")

	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	var in string
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	strs := strings.Split(in, ",")

	intfs := make([]interface{}, len(strs))
	for i, v := range strs {
		intfs[i] = interface{}(v)
	}
	res, err := ssdb.Do(intfs...)
	// lwutil.CheckError(err, "")
	lwutil.CheckSsdbError(res, err)

	lwutil.WriteResponse(w, res)
}

func apiAuthRegisterTmp(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")

	//ssdb
	authDb, err := ssdbAuthPool.Get()
	lwutil.CheckError(err, "")
	defer authDb.Close()

	matchDb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer matchDb.Close()

	//set tmp account
	uuid := lwutil.GenUUID()
	userId := GenSerial(authDb, ACCOUNT_SERIAL)

	resp, err := authDb.Do("hset", H_TMP_ACCOUNT, uuid, userId)
	lwutil.CheckSsdbError(resp, err)

	playerIn := struct {
		NickName        string
		GravatarKey     string
		CustomAvatarKey string
		TeamName        string
		Email           string
		Gender          int
	}{
		fmt.Sprintf("u%d", userId),
		fmt.Sprintf("%d", rand.Intn(99999)),
		"",
		TEAM_NAMES[rand.Intn(len(TEAM_NAMES))],
		"",
		rand.Intn(1),
	}
	playerOut := savePlayerInfo(matchDb, userId, playerIn)

	userToken := newSession(w, userId, "", 0, authDb)

	doFollow(matchDb, userId, 128)
	doFollow(matchDb, userId, 136)
	doFollow(matchDb, userId, 138)

	// out
	out := struct {
		Token  string
		Now    int64
		UserId int64
		UUID   string
		Player *PlayerInfo
	}{
		userToken,
		lwutil.GetRedisTimeUnix(),
		userId,
		uuid,
		playerOut,
	}
	lwutil.WriteResponse(w, out)
}

func apiAuthLoginTmp(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")

	//ssdb
	authDb, err := ssdbAuthPool.Get()
	lwutil.CheckError(err, "")
	defer authDb.Close()

	matchDb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer matchDb.Close()

	//in
	var in struct {
		UUID string
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	resp, err := authDb.Do("hget", H_TMP_ACCOUNT, in.UUID)
	lwutil.CheckSsdbError(resp, err)

	userId, err := strconv.ParseInt(resp[1], 10, 64)
	lwutil.CheckError(err, "err_strconv")

	userToken := newSession(w, userId, "", 0, authDb)

	playerInfo, err := getPlayerInfo(matchDb, userId)
	lwutil.CheckError(err, "err_getplayerinfo")

	// out
	out := struct {
		Token  string
		Now    int64
		UserId int64
		Player *PlayerInfo
	}{
		userToken,
		lwutil.GetRedisTimeUnix(),
		userId,
		playerInfo,
	}
	lwutil.WriteResponse(w, out)
}

const (
	WEIBO_APP_ID       = 2485478034
	WEIBO_APP_SECRET   = "d52bd51bd1b43d6562a5fb19e94883ff"
	WEIBO_REDIRECT_URI = "http://g.pintugame.com/oauth.html"
)

func apiWeiboBind(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")

	authDb, err := ssdbAuthPool.Get()
	lwutil.CheckError(err, "")
	defer authDb.Close()

	//session
	session, err := findSession(w, r, authDb)
	lwutil.CheckError(err, "err_auth")

	//in
	var in struct {
		Code string
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//
	url := fmt.Sprintf("https://api.weibo.com/oauth2/access_token?client_id=%d&client_secret=%s&grant_type=authorization_code&code=%s&redirect_uri=%s", WEIBO_APP_ID, WEIBO_APP_SECRET, in.Code, WEIBO_REDIRECT_URI)
	res, err := http.PostForm(url, nil)
	if err != nil {
		return
	}
	defer res.Body.Close()
	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return
	}

	//
	authData := struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
		Uid         string `json:"uid"`
	}{}
	err = json.Unmarshal(data, &authData)

	//check binded already
	resp, err := authDb.Do("hget", H_WEIBO_ACCOUNT, authData.Uid)
	lwutil.CheckError(err, "err_ssdb")
	if resp[0] == SSDB_OK {
		lwutil.SendError("err_weibo_account_using", "")
	}

	//bind
	resp, err = authDb.Do("hset", H_WEIBO_ACCOUNT, authData.Uid, session.Userid)
	lwutil.CheckSsdbError(resp, err)

	//playerInfo
	matchDb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer matchDb.Close()

	playerInfo, err := getPlayerInfo(matchDb, session.Userid)
	lwutil.CheckError(err, "err_getplayerinfo")

	////fill weibo user info
	//check name change
	nameChanged := false
	avatarChanged := false

	if playerInfo.NickName != fmt.Sprintf("u%d", session.Userid) {
		nameChanged = true
	}
	if playerInfo.CustomAvatarKey != "" {
		avatarChanged = true
	}

	if !nameChanged || !avatarChanged {
		url = fmt.Sprintf("https://api.weibo.com/2/users/show.json?access_token=%s&uid=%s", authData.AccessToken, authData.Uid)
		res, err = http.Get(url)
		if err != nil {
			return
		}
		defer res.Body.Close()
		data, err = ioutil.ReadAll(res.Body)
		if err != nil {
			return
		}

		udata := struct {
			Name   string `json:"name"`
			Avatar string `json:"avatar_large"`
			Gender string `json:"gender"`
		}{}
		err = json.Unmarshal(data, &udata)

		if !nameChanged && udata.Name != "" {
			playerInfo.NickName = udata.Name
		}

		//upload avatar to qiniu
		if udata.Avatar != "" && !avatarChanged {
			url = udata.Avatar
			res, err = http.Get(url)
			lwutil.CheckError(err, "err_http_get")

			defer res.Body.Close()
			data, err = ioutil.ReadAll(res.Body)
			lwutil.CheckError(err, "err_ioutil")

			ext := filepath.Ext(url)
			imgkey := genImageKey(data, ext)

			//checkexists
			rsCli := qiniurs.New(nil)

			_, err = rsCli.Stat(nil, USER_UPLOAD_BUCKET, imgkey)
			if err != nil {
				//upload
				putPolicy := qiniurs.PutPolicy{
					Scope: USER_UPLOAD_BUCKET,
				}
				token := putPolicy.Token(nil)

				var putRet qiniuio.PutRet
				err = qiniuio.Put(nil, &putRet, token, imgkey, bytes.NewReader(data), nil)
				lwutil.CheckError(err, "err_qiniu_put")
			}

			playerInfo.CustomAvatarKey = imgkey
		}
		savePlayerInfo(matchDb, session.Userid, *playerInfo)
	}

	//out
	out := struct {
		UserId int64
		Player *PlayerInfo
	}{
		session.Userid,
		playerInfo,
	}

	//out
	lwutil.WriteResponse(w, out)
}

func apiWeiboLogin(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")

	authDb, err := ssdbAuthPool.Get()
	lwutil.CheckError(err, "")
	defer authDb.Close()
	matchDb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer matchDb.Close()

	//in
	var in struct {
		Code string
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//
	url := fmt.Sprintf("https://api.weibo.com/oauth2/access_token?client_id=%d&client_secret=%s&grant_type=authorization_code&code=%s&redirect_uri=%s", WEIBO_APP_ID, WEIBO_APP_SECRET, in.Code, WEIBO_REDIRECT_URI)
	res, err := http.PostForm(url, nil)
	if err != nil {
		return
	}
	defer res.Body.Close()
	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return
	}

	authData := struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
		Uid         string `json:"uid"`
	}{}
	err = json.Unmarshal(data, &authData)

	if authData.Uid == "" {
		lwutil.SendError("err_code", string(data))
	}

	//check binded
	userId := int64(0)
	var playerInfo *PlayerInfo

	resp, err := authDb.Do("hget", H_WEIBO_ACCOUNT, authData.Uid)
	lwutil.CheckError(err, "err_ssdb")

	//register or login
	if resp[0] == SSDB_NOT_FOUND {
		//set tmp account
		userId = GenSerial(authDb, ACCOUNT_SERIAL)

		resp, err := authDb.Do("hset", H_WEIBO_ACCOUNT, authData.Uid, userId)
		lwutil.CheckSsdbError(resp, err)

		playerIn := struct {
			NickName        string
			GravatarKey     string
			CustomAvatarKey string
			TeamName        string
			Email           string
			Gender          int
		}{
			fmt.Sprintf("u%d", userId),
			fmt.Sprintf("%d", rand.Intn(99999)),
			"",
			TEAM_NAMES[rand.Intn(len(TEAM_NAMES))],
			"",
			rand.Intn(1),
		}
		playerInfo = savePlayerInfo(matchDb, userId, playerIn)

		//fill weibo user info
		url = fmt.Sprintf("https://api.weibo.com/2/users/show.json?access_token=%s&uid=%s", authData.AccessToken, authData.Uid)
		res, err = http.Get(url)
		if err != nil {
			return
		}
		defer res.Body.Close()
		data, err = ioutil.ReadAll(res.Body)
		if err != nil {
			return
		}

		udata := struct {
			Name   string `json:"name"`
			Avatar string `json:"avatar_large"`
			Gender string `json:"gender"`
		}{}
		err = json.Unmarshal(data, &udata)

		playerInfo.NickName = udata.Name

		//upload avatar to qiniu
		if udata.Avatar != "" {
			url = udata.Avatar
			res, err = http.Get(url)
			lwutil.CheckError(err, "err_http_get")

			defer res.Body.Close()
			data, err = ioutil.ReadAll(res.Body)
			lwutil.CheckError(err, "err_ioutil")

			ext := filepath.Ext(url)
			imgkey := genImageKey(data, ext)

			//checkexists
			rsCli := qiniurs.New(nil)

			_, err = rsCli.Stat(nil, USER_UPLOAD_BUCKET, imgkey)
			if err != nil {
				//upload
				putPolicy := qiniurs.PutPolicy{
					Scope: USER_UPLOAD_BUCKET,
				}
				token := putPolicy.Token(nil)

				var putRet qiniuio.PutRet
				err = qiniuio.Put(nil, &putRet, token, imgkey, bytes.NewReader(data), nil)
				lwutil.CheckError(err, "err_qiniu_put")
			}

			playerInfo.CustomAvatarKey = imgkey
		}
		savePlayerInfo(matchDb, userId, *playerInfo)

	} else {
		userId, err = strconv.ParseInt(resp[1], 10, 64)
		lwutil.CheckError(err, "err_strconv")

		playerInfo, err = getPlayerInfo(matchDb, userId)
		lwutil.CheckError(err, "err_getplayerinfo")
	}

	//session
	userToken := newSession(w, userId, "", 0, authDb)

	// out
	out := struct {
		Token  string
		Now    int64
		UserId int64
		Player *PlayerInfo
	}{
		userToken,
		lwutil.GetRedisTimeUnix(),
		userId,
		playerInfo,
	}
	lwutil.WriteResponse(w, out)
}

func regAuth() {
	http.Handle("/auth/login", lwutil.ReqHandler(apiAuthLogin))
	http.Handle("/auth/getSnsSecret", lwutil.ReqHandler(apiAuthGetSnsSecret))
	http.Handle("/auth/loginSns", lwutil.ReqHandler(apiAuthLoginSns))
	http.Handle("/auth/logout", lwutil.ReqHandler(apiAuthLogout))
	http.Handle("/auth/register", lwutil.ReqHandler(apiAuthRegister))
	http.Handle("/auth/info", lwutil.ReqHandler(apiAuthLoginInfo))
	http.Handle("/auth/forgotPassword", lwutil.ReqHandler(apiForgotPassword))
	http.Handle("/auth/resetPassword", lwutil.ReqHandler(apiResetPassword))
	http.Handle("/auth/checkVersion", lwutil.ReqHandler(apiCheckVersion))
	// http.Handle("/auth/ssdbTest", lwutil.ReqHandler(apiSsdbTest))

	http.Handle("/auth/registerTmp", lwutil.ReqHandler(apiAuthRegisterTmp))
	http.Handle("/auth/loginTmp", lwutil.ReqHandler(apiAuthLoginTmp))
	http.Handle("/auth/weiboBind", lwutil.ReqHandler(apiWeiboBind))
	http.Handle("/auth/weiboLogin", lwutil.ReqHandler(apiWeiboLogin))
}
