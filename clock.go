// Package clock
// 定时任务消息通知队列，实现了单一timer对多个注册任务的触发调用，其特点在于：
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
	"time"
	"sync"
)

const _UNTOUCHED = time.Duration(math.MaxInt64)

type JobType int

// Clock 任务队列的控制器
type Clock struct {
	mut      sync.Mutex
	seq      uint64
	jobIndex map[uint64]*jobItem //缓存任务id-jobItem
	jobList  *rbtree.Rbtree      //job索引，定位撤销通道
	times    uint64              //最多执行次数
	counter  uint64              //已执行次数，不得大于times
	timer    *time.Timer         //计时器
}

//NewClock Create a task queue controller
func NewClock() *Clock {
	clock := &Clock{
		jobList:  rbtree.New(),
		jobIndex: make(map[uint64]*jobItem),
		timer:    time.NewTimer(_UNTOUCHED),
	}

	//开启守护协程
	go func() {
		for {
			<-clock.timer.C
			clock.schedule()
		}
	}()

	return clock
}

func (jl *Clock) schedule() {
	jl.mut.Lock()
	defer jl.mut.Unlock()

	jl.counter++

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

// AddJobWithTimeout insert a timed task with time duration after now
// 	@timeout:	duration after now
//	@jobFunc:	action function
//	return
// 	@job:
func (jl *Clock) AddJobWithTimeout(timeout time.Duration, jobFunc func()) (job Job, inserted bool) {
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

// AddJobWithTimeout update a timed task with time duration after now
//	@jobId:		job Unique identifier
//	@timeout:	new job schedule time
func (jl *Clock) UpdateJobTimeout(jobId uint64, timeout time.Duration) (job Job, updated bool) {
	if timeout.Nanoseconds() <= 0 {
		return nil, false
	}
	now := time.Now()

	jl.mut.Lock()
	defer jl.mut.Unlock()

	jobitem, founded := jl.jobIndex[jobId]
	if !founded {
		//job=nil
		//update=false
		return
	}
	// update jobitem rbtree node
	jl.jobList.Delete(jobitem)
	jobitem.actionTime = now.Add(timeout)
	jl.jobList.Insert(jobitem)

	jl.timeRefreshAfterAdd(jobitem)

	updated = true
	job = jobitem
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

// AddJobWithDeadtime insert a timed task with time point after now
//	@timeaction:	Execution start time. must after now,else return false
//	@jobFunc:	Exectuion function
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
//	@jobTimes:	Execution times
//	@jobFunc:	Exectuion funciton
//	return
// 	@job:	返还注册的任务事件。
//Note：
// 对于times为0的任务，在不使用时，务必调用DelJob，以释放。
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
	jl.jobIndex[job.id] = job
	jl.jobList.Insert(job)

	return

}

// DelJob Deletes the task that has been added to the task queue. If the key does not exist, return false.
func (jl *Clock) DelJob(jobId uint64) (deleted bool) {
	if jobId < 0 {
		deleted = false
		return
	}

	jl.mut.Lock()
	defer jl.mut.Unlock()

	job, founded := jl.jobIndex[jobId]
	if !founded {
		return
	}
	jl.removeJob(job)
	deleted = true

	jl.timeRefreshAfterDel()

	return
}

// DelJobs 向任务队列中批量删除给定key!=""的任务事件。
func (jl *Clock) DelJobs(jobIds []uint64) {
	jl.mut.Lock()
	defer jl.mut.Unlock()

	for _, jobId := range jobIds {
		job, founded := jl.jobIndex[jobId]
		if founded {
			jl.removeJob(job)
		}
	}

	jl.timeRefreshAfterDel()

	return
}
func (jl *Clock) removeJob(job *jobItem) {
	jl.jobList.Delete(job)
	delete(jl.jobIndex, job.Id())
	close(job.msgChan)

	return
}

// count 已经执行的任务数。对于重复任务，会计算多次
func (jl *Clock) Counter() uint64 {
	jl.mut.Lock()
	defer jl.mut.Unlock()

	return jl.counter
}

//WaitJobs 待执行任务数
// Note:每次使用，对任务队列会有微秒级的阻塞
func (jl *Clock) WaitJobs() int {
	jl.mut.Lock()
	defer jl.mut.Unlock()

	return len(jl.jobIndex)
}
