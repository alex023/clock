package clock

import (
	"github.com/HuKeping/rbtree"
	"sync/atomic"
	"time"
	"runtime/debug"
	"log"
)

// Job External access interface for timed tasks
type Job interface {
	C() <-chan Job //C Get a Chan，which can get message if Job is executed
	Count() uint64 //计数器，表示已执行（或触发）的次数
	Times() uint64 //允许执行的最大次数
	Cancel()       //撤销加载的任务，不再定时执行
}

// jobItem implementation of  "Job" interface and "rbtree.Item" interface
type jobItem struct {
	id           uint64        //唯一键值，内部由管理器生成，以区分同一时刻的不同任务事件
	count        uint64        //计数器，表示已执行（或触发）的次数
	actionTimes  uint64        //允许执行的最大次数
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

func (je *jobItem) action(callByGoroutine bool) {
	je.count++
	if je.fn != nil {
		if callByGoroutine {
			go je.call()
		} else {
			je.call()
		}
	}
	select {
	case je.msgChan <- je:
	default:
		//some times,client should not receive msgChan,so must discard jobItem when blocking
	}
}
func (je *jobItem) call() {
	defer func() {
		if err := recover(); err != nil {
			log.Printf("[clock] recovering reason is %+v. More detail:",err)
			log.Println(string(debug.Stack()))
		}
	}()
	je.fn()
}

func (je *jobItem) Cancel() {
	if atomic.CompareAndSwapInt32(&je.cancelFlag, 0, 1) {
		je.clock.rmJob(je)
		je.clock = nil
	}
}

// Count implement for Job
func (je jobItem) Count() uint64 {
	return je.count
}

// Times implement for Job
func (je jobItem) Times() uint64 {
	return je.actionTimes
}
