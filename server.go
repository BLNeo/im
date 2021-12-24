package im

import (
	im "github.com/mongofs/api/im/v1"
	"github.com/mongofs/im/bucket"
	"go.uber.org/atomic"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"net/http"
	"time"
)

type ImSrever struct {
	http     *http.ServeMux
	rpc      *grpc.Server
	bs       []bucket.Bucketer
	ps       atomic.Int64

	buffer 	 chan *im.BroadcastReq
	cancel   func()

	opt *Option
}



// 统计用户在线人数
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

// 单独处理广播业务
func (s *ImSrever)PushBroadCast()error{

	wg:= errgroup.Group{}

	for i:= 0;i<10 ;i++{
		wg.Go(func() error {
			for {
				req := <- s.buffer
				for _,v :=range s.bs{
					v.BroadCast(req.Data,false)
				}
			}
			return nil
		})
	}
	return nil
}


func (s *ImSrever) bucket(token string) bucket.Bucketer {
	idx := Index(token,uint32(s.opt.ServerBucketNumber))
	return s.bs[idx]
}





