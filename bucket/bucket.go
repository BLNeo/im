package bucket

import (
	"errors"
	"fmt"
	"github.com/mongofs/im/ack"
	"github.com/mongofs/im/client"
	"github.com/mongofs/im/log"
	"go.uber.org/atomic"
	"net/http"
	"sync"
)


var (
	ErrUserExist =errors.New("im/bucket : Cannot login repeatedly")
	ErrCliISNil  =errors.New("im/bucket : cli is nil")
)

type bucket struct {
	rw sync.RWMutex

	// Number of people
	np *atomic.Int64

	// users set
	clis map[string]client.Clienter

	// User offline notification
	closeSig chan string

	// Ack map
	ack ack.Acker

	// log
	log log.Logger

	opts * Option
}

func New(log log.Logger,option *Option) Bucketer {
	res := & bucket{
		rw:       sync.RWMutex{},
		np:       &atomic.Int64{},
		closeSig: make(chan string,0),
		opts: option,
		log :log,
	}
	res.clis = make(map[string]client.Clienter,res.opts.BucketSize)
	res.start()
	return res
}


func (h *bucket)Flush(){
	h.rw.RLock()
	defer h.rw.RUnlock()
	h.np.Store(int64(len(h.clis)))
}


func(h *bucket)CreateConn(w http.ResponseWriter,r * http.Request,token string,handler client.Receiver)(client.Clienter,error){
	return  client.CreateConn(w , r ,
				h.closeSig,
				h.opts.ClientBufferSize,
				h.opts.MessageType,
				h.opts.Protocol,
				h.opts.ReaderBufferSize,
				h.opts.WriteBufferSize,
				token ,
				h.opts.ctx,
				handler,
				h.log)
}

func (h *bucket)randId()int64{
	return 0
}

func (h *bucket) Onlines()int64 {
	return h.np.Load()
}



func (h *bucket) send (cli client.Clienter,token string,data []byte,ack bool)error{
	if ack {
		sid := h.randId()
		if err := h.ack.AddMessage(token,sid,data);err !=nil{
			return err
		}
		return  cli.Send(data,sid)
	}else{
		return cli.Send(data)
	}
	return nil
}

func (h *bucket) Send(data []byte, token string, Ack bool) error{
	h.rw.RLock()
	cli ,ok:= h.clis[token];
	h.rw.RUnlock()
	if !ok{
		return ErrCliISNil
	}else {
		return h.send(cli,token,data,Ack)
	}
}

func (h *bucket) BroadCast(data []byte, Ack bool) error{
	counter := 0
	success :=0

	failedTokens := ""
	h.rw.RLock()
	for token,cli := range h.clis{
		err := h.send(cli,token,data,Ack)
		if err !=nil {
			//log.Infof("im/bucket: %v",err)
			counter ++
			failedTokens = failedTokens +"." +token
		}else {
			success ++
		}
	}
	h.rw.RUnlock()
	if counter !=0 {return fmt.Errorf("im/bucket :  bucket 广播成功数量 %v ，广播失败数量 is %v,具体tokens :%s", success,counter,failedTokens)}
	return nil
}

func (h *bucket) OffLine(token string) {
	h.rw.RLock()
	cli,ok := h.clis[token]
	h.rw.RUnlock()
	if ok {
		cli.Offline()
	}
}

// 将用户注册到bucket中
func (h *bucket) Register(cli client.Clienter,token string) error {
	if cli == nil  {
		return ErrCliISNil
	}
	h.rw.Lock()
	defer h.rw.Unlock()
	old,ok := h.clis[token];
	if ok {
		h.log.Warnf("im/bucket : User token %s is online, but is trying to connect again",token)
		clienter ,_:= old.(*client.Cli)
		clienter.OfflineForRetry(true)
	}
	h.clis[token] = cli
	h.np.Add(1)
	return nil
}

func (h *bucket) IsOnline(token string) bool {
	h.rw.RLock()
	defer h.rw.RUnlock()
	if _,ok:= h.clis[token];ok {
		return true
	}
	return false
}


func (h *bucket)NotifyBucketConnectionIsClosed()chan <- string{
	return h.closeSig
}




