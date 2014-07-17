package main

import (
	"fmt"
	"github.com/garyburd/redigo/redis"
	"strconv"
)

func redisUint64(reply interface{}, err error) (uint64, error) {
	if err != nil {
		return 0, err
	}
	switch reply := reply.(type) {
	case uint64:
		return reply, nil
	case []byte:
		n, err := strconv.ParseUint(string(reply), 10, 64)
		return uint64(n), err
	case nil:
		return 0, redis.ErrNil
	case redis.Error:
		return 0, reply
	}
	return 0, fmt.Errorf("redigo: unexpected type for Int, got type %T", reply)
}

func redisInt64(reply interface{}, err error) (int64, error) {
	if err != nil {
		return 0, err
	}
	switch reply := reply.(type) {
	case int64:
		return reply, nil
	case []byte:
		n, err := strconv.ParseInt(string(reply), 10, 64)
		return int64(n), err
	case nil:
		return 0, redis.ErrNil
	case redis.Error:
		return 0, reply
	}
	return 0, fmt.Errorf("redigo: unexpected type for Int, got type %T", reply)
}

func redisUint32(reply interface{}, err error) (uint32, error) {
	if err != nil {
		return 0, err
	}
	switch reply := reply.(type) {
	case uint32:
		return reply, nil
	case []byte:
		n, err := strconv.ParseUint(string(reply), 10, 32)
		return uint32(n), err
	case nil:
		return 0, redis.ErrNil
	case redis.Error:
		return 0, reply
	}
	return 0, fmt.Errorf("redigo: unexpected type for Int, got type %T", reply)
}

func redisInt32(reply interface{}, err error) (int32, error) {
	if err != nil {
		return 0, err
	}
	switch reply := reply.(type) {
	case int32:
		return reply, nil
	case []byte:
		n, err := strconv.ParseInt(string(reply), 10, 32)
		return int32(n), err
	case nil:
		return 0, redis.ErrNil
	case redis.Error:
		return 0, reply
	}
	return 0, fmt.Errorf("redigo: unexpected type for Int, got type %T", reply)
}
