package ssdb

import (
	"bytes"
	"container/list"
	"fmt"
	"github.com/golang/glog"
	"net"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	OK        = "ok"
	NOT_FOUND = "not_found"
)

type Pool struct {
	Ip      string
	Port    int
	MaxIdle uint32
	TimeOut time.Duration

	mu          sync.Mutex
	idleClients list.List
}

func _ssdbGlog() {
	glog.Info("ssdb")
}

func NewPool(ip string, port int, maxIdel uint32, timeOutSec uint32) *Pool {
	pool := Pool{
		Ip:      ip,
		Port:    port,
		MaxIdle: maxIdel,
		TimeOut: time.Duration(timeOutSec) * time.Second,
	}
	pool.idleClients.Init()
	return &pool
}

func (self *Pool) Get() (*Client, error) {
	self.mu.Lock()
	for self.idleClients.Len() > 0 {
		e := self.idleClients.Front()
		if e != nil {
			self.idleClients.Remove(e)
			c := e.Value.(*Client)
			if time.Now().Before(c.timeOut) {
				self.mu.Unlock()
				return e.Value.(*Client), nil
			} else {
				c.sock.Close()
			}
		} else {
			break
		}
	}

	self.mu.Unlock()

	//create new
	client, err := Connect(self.Ip, self.Port, self)

	if err != nil {
		return nil, err
	}

	return client, nil
}

func (self *Pool) Put(client *Client) {
	self.mu.Lock()
	client.timeOut = time.Now().Add(self.TimeOut)
	self.idleClients.PushBack(client)
	if self.idleClients.Len() > int(self.MaxIdle) {
		e := self.idleClients.Front()
		e.Value.(*Client).sock.Close()
		self.idleClients.Remove(e)
	}
	self.mu.Unlock()
}

type Client struct {
	sock     *net.TCPConn
	recv_buf bytes.Buffer
	pool     *Pool
	err      error
	timeOut  time.Time
}

func (self *Client) Close() error {
	if self.err != nil {
		return self.sock.Close()
	} else {
		self.pool.Put(self)
		return nil
	}
}

func Connect(ip string, port int, pool *Pool) (*Client, error) {
	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", ip, port))
	if err != nil {
		return nil, err
	}
	sock, err := net.DialTCP("tcp", nil, addr)
	if err != nil {
		return nil, err
	}
	var c Client
	c.sock = sock
	c.pool = pool
	return &c, nil
}

func (c *Client) Do(args ...interface{}) (_ []string, rErr error) {
	defer func() {
		if rErr != nil {
			c.err = rErr
		}
	}()

	err := c.send(args)
	if err != nil {
		return nil, err
	}
	resp, err := c.recv()
	return resp, err
}

func (c *Client) Batch(args [][]interface{}) (_ [][]string, rErr error) {
	defer func() {
		if rErr != nil {
			c.err = rErr
		}
	}()

	err := c.batchSend(args)
	if err != nil {
		return nil, err
	}
	num := len(args)

	resps, err := c.batchRecv(num)
	return resps, err

	// for i := 0; i < num; i++ {
	// 	resp, err := c.recv()
	// 	if err == nil && resp[0] != OK {
	// 		err = fmt.Errorf("ssdb_not_ok")
	// 	}
	// 	ssdbResp := SsdbResp{
	// 		resp,
	// 		err,
	// 	}
	// 	resps = append(resps, ssdbResp)
	// 	if err != nil && rErr == nil {
	// 		rErr = err
	// 	}
	// }

	// return resps, rErr
}

func (c *Client) Set(key string, val string) (_ interface{}, rErr error) {
	defer func() {
		if rErr != nil {
			c.err = rErr
		}
	}()

	resp, err := c.Do("set", key, val)
	if err != nil {
		return nil, err
	}
	if resp[0] == "ok" {
		return true, nil
	}
	return nil, fmt.Errorf(resp[0])
}

// TODO: Will somebody write addition semantic methods?
func (c *Client) Get(key string) (_ interface{}, rErr error) {
	defer func() {
		if rErr != nil {
			c.err = rErr
		}
	}()

	resp, err := c.Do("get", key)
	if err != nil {
		return nil, err
	}
	if len(resp) == 2 && resp[0] == "ok" {
		return resp[1], nil
	}
	if resp[0] == "not_found" {
		return nil, nil
	}
	return nil, fmt.Errorf("bad response")
}

func (c *Client) Del(key string) (_ interface{}, rErr error) {
	defer func() {
		if rErr != nil {
			c.err = rErr
		}
	}()

	resp, err := c.Do("del", key)
	if err != nil {
		return nil, err
	}
	if len(resp) == 1 && resp[0] == "ok" {
		return true, nil
	}
	return nil, fmt.Errorf("bad response")
}

func (c *Client) send(args []interface{}) (rErr error) {
	defer func() {
		if rErr != nil {
			c.err = rErr
		}
	}()

	var buf bytes.Buffer
	for _, arg := range args {
		var s string
		switch arg := arg.(type) {
		case string:
			s = arg
		case []byte:
			s = string(arg)
		case int, int8, int16, int32, int64:
			s = fmt.Sprintf("%d", arg)
		case uint, uint8, uint16, uint32, uint64:
			s = fmt.Sprintf("%d", arg)
		case float32, float64:
			s = fmt.Sprintf("%f", arg)
		case bool:
			if arg {
				s = "1"
			} else {
				s = "0"
			}
		case nil:
			s = ""
		default:
			return fmt.Errorf("bad arguments")
		}
		buf.WriteString(fmt.Sprintf("%d", len(s)))
		buf.WriteByte('\n')
		buf.WriteString(s)
		buf.WriteByte('\n')
	}
	buf.WriteByte('\n')
	_, err := c.sock.Write(buf.Bytes())
	return err
}

func (c *Client) batchSend(argss [][]interface{}) (rErr error) {
	defer func() {
		if rErr != nil {
			c.err = rErr
		}
	}()

	var buf bytes.Buffer
	for _, args := range argss {
		for _, arg := range args {
			var s string
			switch arg := arg.(type) {
			case string:
				s = arg
			case []byte:
				s = string(arg)
			case int, int8, int16, int32, int64:
				s = fmt.Sprintf("%d", arg)
			case uint, uint8, uint16, uint32, uint64:
				s = fmt.Sprintf("%d", arg)
			case float32, float64:
				s = fmt.Sprintf("%f", arg)
			case bool:
				if arg {
					s = "1"
				} else {
					s = "0"
				}
			case nil:
				s = ""
			default:
				return fmt.Errorf("bad arguments")
			}
			buf.WriteString(fmt.Sprintf("%d", len(s)))
			buf.WriteByte('\n')
			buf.WriteString(s)
			buf.WriteByte('\n')
		}
		buf.WriteByte('\n')
	}

	_, err := c.sock.Write(buf.Bytes())
	return err
}

func (c *Client) recv() (_ []string, rErr error) {
	defer func() {
		if rErr != nil {
			c.err = rErr
		}
	}()

	var tmp [8192]byte
	for {
		n, err := c.sock.Read(tmp[0:])
		if err != nil {
			return nil, err
		}
		c.recv_buf.Write(tmp[0:n])
		resp := c.parse()
		if resp == nil || len(resp) > 0 {
			return resp, nil
		}
	}
}

func (c *Client) parse() []string {
	resp := []string{}
	buf := c.recv_buf.Bytes()
	var idx, offset int
	idx = 0
	offset = 0

	for {
		idx = bytes.IndexByte(buf[offset:], '\n')
		if idx == -1 {
			break
		}
		p := buf[offset : offset+idx]
		offset += idx + 1
		//fmt.Printf("> [%s]\n", p);
		if len(p) == 0 || (len(p) == 1 && p[0] == '\r') {
			if len(resp) == 0 {
				continue
			} else {
				c.recv_buf.Next(offset)
				return resp
			}
		}

		size, err := strconv.Atoi(string(p))
		if err != nil || size < 0 {
			return nil
		}
		if offset+size >= c.recv_buf.Len() {
			break
		}

		v := buf[offset : offset+size]
		resp = append(resp, string(v))
		offset += size + 1
	}

	return []string{}
}

func (c *Client) batchRecv(num int) (_ [][]string, rErr error) {
	defer func() {
		if rErr != nil {
			c.err = rErr
		}
	}()

	resps := make([][]string, 0, num)

	var tmp [8192]byte
	for {
		n, rErr := c.sock.Read(tmp[0:])
		if rErr != nil {
			break
		}
		c.recv_buf.Write(tmp[0:n])
		for true {
			resp := c.parse()
			if resp == nil || len(resp) > 0 {
				resps = append(resps, resp)
			} else {
				break
			}
		}

		if len(resps) == num {
			break
		}
	}
	return resps, rErr
}

func (c *Client) HSet(key string, subKey string, obj interface{}) (rErr error) {
	defer func() {
		if rErr != nil {
			c.err = rErr
		}
	}()

	_, err := c.Do("hset", key, subKey, obj)
	return err
}

func (c *Client) HGet(key string, subKey string, obj interface{}) (rErr error) {
	defer func() {
		if rErr != nil {
			c.err = rErr
		}
	}()

	resp, err := c.Do("hget", key, subKey)
	if err != nil {
		return err
	}

	if resp[0] != "ok" {
		return fmt.Errorf("ssdb error:%s", resp[0])
	}

	//
	val := reflect.ValueOf(obj)
	if val.Kind() != reflect.Ptr {
		return fmt.Errorf("need Ptr")
	}
	val = val.Elem()
	str := resp[1]

	//
	switch val.Interface().(type) {
	case string:
		val.SetString(str)
	case []byte:
		val.SetBytes([]byte(str))
	case int, int8, int16, int32, int64:
		intv, _ := strconv.ParseInt(str, 0, 64)
		val.SetInt(intv)
	case uint, uint8, uint16, uint32, uint64:
		intv, _ := strconv.ParseUint(str, 0, 64)
		val.SetUint(intv)
	case float32, float64:
		intv, _ := strconv.ParseFloat(str, 64)
		val.SetFloat(intv)
	case bool:
		b := true
		if str == "0" || str == "false" {
			b = false
		}
		val.SetBool(b)
	case nil:
		val.SetPointer(nil)
	default:
		return fmt.Errorf("bad arguments")
	}

	return nil
}

func (c *Client) HSetStruct(key string, obj interface{}) (rErr error) {
	defer func() {
		if rErr != nil {
			c.err = rErr
		}
	}()

	objVal := reflect.ValueOf(obj)

	objType := objVal.Type()
	if objVal.Kind() != reflect.Struct {
		return fmt.Errorf("need struct")
	}

	numField := objType.NumField()
	cmds := make([]interface{}, 2, 2+numField*2)
	cmds[0] = "multi_hset"
	cmds[1] = key
	for i := 0; i < numField; i++ {
		field := objType.Field(i)
		val := objVal.Field(i)
		cmds = append(cmds, field.Name)
		cmds = append(cmds, val.Interface())
	}

	resp, err := c.Do(cmds...)

	if err != nil {
		return err
	}
	if len(resp) > 0 && resp[0] == "ok" {
		return nil
	}

	return fmt.Errorf("bad response")
}

func (c *Client) HGetStruct(key string, objPtr interface{}) (rErr error) {
	defer func() {
		if rErr != nil {
			c.err = rErr
		}
	}()

	objVal := reflect.ValueOf(objPtr)

	objVal = objVal.Elem()
	if objVal.Kind() != reflect.Struct {
		return fmt.Errorf("need struct pointer")
	}

	objType := objVal.Type()

	numField := objType.NumField()
	cmds := make([]interface{}, 2, 2+numField)
	cmds[0] = "multi_hget"
	cmds[1] = key
	for i := 0; i < numField; i++ {
		field := objType.Field(i)
		cmds = append(cmds, field.Name)
	}

	resp, err := c.Do(cmds...)

	if err != nil {
		return err
	}
	if resp[0] != "ok" {
		return fmt.Errorf("%s", resp[0])
	}
	if len(resp) == 1 {
		return fmt.Errorf("not_found")
	}

	resp = resp[1:]
	numResp := len(resp)
	for i := 0; i < numResp/2; i++ {
		k := resp[i*2]
		v := resp[i*2+1]
		fieldValue := objVal.FieldByName(k)
		if !fieldValue.IsValid() {
			continue
		}
		switch fieldValue.Interface().(type) {
		case string:
			fieldValue.SetString(v)
		case []byte:
			fieldValue.SetBytes([]byte(v))
		case int, int8, int16, int32, int64:
			intv, _ := strconv.ParseInt(v, 0, 64)
			fieldValue.SetInt(intv)
		case uint, uint8, uint16, uint32, uint64:
			intv, _ := strconv.ParseUint(v, 0, 64)
			fieldValue.SetUint(intv)
		case float32, float64:
			intv, _ := strconv.ParseFloat(v, 64)
			fieldValue.SetFloat(intv)
		case bool:
			b := true
			if v == "0" || v == "false" {
				b = false
			}
			fieldValue.SetBool(b)
		default:
			return fmt.Errorf("bad arguments")
		}
	}

	return nil
}

func (c *Client) HSetMap(key string, mp map[string]interface{}) (rErr error) {
	defer func() {
		if rErr != nil {
			c.err = rErr
		}
	}()

	num := len(mp)
	cmds := make([]interface{}, 0, 2+num*2)
	cmds = append(cmds, "multi_hset")
	cmds = append(cmds, key)

	for k, v := range mp {
		cmds = append(cmds, k)
		cmds = append(cmds, v)
	}

	resp, err := c.Do(cmds...)

	if err != nil {
		return err
	}
	if resp[0] != "ok" {
		return fmt.Errorf("ssdbErr:", resp[0])
	}
	return nil
}

func (c *Client) HGetMap(key string, mp map[string]interface{}) (rErr error) {
	defer func() {
		if rErr != nil {
			c.err = rErr
		}
	}()

	num := len(mp)
	cmds := make([]interface{}, 0, 2+num)
	cmds = append(cmds, "multi_hget")
	cmds = append(cmds, key)

	for k, _ := range mp {
		cmds = append(cmds, k)
	}

	resp, err := c.Do(cmds...)
	if err != nil {
		return err
	}
	if resp[0] != "ok" {
		return fmt.Errorf("ssdbErr:", resp[0])
	}

	resp = resp[1:]
	for i := 0; i < len(resp)/2; i++ {
		k := resp[i*2]
		v := resp[i*2+1]

		switch mp[k].(type) {
		case string:
			mp[k] = v
		case []byte:
			mp[k] = []byte(v)
		case int, int8, int16, int32, int64:
			intv, _ := strconv.ParseInt(v, 0, 64)
			mp[k] = intv
		case uint, uint8, uint16, uint32, uint64:
			intv, _ := strconv.ParseUint(v, 0, 64)
			mp[k] = intv
		case float32, float64:
			intv, _ := strconv.ParseFloat(v, 64)
			mp[k] = intv
		case bool:
			b := true
			if v == "0" || v == "false" {
				b = false
			}
			mp[k] = b
		default:
			return fmt.Errorf("bad arguments")
		}
	}

	return nil
}

func (c *Client) HGetMapAll(key string, mp interface{}) (rErr error) {
	defer func() {
		if rErr != nil {
			c.err = rErr
		}
	}()

	val := reflect.ValueOf(mp)
	if val.Kind() != reflect.Map {
		return fmt.Errorf("second arg must be map")
	}

	tp := val.Type()

	//
	resp, err := c.Do("hgetall", key)
	if err != nil {
		return err
	}
	if resp[0] != "ok" {
		return fmt.Errorf("ssdbErr:", resp[0])
	}

	//
	resp = resp[1:]
	for i := 0; i < len(resp)/2; i++ {
		k := resp[i*2]
		v := resp[i*2+1]

		mapkeyVal := reflect.New(tp.Key()).Elem()
		switch mapkeyVal.Interface().(type) {
		case string:
			mapkeyVal.SetString(k)
		case []byte:
			mapkeyVal.SetBytes([]byte(k))
		case int, int8, int16, int32, int64:
			intv, _ := strconv.ParseInt(k, 0, 64)
			mapkeyVal.SetInt(intv)
		case uint, uint8, uint16, uint32, uint64:
			uiv, _ := strconv.ParseUint(k, 0, 64)
			mapkeyVal.SetUint(uiv)
		case float32, float64:
			f, _ := strconv.ParseFloat(k, 64)
			mapkeyVal.SetFloat(f)
		case bool:
			b := true
			if k == "0" || k == "false" {
				b = false
			}
			mapkeyVal.SetBool(b)
		default:
			return fmt.Errorf("bad arguments")
		}

		switch reflect.New(tp.Elem()).Elem().Interface().(type) {
		case string:
			val.SetMapIndex(mapkeyVal, reflect.ValueOf(v))
		case []byte:
			val.SetMapIndex(mapkeyVal, reflect.ValueOf([]byte(v)))
		case int, int8, int16, int32, int64:
			iv, _ := strconv.ParseInt(v, 0, 64)
			val.SetMapIndex(mapkeyVal, reflect.ValueOf(iv))
		case uint, uint8, uint16, uint32, uint64:
			uv, _ := strconv.ParseUint(v, 0, 64)
			val.SetMapIndex(mapkeyVal, reflect.ValueOf(uv))
		case float32, float64:
			fv, _ := strconv.ParseFloat(v, 64)
			val.SetMapIndex(mapkeyVal, reflect.ValueOf(fv))
		case bool:
			b := true
			if v == "0" || v == "false" {
				b = false
			}
			val.SetMapIndex(mapkeyVal, reflect.ValueOf(b))
		default:
			return fmt.Errorf("bad arguments")
		}
	}

	return nil
}

func (c *Client) TableSetRow(tableName string, key interface{}, obj interface{}) (rErr error) {
	defer func() {
		if rErr != nil {
			c.err = rErr
		}
	}()

	objVal := reflect.ValueOf(obj)

	objType := objVal.Type()
	if objVal.Kind() != reflect.Struct {
		return fmt.Errorf("need struct")
	}

	numField := objType.NumField()
	cmds := make([]interface{}, 2, 2+numField*2)
	cmds[0] = "multi_hset"
	cmds[1] = tableName
	cmds = append(cmds, key, 1)
	for i := 0; i < numField; i++ {
		field := objType.Field(i)
		val := objVal.Field(i)
		subkey := fmt.Sprintf("%v^%v", key, field.Name)
		cmds = append(cmds, subkey, val.Interface())
	}

	resp, err := c.Do(cmds...)

	if err != nil {
		return err
	}
	if len(resp) > 0 && resp[0] == "ok" {
		return nil
	}

	return fmt.Errorf("bad response")
}

func (c *Client) TableSetRowWithMap(tableName string, key interface{}, mp map[string]interface{}) (rErr error) {
	defer func() {
		if rErr != nil {
			c.err = rErr
		}
	}()

	num := len(mp)
	cmds := make([]interface{}, 2, 2+num*2)
	cmds[0] = "multi_hset"
	cmds[1] = tableName
	for k, v := range mp {
		subkey := fmt.Sprintf("%v^%v", key, k)
		cmds = append(cmds, subkey)
		cmds = append(cmds, v)
	}
	resp, err := c.Do(cmds...)

	if err != nil {
		return err
	}
	if len(resp) > 0 && resp[0] == "ok" {
		return nil
	}

	return fmt.Errorf("bad response")
}

func (c *Client) TableDelRow(tableName string, key interface{}, obj interface{}) (rErr error) {
	defer func() {
		if rErr != nil {
			c.err = rErr
		}
	}()

	objVal := reflect.ValueOf(obj)

	objType := objVal.Type()
	if objVal.Kind() != reflect.Struct {
		return fmt.Errorf("need struct")
	}

	numField := objType.NumField()
	cmds := make([]interface{}, 2, 2+numField*2)
	cmds[0] = "multi_hdel"
	cmds[1] = tableName
	cmds = append(cmds, key)
	for i := 0; i < numField; i++ {
		field := objType.Field(i)
		subkey := fmt.Sprintf("%v^%v", key, field.Name)
		cmds = append(cmds, subkey)
	}

	resp, err := c.Do(cmds...)

	if err != nil {
		return err
	}
	if len(resp) > 0 && resp[0] == "ok" {
		return nil
	}

	return fmt.Errorf("bad response")
}

func (c *Client) TableGetRow(tableName string, key string, objPtr interface{}) (rErr error) {
	defer func() {
		if rErr != nil {
			c.err = rErr
		}
	}()

	objVal := reflect.ValueOf(objPtr)

	objVal = objVal.Elem()
	if objVal.Kind() != reflect.Struct {
		return fmt.Errorf("need struct pointer")
	}

	objType := objVal.Type()

	numField := objType.NumField()
	cmds := make([]interface{}, 2, 2+numField)
	cmds[0] = "multi_hget"
	cmds[1] = tableName
	for i := 0; i < numField; i++ {
		field := objType.Field(i)
		subkey := fmt.Sprintf("%v^%v", key, field.Name)
		cmds = append(cmds, subkey)
	}

	resp, err := c.Do(cmds...)

	if err != nil {
		return err
	}
	if resp[0] != "ok" {
		return fmt.Errorf("%s", resp[0])
	}
	if len(resp) == 1 {
		return fmt.Errorf("not_found")
	}

	resp = resp[1:]
	numResp := len(resp)
	for i := 0; i < numResp/2; i++ {
		k := resp[i*2]

		strs := strings.Split(k, "^")
		if len(strs) != 2 {
			return fmt.Errorf("string_split")
		}
		k = strs[1]
		v := resp[i*2+1]
		fieldValue := objVal.FieldByName(k)
		if !fieldValue.IsValid() {
			continue
		}
		switch fieldValue.Interface().(type) {
		case string:
			fieldValue.SetString(v)
		case []byte:
			fieldValue.SetBytes([]byte(v))
		case int, int8, int16, int32, int64:
			intv, _ := strconv.ParseInt(v, 0, 64)
			fieldValue.SetInt(intv)
		case uint, uint8, uint16, uint32, uint64:
			intv, _ := strconv.ParseUint(v, 0, 64)
			fieldValue.SetUint(intv)
		case float32, float64:
			intv, _ := strconv.ParseFloat(v, 64)
			fieldValue.SetFloat(intv)
		case bool:
			b := true
			if v == "0" || v == "false" {
				b = false
			}
			fieldValue.SetBool(b)
		default:
			return fmt.Errorf("bad arguments")
		}
	}

	return nil
}

func (c *Client) TableGetRows(tableName string, keys []string, objs interface{}) (rErr error) {
	defer func() {
		if rErr != nil {
			c.err = rErr
		}
	}()

	objsVal := reflect.ValueOf(objs).Elem()
	if objsVal.Kind() != reflect.Slice {
		return fmt.Errorf("need slice")
	}

	elemType := objsVal.Type().Elem()
	if elemType.Kind() != reflect.Struct {
		return fmt.Errorf("must be slice of struct")
	}

	elems := reflect.MakeSlice(objsVal.Type(), 0, 16)

	numRow := len(keys)
	numField := elemType.NumField()
	cmds := make([]interface{}, 2, 2+numField)
	cmds[0] = "multi_hget"
	cmds[1] = tableName
	for i := 0; i < numRow; i++ {
		for j := 0; j < numField; j++ {
			key := keys[i]
			field := elemType.Field(j)
			subkey := fmt.Sprintf("%v^%v", key, field.Name)
			cmds = append(cmds, subkey)
		}
	}

	resp, err := c.Do(cmds...)

	if err != nil {
		return err
	}
	if resp[0] != "ok" {
		return fmt.Errorf("%s", resp[0])
	}
	if len(resp) == 1 {
		return fmt.Errorf("not_found")
	}

	resp = resp[1:]
	numResp := len(resp)
	lastKey := ""
	var objValue reflect.Value
	hasValue := false
	for i := 0; i < numResp/2; i++ {
		k := resp[i*2]

		strs := strings.Split(k, "^")
		if len(strs) != 2 {
			return fmt.Errorf("string_split")
		}
		key := strs[0]
		if key != lastKey {
			if hasValue {
				elems = reflect.Append(elems, objValue)
			}
			lastKey = key
			objValue = reflect.New(elemType).Elem()
			hasValue = true
		}
		fieldName := strs[1]
		fieldValue := objValue.FieldByName(fieldName)
		if !fieldValue.IsValid() {
			continue
		}

		v := resp[i*2+1]
		switch fieldValue.Interface().(type) {
		case string:
			fieldValue.SetString(v)
		case []byte:
			fieldValue.SetBytes([]byte(v))
		case int, int8, int16, int32, int64:
			intv, _ := strconv.ParseInt(v, 0, 64)
			fieldValue.SetInt(intv)
		case uint, uint8, uint16, uint32, uint64:
			intv, _ := strconv.ParseUint(v, 0, 64)
			fieldValue.SetUint(intv)
		case float32, float64:
			intv, _ := strconv.ParseFloat(v, 64)
			fieldValue.SetFloat(intv)
		case bool:
			b := true
			if v == "0" || v == "false" {
				b = false
			}
			fieldValue.SetBool(b)
		default:
			return fmt.Errorf("bad arguments")
		}
	}
	if hasValue {
		elems = reflect.Append(elems, objValue)
	}

	objsVal.Set(elems)

	return nil
}

func (c *Client) ZScan(zkey, hkey string, keyStart interface{}, scoreStart interface{}, limit int, reverse bool) (result []string, lastKey string, lastScore string, err error) {
	api := "zscan"
	if reverse {
		api = "zrscan"
	}
	resp, err := c.Do(api, zkey, keyStart, scoreStart, "", limit)
	if err != nil {
		return nil, "", "", err
	}
	resp = resp[1:]
	if len(resp) == 0 {
		return nil, "", "", nil
	}

	num := len(resp) / 2
	cmds := make([]interface{}, num+2)
	cmds[0] = "multi_hget"
	cmds[1] = hkey
	for i := 0; i < num; i++ {
		cmds[i+2] = resp[i*2]
	}

	lastKey = resp[(num-1)*2]
	lastScore = resp[(num-1)*2+1]

	//
	resp, err = c.Do(cmds...)
	if err != nil {
		return nil, "", "", err
	}
	resp = resp[1:]
	return resp, lastKey, lastScore, nil
}

func (c *Client) QDel(qkey string, value string, limit int, fromBack bool) error {
	if limit < 0 {
		return fmt.Errorf("err_limit")
	}
	if limit > 100 {
		limit = 100
	}

	if fromBack {
		resp, err := c.Do("qsize", qkey)
		if err != nil {
			return err
		}
		if resp[0] != OK {
			return fmt.Errorf("err_ssdb:%s", resp[0])
		}
		size, err := strconv.Atoi(resp[1])
		if err != nil {
			return err
		}
		if limit > size {
			limit = size
		}
	}

	index := 0
	if fromBack {
		index = -limit
	}
	resp, err := c.Do("qrange", qkey, index, limit)
	if err != nil {
		return err
	}
	if resp[0] != OK {
		return fmt.Errorf("err_ssdb:%s", resp[0])
	}

	resp = resp[1:]

	pushValues := make([]interface{}, 0, len(resp))
	find := false
	trimNum := 0
	if fromBack {
		for _, v := range resp {
			if !find {
				if v == value {
					find = true
				}
			}
			if find {
				trimNum++
				if v != value {
					pushValues = append(pushValues, v)
				}
			}
		}
	} else {
		l := len(resp)
		for i := range resp {
			v := resp[l-i-1]
			if !find {
				if v == value {
					find = true
				}
			}
			if find {
				trimNum++
				if v != value {
					pushValues = append(pushValues, v)
				}
			}
		}
	}

	if find {
		trimApi := "qtrim_front"
		pushApi := "qpush_front"
		if fromBack {
			trimApi = "qtrim_back"
			pushApi = "qpush_back"
		}
		_, err := c.Do(trimApi, qkey, trimNum)
		if err != nil {
			return err
		}

		if len(pushValues) > 0 {
			cmds := make([]interface{}, 2, len(pushValues)+2)
			cmds[0] = pushApi
			cmds[1] = qkey
			cmds = append(cmds, pushValues...)

			_, err = c.Do(cmds...)
			if err != nil {
				return err
			}
		}
	}

	return nil

}
