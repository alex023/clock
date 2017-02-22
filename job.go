package clock

import (
	"time"
	"github.com/HuKeping/rbtree"
)

// Job 定时任务的外部访问接口
type Job interface {
	Id() uint64      //Id 获取该任务的唯一标识号
	C() <-chan Job   //C 当Job被执行的时候，将会同时发送一个事件消息，告知外部应用。
	Counter() uint64 //计数器，表示已执行（或触发）的次数
	Times() uint64   //允许执行的最大次数
	delay() time.Duration
}

// jobItem implementation of "Job" and "rbtree.Item"
type jobItem struct {
	times        uint64        //允许执行的最大次数
	counter      uint64        //计数器，表示已执行（或触发）的次数
	IntervalTime time.Duration //间隔时间
	id           uint64        //唯一键值，删除或更新需要该参数。由控制器创建,可以通过Key()获取
	createTime   time.Time     //创建时间
	actionTime   time.Time     //计算得出的此次执行时间点，有误差
	f            func()        //事件函数
	msgChan      chan Job      //消息通道，执行时，控制器通过该通道向外部传递消息
}

// Less 遵循采用的红黑树的排序接口，对actionTime、Key的两重排序
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

//Id 获取Job的唯一标志号
func (je *jobItem) Id() uint64 {
	return je.id
}

//C 当Job被执行的时候，将会同时发送一个事件消息，告知外部应用。
func (je *jobItem) C() <-chan Job {
	return je.msgChan
}
func (je *jobItem) done() {
	je.counter++
	if je.f != nil {
		go je.f()
	}
	select {
	case je.msgChan <- je:
	default:
	//some times,client should not receive msgChan,so must discard jobItem when blocking
	}
}
func (je *jobItem) canContinue() bool {
	if je.times == 0 {
		return true
	}
	return je.times > je.counter

}
func (je jobItem) Counter() uint64 {
	return je.counter
}
func (je jobItem) Times() uint64 {
	return je.times
}

//delay 获取Job执行时相对之前设置的时间点延迟时长
//for test
func (je *jobItem) delay() time.Duration {
	now := time.Now()
	return now.Sub(je.actionTime)
}
