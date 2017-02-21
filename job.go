package clock

import (
	"time"
	"github.com/HuKeping/rbtree"
)

// Job 注册的任务信息
type Job struct {
	Type         JobType       //任务类型，确定是重复性还是一次性
	Times        uint64        //允许执行的最大次数
	Counter      uint64        //计数器，表示已执行（或触发）的次数
	IntervalTime time.Duration //间隔时间
	id           uint64        //唯一键值，删除或更新需要该参数。由控制器创建,可以通过Key()获取
	createTime   time.Time     //创建时间
	actionTime   time.Time     //计算得出的此次执行时间点，有误差
	f            func()        //事件函数
	msgChan      chan *Job     //消息通道，执行时，控制器通过该通道向外部传递消息
}

// Less 遵循采用的红黑树的排序接口，对actionTime、Key的两重排序
func (je Job) Less(another rbtree.Item) bool {
	item, ok := another.(*Job)
	if !ok {
		return false
	}
	if !je.actionTime.Equal(item.actionTime) {
		return je.actionTime.Before(item.actionTime)
	}
	return je.id < item.id
}

//Id JackJob在JackClock中唯一标识，JackClock在执行DelJob时将会使用
func (je Job) Id() uint64 {
	return je.id
}

//C 当Job被执行的时候，将会同时发送一个事件消息，告知外部应用。
func (je *Job) C() <-chan *Job {
	return je.msgChan
}
func (je *Job) done() {
	je.Counter++
	if je.f != nil {
		go je.f()
	}
	select {
	case je.msgChan <- je:
	default:
	//some times,client should not receive msgChan,so must discard job when blocking
	}
}
func (je *Job) canContinue() bool {
	if je.Times == 0 {
		return true
	}
	return je.Times > je.Counter

}

//Delay 获取Job执行时相对之前设置的时间点延迟时长
func (je *Job) Delay() time.Duration {
	now := time.Now()
	return now.Sub(je.actionTime)
}
