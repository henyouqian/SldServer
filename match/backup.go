package main

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/golang/glog"
	qiniuio "github.com/qiniu/api/io"
	qiniurs "github.com/qiniu/api/rs"
)

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
