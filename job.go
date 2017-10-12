package clock

import (
	"github.com/HuKeping/rbtree"
	"log"
	"runtime/debug"
	"sync/atomic"
	"time"
)

// Job External access interface for timed tasks
type Job interface {
	C() <-chan Job     //C Get a Chan，which can get message if Job is executed
	Count() uint64     //计数器，表示已执行（或触发）的次数
	Max() uint64       //允许执行的最大次数
	Cancel()           //撤销加载的任务，不再定时执行
	isAvailable() bool //return true，if job not action or not cancel
}

// jobItem implementation of  "Job" interface and "rbtree.Item" interface
type jobItem struct {
	id           uint64        //唯一键值，内部由管理器生成，以区分同一时刻的不同任务事件
	actionCount  uint64        //计数器，表示已执行（或触发）的次数
	actionMax    uint64        //允许执行的最大次数
	intervalTime time.Duration //间隔时间
	createTime   time.Time     //创建时间，略有误差
	actionTime   time.Time     //计算得出的最近一次执行时间点
	fn           func()        //事件函数
	msgChan      chan Job      //消息通道，执行时，控制器通过该通道向外部传递消息
	cancelFlag   int32
	clock        *Clock
}

// Less Based rbtree ，implements Item interface for sort
func (je jobItem) Less(another rbtree.Item) bool {
	item, ok := another.(*jobItem)
	if !ok {
		return false
	}
	if !je.actionTime.Equal(item.actionTime) {
		return je.actionTime.Before(item.actionTime)
	}
	return je.id < item.id
}

func (je *jobItem) C() <-chan Job {
	return je.msgChan
}

func (je *jobItem) action(async bool) {
	je.actionCount++
	if async {
		go safeCall(je.fn)
	} else {
		safeCall(je.fn)
	}
	select {
	case je.msgChan <- je:
	default:
		//some times,client should not receive msgChan,so must discard jobItem when blocking
	}
}

func (je *jobItem) Cancel() {
	if atomic.CompareAndSwapInt32(&je.cancelFlag, 0, 1) {
		je.clock.rmJob(je)
		je.innerCancel()
	}
}
func (je *jobItem) isAvailable() bool {
	return je.cancelFlag == 0
}
func (je *jobItem) innerCancel() {
	je.clock = nil
	close(je.msgChan)
}

// Count implement for Job
func (je jobItem) Count() uint64 {
	return je.actionCount
}

// Max implement for Job
func (je jobItem) Max() uint64 {
	return je.actionMax
}
func safeCall(fn func()) {
	defer func() {
		if err := recover(); err != nil {
			log.Printf("[clock] recovering reason is %+v. More detail:", err)
			log.Println(string(debug.Stack()))
		}
	}()
	fn()
}
