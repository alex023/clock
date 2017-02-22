// Package clock
// 定时任务消息通知队列，实现了单一timer对多个注册任务的触发调用，其特点在于：
//	1、适用于低密度，大跨度的单次、多次定时任务。
// 	2、支持高达万次/秒的定时任务执行或提醒。
//	3、支持同一时间点，多个任务提醒。
//	4、支持注册任务的函数调用，及事件通知。
//	5、低延迟，在万次/秒情况下，平均延迟在200微秒内，最大不超过100毫秒。
// 基本处理逻辑：
//	1、定时轮训事件，流程是：
// 		a、注册重复任务
//		b、时间抵达时，控制器调用注册函数，并发送通知
//		c、控制器更新该任务的下次执行时间点
//		d、控制器等待最近一次需要执行的任务
//	2、一次性事件，可以是服务运行时，当前时间点之后的任意事件，流程是：
// 		a、注册一次性任务
//		b、时间抵达时，控制器调用注册函数，并发送通知
//		c、控制器释放该任务
//		d、控制器等待最近一次需要执行的任务
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

//NewClock 创建一个任务队列的控制器
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

// AddJobWithTimeout 向任务队列中添加一次性任务。
//	@timeout:	相对于当前时间，延后多久执行任务，不可为负。
//	@jobFunc:	定时执行的任务。
//	return
// 	@job:		返还注册的任务事件。
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

// AddJobWithDeadtime 向任务队列中添加一次性任务。
//	@timeaction:	在当前时间之后的时间点，不可早于当前时间。
//	@jobFunc:	定时执行的任务。
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

// AddJobRepeat 向任务队列中添加重复执行任务。
//	@jobInterval:	相对于当前时间，间隔事件必须大于0。
//	@jobTimes:	执行次数，为0表示不限制
//	@jobFunc:	定时执行的任务。
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

// DelJob 向任务队列中删除已经添加的任务，如果该key不存在，则返回false.
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

// counter 已经执行的任务数。对于重复任务，会计算多次
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
