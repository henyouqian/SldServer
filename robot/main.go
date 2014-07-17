package main

import (
	"fmt"
	"github.com/golang/glog"
	// "github.com/henyouqian/lwutil"
	"bytes"
	"encoding/json"
	"flag"
	"io/ioutil"
	"math/rand"
	"net/http"
	"runtime"
	// "sync"
	"crypto/sha1"
	"encoding/hex"
	"time"
)

const (
	PASSWORD       = "aaa"
	HOST           = "http://localhost:9999"
	ADMIN_NAME     = "henyouqian@gmail.com"
	ADMIN_PASSWORD = "Nmmgb808313"
)

var (
	TEAM_NAMES = []string{"安徽", "澳门", "北京", "重庆", "福建", "甘肃", "广东", "广西", "贵州", "海南", "河北", "黑龙江", "河南", "湖北", "湖南", "江苏", "江西", "吉林", "辽宁", "内蒙古", "宁夏", "青海", "陕西", "山东", "上海", "山西", "四川", "台湾", "天津", "香港", "新疆", "西藏", "云南", "浙江"}
)

func checkErr(err error) {
	if err != nil {
		glog.Fatalln(err)
	}
}

func register(userName string) {
	url := HOST + "/auth/register"

	body := struct {
		Username string
		Password string
	}{
		userName,
		PASSWORD,
	}
	bodyjs, err := json.Marshal(body)
	checkErr(err)
	resp, err := http.Post(url, "application/json", bytes.NewReader(bodyjs))
	checkErr(err)
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		glog.Errorf("register error: username=%s", userName)
		return
	}
	_, err = ioutil.ReadAll(resp.Body)
	checkErr(err)
	glog.Infof("register ok: %s", userName)
}

func login(userName string, password string) *http.Cookie {
	url := HOST + "/auth/login"
	loginBody := struct {
		Username string
		Password string
	}{
		userName,
		password,
	}
	loginBodyJs, err := json.Marshal(loginBody)
	checkErr(err)
	resp, err := http.Post(url, "application/json", bytes.NewReader(loginBodyJs))
	checkErr(err)
	defer resp.Body.Close()
	cookies := resp.Cookies()

	return cookies[0]
}

func post(userName string, url string, body interface{}) (resp *http.Response) {
	cookie := login(userName, PASSWORD)

	bodyjs, err := json.Marshal(body)
	checkErr(err)

	client := &http.Client{}
	req, err := http.NewRequest("POST", url, bytes.NewReader(bodyjs))
	checkErr(err)
	req.AddCookie(cookie)

	resp, err = client.Do(req)
	checkErr(err)
	return
}

func postWithCookie(cookie *http.Cookie, url string, body interface{}) (resp *http.Response) {
	client := &http.Client{}

	bodyjs, err := json.Marshal(body)
	checkErr(err)

	req, err := http.NewRequest("POST", url, bytes.NewReader(bodyjs))
	checkErr(err)
	req.AddCookie(cookie)

	resp, err = client.Do(req)
	checkErr(err)
	return resp
}

func freePlay(userName string, matchId uint32, minScore int32, maxScore int32) {
	//free play
	url := HOST + "/match/freePlay"
	score := minScore + int32(rand.Int())%(maxScore-minScore)
	body := struct {
		MatchId uint32
		Score   int32
	}{
		matchId,
		score,
	}

	_resp := post(userName, url, body)
	defer _resp.Body.Close()

	resp, err := ioutil.ReadAll(_resp.Body)
	checkErr(err)
	glog.Info(string(resp))
}

func play(userName string, eventId uint64, minScore int32, maxScore int32) {
	//playBegin
	url := HOST + "/event/playBegin"
	body := struct {
		EventId uint64
	}{
		eventId,
	}

	_resp := post(userName, url, body)
	defer _resp.Body.Close()

	resp, err := ioutil.ReadAll(_resp.Body)
	checkErr(err)

	if _resp.StatusCode != 200 {
		glog.Fatalf("http code=%d, body=%s", _resp.StatusCode, string(resp))
	}

	secret := struct {
		Secret string
	}{}
	err = json.Unmarshal(resp, &secret)
	checkErr(err)

	//
	score := minScore + int32(rand.Int())%(maxScore-minScore)

	//checksum
	checksum := fmt.Sprintf("%s+%d9d7a", secret.Secret, score+8703)
	hasher := sha1.New()
	hasher.Write([]byte(checksum))
	checksum = hex.EncodeToString(hasher.Sum(nil))

	//playEnd
	url = HOST + "/event/playEnd"
	playEndBody := struct {
		EventId  uint64
		Secret   string
		Score    int32
		CheckSum string
	}{
		eventId,
		secret.Secret,
		score,
		checksum,
	}

	_resp = post(userName, url, playEndBody)
	defer _resp.Body.Close()
	resp, err = ioutil.ReadAll(_resp.Body)
	checkErr(err)
	glog.Info(string(resp))
}

func setInfo(userName string) {
	url := HOST + "/player/setInfo"
	teamName := TEAM_NAMES[rand.Int()%len(TEAM_NAMES)]
	gender := rand.Int() % 2
	gravatarKey := fmt.Sprintf("%d", rand.Int()%10000)

	body := struct {
		NickName    string
		TeamName    string
		Gender      int
		GravatarKey string
	}{
		userName,
		teamName,
		gender,
		gravatarKey,
	}

	_resp := post(userName, url, body)
	defer _resp.Body.Close()

	resp, err := ioutil.ReadAll(_resp.Body)
	checkErr(err)
	glog.Info(string(resp))
}

func bet(userName string, eventId int, teamName string, money int) {
	url := HOST + "/event/bet"

	body := struct {
		EventId  int
		TeamName string
		Money    int
	}{
		eventId,
		teamName,
		money,
	}

	_resp := post(userName, url, body)
	defer _resp.Body.Close()

	resp, err := ioutil.ReadAll(_resp.Body)
	checkErr(err)
	glog.Info(string(resp))
}

func addMoney(adminCookie *http.Cookie, userName string, addMoney int) {
	url := HOST + "/admin/addMoney"

	body := struct {
		UserName string
		AddMoney int
	}{
		userName,
		addMoney,
	}

	_resp := postWithCookie(adminCookie, url, body)
	defer _resp.Body.Close()

	resp, err := ioutil.ReadAll(_resp.Body)
	checkErr(err)
	glog.Info(string(resp))
}

func main() {
	flag.Parse()
	runtime.GOMAXPROCS(runtime.NumCPU())

	glog.Infof("Robot running: cpu=%d", runtime.NumCPU())

	rand.Seed(time.Now().UnixNano())
	//adminCookie := login(ADMIN_NAME, ADMIN_PASSWORD)
	for i := 0; i < 1000; i++ {
		username := fmt.Sprintf("test%d@pt.com", i)
		// register(username)
		// setInfo(username)

		//play(userName string, eventId uint64, minScore int32, maxScore int32)
		//play(username, 25, -1000*50, -1000*30)

		//addMoney(adminCookie, username, 10000)

		eventId := 25
		teamName := TEAM_NAMES[rand.Int()%len(TEAM_NAMES)]
		money := 20 + rand.Int()%200
		bet(username, eventId, teamName, money)
	}

	// var w sync.WaitGroup
	// w.Add(1)
	// w.Wait()
}
