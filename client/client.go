package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/gorilla/websocket"
	im "github.com/mongofs/api/im/v1"
	"github.com/mongofs/im/log"
)


const (
	waitTime         = 1 << 7
	ProtocolJson     = 1
	ProtocolProtobuf = 2

	MessageTypeText    = 1
	MessageTypeBinary  = 2

)

type Cli struct {
	lastHeartBeatT int64
	conn            *websocket.Conn
	reader 			*http.Request
	token          string
	closeFunc      sync.Once
	done           chan struct{}
	ctx            context.Context
	buf            chan []byte
	closeSig       chan<- string
	handleReceive  Receiver

	log log.Logger
	protocol    int // json /protobuf
	messageType int // text /binary
}

func (c * Cli)Token()string{
	return c.token
}



func CreateConn(w http.ResponseWriter, r *http.Request,closeSig chan <- string, buffer, messageType, protocol,
						readBuffSize, writeBuffSize int, token string, ctx context.Context,handler Receiver,log log.Logger) (Clienter, error) {
	res := &Cli{
		lastHeartBeatT: time.Now().Unix(),
		done:        make(chan struct{}),
		reader: r,
		closeFunc:   sync.Once{},
		buf:         make(chan []byte, buffer),
		token:       token,
		ctx:         ctx,
		closeSig: closeSig,
		protocol:    protocol,
		messageType: messageType,
		handleReceive: handler,
		log: log,
	}
	if err := res.upgrade(w, r, readBuffSize, writeBuffSize); err != nil {
		return nil, err
	}
	if err := res.start(); err != nil {
		return nil, err
	}
	return res, nil
}

func (c *Cli) upgrade(w http.ResponseWriter, r *http.Request, readerSize, writeSize int) error {
	conn, err := (&websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
		ReadBufferSize:  readerSize,
		WriteBufferSize: writeSize,
	}).Upgrade(w, r, nil)
	if err != nil {
		return err
	}
	c.conn = conn
	return nil
}

func (c *Cli) Send(data []byte, i ...int64) error {
	var (
		sid int64
		d   []byte
		err error
	)
	if len(i) > 0 {
		sid = i[0]
	}
	basic := &im.PushToClient{
		Sid: sid,
		Msg: data,
	}
	if c.protocol == ProtocolJson {
		d, err = json.Marshal(basic)
	} else {
		d, err = proto.Marshal(basic)
	}
	if err != nil {
		return err
	}
	if err := c.send(d) ;err !=nil {return err}
	return nil
}

func (c *Cli) LastHeartBeat() int64 {
	return c.lastHeartBeatT
}

func (c *Cli) send(data []byte) error{
	if len(c.buf) *10 > cap(c.buf) * 7 {
		// 记录当前用户被丢弃的信息
		//c.log.Infof(fmt.Sprintf("im/client: 用户消息通道堵塞 , token is %s ,len %v but user cap is %v",c.token,len(c.buf),cap(c.buf)))

		return errors.New(fmt.Sprintf("im/client: too much data , user len %v but user cap is %s",len(c.buf),cap(c.buf)))
	}

	c.buf <- data
	return nil
}

// param retry ,if retry is ture , don't delete the token
func (c *Cli) Offline() {
	c.close(false)
}


func (c *Cli)OfflineForRetry(retry ...bool){
	c.close(retry...)
}


func (c *Cli) start() error {
	go c.sendProc()
	go c.recvProc()
	return nil
}

func (c *Cli) sendProc() {
	defer func() {
		if err := recover(); err != nil {
			c.log.Error(errors.New(fmt.Sprintf("im/client :	token '%v' 发生panic错误 :'%v'", c.token, err)))
		}
	}()
	for {
		select {
		case data := <-c.buf:
			temtime := time.Now()
			err := c.conn.WriteMessage(c.messageType, data)
			spendTime :=time.Since(temtime)
			if spendTime > time.Duration(2) *time.Second {
				c.log.Infof(fmt.Sprintf("im/client :token '%v'网络状态不好，消息写入通道时间过长 :'%v'", c.token,spendTime))
			}
			if err != nil {
				c.log.Error(errors.New(fmt.Sprintf("im/client :	token '%v' 消息写入通道发生错误 :'%v'", c.token, err)))
				goto loop
			}
		case <-c.done:
			goto loop
		}
	}
loop:
	c.close()
}

// 如果close 是为了重连，就没有
func (c *Cli) close(forRetry ...bool) {
	flag := false
	if len(forRetry)> 0 {
		flag =forRetry[0]
	}

	c.closeFunc.Do(func() {
		close(c.done)
		c.conn.Close()
		if ! flag {
			c.closeSig <- c.token
		}

		//log.Info(fmt.Sprintf("client : %s is offline",c.token))
	})
}

func (c *Cli) recvProc() {
	defer func() {
		if err := recover(); err != nil {
			c.log.Error(errors.New(fmt.Sprintf("im/client :	token '%v' 发生panic错误  :'%v'", c.token, err)))
		}
	}()
	for {
		select {
		case <-c.done:
			goto loop
		default:
			_, data, err := c.conn.ReadMessage()
			if err != nil {
				c.log.Error(errors.New(fmt.Sprintf("im/client :	token '%v' 消息通道读取发生错误 :'%v'", c.token, err)))
				goto loop
			}
			c.handleReceive.Handle(c,data)
		}
	}
loop:
	c.close()
}


func (c *Cli) ResetHeartBeatTime(){
	c.lastHeartBeatT =time.Now().Unix()
}



func (c *Cli)Request()*http.Request{

	return c.reader
}
