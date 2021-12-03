package im

import (
	"github.com/gin-gonic/gin"
	"github.com/mongofs/im/bucket"
	"github.com/mongofs/im/recieve"
	"github.com/mongofs/im/validate"
	"go.uber.org/atomic"
	"google.golang.org/grpc"
	"time"
)

type ImSrever struct {
	http     *gin.Engine
	rpc      *grpc.Server
	rpcPort  string
	httpPort string

	validate validate.Validater
	recevier recieve.Receiver
	bs       []bucket.Bucketer
	ps       atomic.Int64
	bsIdx    uint32
	cancel   func()
}




func (s *ImSrever)monitor ()error{
	for{
		n := int64(0)
		for _,bck := range  s.bs{
			bck.Flush()
			n += bck.Onlines()
		}
		s.ps.Store(n)
		time.Sleep(10 *time.Second)
	}
	return nil
}


func (s *ImSrever) bucket(token string) bucket.Bucketer {
	idx := Index(token,s.bsIdx)
	return s.bs[idx]
}




