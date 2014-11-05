package main

import (
	"flag"
	"fmt"
	"github.com/golang/glog"
	//"github.com/henyouqian/lwutil"
	qiniuconf "github.com/qiniu/api/conf"
	qiniuio "github.com/qiniu/api/io"
	qiniurs "github.com/qiniu/api/rs"
	"github.com/robfig/cron"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"time"
)

var (
	_cron cron.Cron
)

func init() {
	qiniuconf.ACCESS_KEY = QINIU_KEY
	qiniuconf.SECRET_KEY = QINIU_SEC
}

func staticFile(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, r.URL.Path[1:])
}

func html5(w http.ResponseWriter, r *http.Request) {
	url := fmt.Sprintf("%s%s", "../../delight/html5/slider/", r.URL.Path[7:])
	glog.Info(url)
	// glog.Info(r.URL.Path[1:])
	http.ServeFile(w, r, url)
}

// func rootTextFile(w http.ResponseWriter, r *http.Request) {
// 	http.ServeFile(w, r, "./root/"+r.URL.Path[1:])
// }

func backupTask() {
	defer handleError()

	//mkdir
	os.Mkdir("bak", 0777)

	//dir name
	now := time.Now()
	timeStr := now.Format("2006-01-02T15-04")

	glog.Info("backup begin")

	//ssdb/auth
	glog.Info("backup ssdb/auth")
	authFile := fmt.Sprintf("%s-auth%s", _conf.AppName, timeStr)
	dir := fmt.Sprintf("./bak/%s", authFile)
	cmd := exec.Command("/usr/local/ssdb/ssdb-dump", "127.0.0.1", fmt.Sprintf("%d", _conf.SsdbAuthPort), dir)
	err := cmd.Run()
	checkError(err)

	gz := fmt.Sprintf("%s.tar.gz", dir)
	cmd = exec.Command("tar", "cvzf", gz, dir)
	err = cmd.Run()
	checkError(err)

	cmd = exec.Command("rm", "-fr", dir)
	err = cmd.Run()
	checkError(err)

	authPath := gz

	//ssdb/match
	glog.Info("backup ssdb/match")
	matchFile := fmt.Sprintf("%s-match%s", _conf.AppName, timeStr)
	dir = fmt.Sprintf("./bak/%s", matchFile)
	cmd = exec.Command("/usr/local/ssdb/ssdb-dump", "127.0.0.1", fmt.Sprintf("%d", _conf.SsdbMatchPort), dir)
	err = cmd.Run()
	checkError(err)

	gz = fmt.Sprintf("%s.tar.gz", dir)
	cmd = exec.Command("tar", "cvzf", gz, dir)
	err = cmd.Run()
	checkError(err)

	cmd = exec.Command("rm", "-fr", dir)
	err = cmd.Run()
	checkError(err)

	matchPath := gz

	//redis
	glog.Info("backup redis")
	from := "../redis/dump.rdb"
	// from := "/var/lib/redis/dump.rdb"
	redisFile := fmt.Sprintf("%s-redis%s.rdb", _conf.AppName, timeStr)
	to := fmt.Sprintf("./bak/%s", redisFile)
	cmd = exec.Command("cp", "-f", from, to)
	err = cmd.Run()
	checkError(err)

	redisPath := to

	//upload to qiniu
	upload := func(key string, path string) {
		putPolicy := qiniurs.PutPolicy{
			Scope: BACKUP_BUCKET,
		}
		token := putPolicy.Token(nil)

		//upload
		var ret qiniuio.PutRet
		err = qiniuio.PutFile(nil, &ret, token, key, path, nil)
		checkError(err)
		glog.Infof("upload image ok: path=%s", path)
	}

	upload(authFile, authPath)
	upload(matchFile, matchPath)
	upload(redisFile, redisPath)

	//
	glog.Info("backup end")

}

func main() {
	var confFile string
	flag.StringVar(&confFile, "conf", "", "config file")
	flag.Parse()

	if len(confFile) == 0 {
		glog.Errorln("need -conf")
		return
	}

	initConf(confFile)
	initDb()
	// initEvent()
	// initPickSide()
	initAdmin()
	initMatchCron()
	initStore()

	u, _ := user.Current()

	if u.Username != "liwei" {
		_cron.AddFunc("0 19 3 * * *", backupTask)
	}
	// backupTask()

	_cron.Start()

	http.HandleFunc("/www/", staticFile)
	http.HandleFunc("/html5/", html5)
	// http.HandleFunc("/", rootTextFile)

	regAuth()
	regPack()
	regCollection()
	regPlayer()
	regMatch()
	regAdmin()
	regStore()
	regEtc()
	regSocial()
	regEcoMonitor()
	// regEvent()
	// regChallenge()
	// regUserPack()
	// regPickSide()

	runtime.GOMAXPROCS(runtime.NumCPU())

	// go scoreKeeperMain()

	glog.Infof("Server running: cpu=%d, port=%d", runtime.NumCPU(), _conf.Port)
	glog.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", _conf.Port), nil))
}
