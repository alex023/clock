// Package clock is a low consumption, low latency support for frequent updates of large capacity timing manager：
//	1、能够添加一次性、重复性任务，并能在其执行前撤销或频繁更改。
//	2、支持同一时间点，多个任务提醒。
//	3、适用于中等密度，大跨度的单次、多次定时任务。
//	4、支持10万次/秒的定时任务执行、提醒、撤销或添加操作，平均延迟10微秒内
//	5、支持注册任务的函数调用，及事件通知。
// 基本处理逻辑：
//	1、重复性任务，流程是：
//		a、注册重复任务
//		b、时间抵达时，控制器调用注册函数，并发送通知
//		c、如果次数达到限制，则撤销；否则，控制器更新该任务的下次执行时间点
//		d、控制器等待下一个最近需要执行的任务
//	2、一次性任务，可以是服务运行时，当前时间点之后的任意事件，流程是：
//		a、注册一次性任务
//		b、时间抵达时，控制器调用注册函数，并发送通知
//		c、控制器释放该任务
//		d、控制器等待下一个最近需要执行的任务
// 使用方式，参见示例代码。
package clock

import (
	"github.com/HuKeping/rbtree"
	"math"
	"sync"
	"time"
)

const _UNTOUCHED = time.Duration(math.MaxInt64)

// Clock is joblist control
type Clock struct {
	mut     sync.Mutex
	seq     uint64
	jobList *rbtree.Rbtree //inner memory storage
	count   uint64         //已执行次数，不得大于times
	timer   *time.Timer    //计时器
}

//NewClock Create a task queue controller
func NewClock() *Clock {
	c := &Clock{
		jobList: rbtree.New(),
		timer:   time.NewTimer(_UNTOUCHED),
	}

	//开启守护协程
	go func() {
		for {
			<-c.timer.C
			c.schedule()
		}
	}()

	return c
}

func (jl *Clock) schedule() {
	jl.mut.Lock()
	defer jl.mut.Unlock()

	jl.count++

	if item := jl.jobList.Min(); item != nil {
		job := item.(*jobItem)
		job.done()
		if job.canContinue() {
			jl.jobList.Delete(job)
			job.actionTime = job.actionTime.Add(job.intervalTime)
			jl.jobList.Insert(job)
		} else {
			jl.removeJob(job)
		}

		jl.timeRefreshAfterDel()
	} else {
		jl.timer.Reset(_UNTOUCHED)
	}
}

// UpdateJobTimeout update a timed task with time duration after now
//	@job:		job identifier
//	@timeout:	new job schedule time
func (jl *Clock) UpdateJobTimeout(job Job, timeout time.Duration) (updated bool) {
	if timeout.Nanoseconds() <= 0 {
		return false
	}
	now := time.Now()

	jl.mut.Lock()
	defer jl.mut.Unlock()

	item, ok := job.(*jobItem)
	if !ok {
		return false
	}
	// update jobitem rbtree node
	jl.jobList.Delete(item)
	item.actionTime = now.Add(timeout)
	jl.jobList.Insert(item)

	jl.timeRefreshAfterAdd(item)

	updated = true
	return
}
func (jl *Clock) timeRefreshAfterDel() {
	if head := jl.jobList.Min(); head != nil {
		item := head.(*jobItem)
		jl.timer.Reset(item.actionTime.Sub(time.Now()))
	}

}
func (jl *Clock) timeRefreshAfterAdd(new *jobItem) {
	if head := jl.jobList.Min(); head != nil && head == new {
		item := head.(*jobItem)
		jl.timer.Reset(item.actionTime.Sub(time.Now()))
	}

}

// AddJobWithInterval insert a timed task with time duration after now
// 	@timeout:	duration after now
//	@jobFunc:	action function
//	return
// 	@job:
func (jl *Clock) AddJobWithInterval(timeout time.Duration, jobFunc func()) (job Job, inserted bool) {
	if timeout.Nanoseconds() <= 0 {
		return nil, false
	}
	now := time.Now()

	jl.mut.Lock()

	newitem, inserted := jl.addJob(now, timeout, 1, jobFunc)
	jl.timeRefreshAfterAdd(newitem)

	jl.mut.Unlock()
	job = newitem
	return
}

// AddJobWithDeadtime insert a timed task with time point after now
//	@timeaction:	Execution start time. must after now,else return false
//	@jobFunc:	Execution function
//	return
// 	@job:		返还注册的任务事件。
func (jl *Clock) AddJobWithDeadtime(timeaction time.Time, jobFunc func()) (job Job, inserted bool) {
	timeout := timeaction.Sub(time.Now())
	if timeout.Nanoseconds() <= 0 {
		return nil, false
	}
	now := time.Now()

	jl.mut.Lock()

	newItem, inserted := jl.addJob(now, timeout, 1, jobFunc)
	jl.timeRefreshAfterAdd(newItem)

	jl.mut.Unlock()

	job = newItem
	return
}

// AddJobRepeat add a repeat task with interval duration
//	@jobInterval:	The two time interval operation
//	@jobTimes:	The number of job execution
//	@jobFunc:	The function of job execution
//	return
// 	@job:		job interface。
//Note：
// when jobTimes==0,the job will be executed without limitation。If you no longer use, be sure to call the DelJob method to release
func (jl *Clock) AddJobRepeat(jobInterval time.Duration, jobTimes uint64, jobFunc func()) (job Job, inserted bool) {
	if jobInterval.Nanoseconds() <= 0 {
		return nil, false
	}
	now := time.Now()

	jl.mut.Lock()
	newItem, inserted := jl.addJob(now, jobInterval, jobTimes, jobFunc)
	jl.timeRefreshAfterAdd(newItem)
	jl.mut.Unlock()

	job = newItem
	return
}

func (jl *Clock) addJob(createTime time.Time, jobInterval time.Duration, jobTimes uint64, jobFunc func()) (job *jobItem, inserted bool) {
	inserted = true
	jl.seq++
	job = &jobItem{
		id:           jl.seq,
		times:        jobTimes,
		createTime:   createTime,
		actionTime:   createTime.Add(jobInterval),
		intervalTime: jobInterval,
		msgChan:      make(chan Job, 10),
		fn:           jobFunc,
	}
	jl.jobList.Insert(job)

	return

}

// DelJob Deletes the task that has been added to the task queue. If the key does not exist, return false.
func (jl *Clock) DelJob(job Job) (deleted bool) {
	if job == nil {
		deleted = false
		return
	}

	jl.mut.Lock()
	defer jl.mut.Unlock()

	item, ok := job.(*jobItem)
	if !ok {
		return false
	}
	jl.removeJob(item)
	deleted = true

	jl.timeRefreshAfterDel()

	return
}

// DelJobs remove jobs from clock schedule list
func (jl *Clock) DelJobs(jobIds []Job) {
	jl.mut.Lock()
	defer jl.mut.Unlock()

	for _, job := range jobIds {
		item, ok := job.(*jobItem)
		if !ok {
			continue
		}
		jl.removeJob(item)
	}

	jl.timeRefreshAfterDel()

	return
}
func (jl *Clock) removeJob(item *jobItem) {
	jl.jobList.Delete(item)
	close(item.msgChan)

	return
}

// Count 已经执行的任务数。对于重复任务，会计算多次
func (jl *Clock) Count() uint64 {
	jl.mut.Lock()
	defer jl.mut.Unlock()

	return jl.count
}

//WaitJobs 待执行任务数
// Note:每次使用，对任务队列会有微秒级的阻塞
func (jl *Clock) WaitJobs() uint {
	jl.mut.Lock()
	defer jl.mut.Unlock()

	return jl.jobList.Len()
}
