package main

import (
	"./ssdb"
	"database/sql"
	"fmt"
	"github.com/garyburd/redigo/redis"
	// _ "github.com/go-sql-driver/mysql"
	"github.com/henyouqian/lwutil"
	"strconv"
	"time"
)

const (
	H_SERIAL = "hSerial"
)

var (
	redisPool *redis.Pool
	// authRedisPool *redis.Pool
	// authDB       *sql.DB
	ssdbPool     *ssdb.Pool
	ssdbAuthPool *ssdb.Pool
)

func init() {
	redisPool = &redis.Pool{
		MaxIdle:     20,
		MaxActive:   0,
		IdleTimeout: 240 * time.Second,
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", "localhost:6379")
			if err != nil {
				return nil, err
			}
			return c, err
		},
	}
	// authRedisPool = &redis.Pool{
	// 	MaxIdle:     10,
	// 	MaxActive:   0,
	// 	IdleTimeout: 240 * time.Second,
	// 	Dial: func() (redis.Conn, error) {
	// 		c, err := redis.Dial("tcp", "localhost:6379")
	// 		if err != nil {
	// 			return nil, err
	// 		}
	// 		c.Do("SELECT", 10)
	// 		return c, err
	// 	},
	// }

	// authDB = opendb("auth_db")
	// authDB.SetMaxIdleConns(10)

	ssdbPool = ssdb.NewPool("localhost", 9876, 10, 60)
	ssdbAuthPool = ssdb.NewPool("localhost", 9875, 10, 60)
}

func opendb(dbname string) *sql.DB {
	db, err := sql.Open("mysql", fmt.Sprintf("root@/%s?parseTime=true", dbname))
	if err != nil {
		panic(err)
	}
	return db
}

func GenSerial(ssdb *ssdb.Client, key string) int64 {
	resp, err := ssdb.Do("hincr", "hSerial", key, 1)
	lwutil.CheckSsdbError(resp, err)

	out, err := strconv.ParseInt(resp[1], 10, 64)
	lwutil.CheckError(err, "")
	return out
}
