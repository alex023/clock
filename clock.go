// Package clock
// 定时任务消息通知队列，实现了单一timer对多个注册任务的触发调用，其特点在于：
//	1、适用于低密度，大跨度的单次、多次定时任务。
// 	2、支持高达万次/秒的定时任务执行或提醒。
//	3、支持同一时间点，多个任务提醒。
//	4、支持注册任务的函数调用，及事件通知。
//	5、低延迟，在万次/秒情况下，平均延迟在200微秒内，最大不超过100毫秒。
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
	"fmt"
	"github.com/HuKeping/rbtree"
	"math"
	"time"
)

type JobType int

// Clock 任务队列的控制器
type Clock struct {
	seq              uint64
	jobIndex         map[uint64]*jobItem //缓存任务id-jobItem
	jobList          *rbtree.Rbtree      //job索引，定位撤销通道
	times            uint64              //最多执行次数
	counter          uint64              //已执行次数，不得大于times
	pauseSignChan    chan struct{}
	continueSignChan chan struct{}
}

//NewClock Create a task queue controller
func NewClock() *Clock {
	clock := &Clock{
		jobList:          rbtree.New(),
		pauseSignChan:    make(chan struct{}, 0),
		continueSignChan: make(chan struct{}, 0),
		jobIndex:         make(map[uint64]*jobItem),
	}
	//为Clock内部的事件队列创建一个一直无法执行的最大节点，使任务队列始终存在未完成任务
	untouchedJob := jobItem{
		createTime:   time.Now(),
		IntervalTime: time.Duration(math.MaxInt64),
		f: func() {
			fmt.Println("this jobItem is untouched!")
		},
	}
	now := time.Now()
	_, inserted := clock.addJob(now, untouchedJob.IntervalTime, 1, untouchedJob.f)
	if !inserted {
		panic("NewClock")
	}
	//开启守护协程
	go clock.do()
	clock.continueSignChan <- struct{}{}

	return clock
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

	//定时器暂停
	jl.pauseSignChan <- struct{}{}

	job, inserted = jl.addJob(now, timeout, 1, jobFunc)
	//定时器启动
	jl.continueSignChan <- struct{}{}
	return
}

// AddJobWithTimeout update a timed task with time duration after now
//	@jobId:		job Unique identifier
//	@timeout:	new job do time
func (jl *Clock) UpdateJobTimeout(jobId uint64, timeout time.Duration) (job Job, updated bool) {
	if timeout.Nanoseconds() <= 0 {
		return nil, false
	}
	now := time.Now()

	//pause
	jl.pauseSignChan <- struct{}{}

	jobitem, founded := jl.jobIndex[jobId]
	if !founded {
		//job=nil
		//update=false
		jl.continueSignChan <- struct{}{}
		return
	}
	// update jobitem rbtree node
	jl.jobList.Delete(jobitem)
	jobitem.actionTime = now.Add(timeout)
	jl.jobList.Insert(jobitem)

	//continue
	jl.continueSignChan <- struct{}{}

	updated = true
	job = jobitem
	return
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

	//定时器暂停
	jl.pauseSignChan <- struct{}{}

	job, inserted = jl.addJob(now, timeout, 1, jobFunc)
	//定时器启动
	jl.continueSignChan <- struct{}{}
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

	//定时器暂停
	jl.pauseSignChan <- struct{}{}

	job, inserted = jl.addJob(now, jobInterval, jobTimes, jobFunc)
	//定时器启动
	jl.continueSignChan <- struct{}{}
	return
}

func (jl *Clock) addJob(createTime time.Time, jobInterval time.Duration, jobTimes uint64, jobFunc func()) (job Job, inserted bool) {
	inserted = true
	jl.seq++
	event := &jobItem{
		id:           jl.seq,
		times:        jobTimes,
		createTime:   createTime,
		actionTime:   createTime.Add(jobInterval),
		IntervalTime: jobInterval,
		msgChan:      make(chan Job, 10),
		f:            jobFunc,
	}
	jl.jobIndex[event.id] = event
	jl.jobList.Insert(event)

	job = event
	return

}

// DelJob Deletes the task that has been added to the task queue. If the key does not exist, return false.
func (jl *Clock) DelJob(jobId uint64) (deleted bool) {
	if jobId < 0 {
		deleted = false
		return
	}

	jl.pauseSignChan <- struct{}{}

	job, founded := jl.jobIndex[jobId]
	if !founded {
		jl.continueSignChan <- struct{}{}
		return
	}
	jl.removeJob(job)
	deleted = true
	jl.continueSignChan <- struct{}{}

	return
}

// DelJobs 向任务队列中批量删除给定key!=""的任务事件。
func (jl *Clock) DelJobs(jobIds []uint64) {
	jl.pauseSignChan <- struct{}{}

	for _, jobId := range jobIds {
		job, founded := jl.jobIndex[jobId]
		if founded {
			jl.removeJob(job)
		}
	}

	jl.continueSignChan <- struct{}{}
	return
}
func (jl *Clock) removeJob(job *jobItem) {
	jl.jobList.Delete(job)
	delete(jl.jobIndex, job.Id())
	close(job.msgChan)

	return
}
func (jl *Clock) do() {
	timer := time.NewTimer(time.Duration(math.MaxInt64))
	defer timer.Stop()
Pause:
	<-jl.continueSignChan
	for {
		v := jl.jobList.Min()
		job, _ := v.(*jobItem) //ignore ok-assert
		timeout := job.actionTime.Sub(time.Now())
		timer.Reset(timeout)
		select {
		case <-timer.C:
			jl.counter++

			job.done()

			if job.canContinue() {
				jl.jobList.Delete(job)
				job.actionTime = job.actionTime.Add(job.IntervalTime)
				jl.jobList.Insert(job)
			} else {
				jl.removeJob(job)
			}

		case <-jl.pauseSignChan:
			goto Pause
		}
	}
}

// count 已经执行的任务数。对于重复任务，会计算多次
func (jl *Clock) Counter() uint64 {
	return jl.counter
}

//WaitJobs 待执行任务数
// Note:每次使用，对任务队列会有微秒级的阻塞
func (jl *Clock) WaitJobs() int {
	jl.pauseSignChan <- struct{}{}
	defer func() {
		jl.continueSignChan <- struct{}{}
	}()

	return len(jl.jobIndex) - 1
}

// waitJobs 按序显示当前列表中的定时事件
func (jl *Clock) waitJobs() []Job {
	jobs := make([]Job, 0)

	echo := func(item rbtree.Item) bool {
		if job, ok := item.(Job); ok {
			jobs = append(jobs, job)
		}
		return true
	}
	jl.pauseSignChan <- struct{}{}
	defer func() {
		jl.continueSignChan <- struct{}{}
	}()

	start := jl.jobList.Min()
	jl.jobList.Ascend(start, echo)
	return jobs[:len(jobs)-1]
}
