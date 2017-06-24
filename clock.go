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
	"sync/atomic"
	"time"
)

const _UNTOUCHED = time.Duration(math.MaxInt64)

var (
	defaultClock *Clock
	oncedo       sync.Once
)

//Default return singal default clock
func Default() *Clock {
	oncedo.Do(initClock)
	return defaultClock
}
func initClock() {
	defaultClock = NewClock()
}

// Clock is jobs schedule
type Clock struct {
	seq        uint64
	jobQueue   *rbtree.Rbtree //inner memory storage
	count      uint64         //已执行次数，不得大于times
	pauseChan  chan struct{}
	resumeChan chan struct{}
	exitChan   chan struct{}
}

var singal = struct{}{}

//NewClock Create a task queue controller
func NewClock() *Clock {
	c := &Clock{
		jobQueue:   rbtree.New(),
		pauseChan:  make(chan struct{}, 0),
		resumeChan: make(chan struct{}, 0),
		exitChan:   make(chan struct{}, 0),
	}

	c.start()

	return c
}
func (jl *Clock) start() {
	untouchedJob := jobItem{
		createTime:   time.Now(),
		intervalTime: time.Duration(math.MaxInt64),
		fn: func() {
			//this jobItem is untouched.
		},
	}

	now := time.Now()
	_, inserted := jl.addJob(now, untouchedJob.intervalTime, 1, untouchedJob.fn)
	if !inserted {
		panic("Clock start")
	}
	//开启守护协程
	go jl.schedule()
	jl.resume()
}

func (jl *Clock) pause() {
	jl.pauseChan <- singal
}
func (jl *Clock) resume() {
	jl.resumeChan <- singal
}
func (jl *Clock) exit() {
	jl.exitChan <- singal
}

func (jl *Clock) immediate() {
	for {
		if item := jl.jobQueue.Min(); item != nil {
			atomic.AddUint64(&jl.count, 1)

			job := item.(*jobItem)
			job.action(false)

			jl.removeJob(job)

		} else {
			break
		}
	}
}

func (jl *Clock) schedule() {
	timer := time.NewTimer(_UNTOUCHED)
	defer timer.Stop()
Pause:
	<-jl.resumeChan
	for {
		v := jl.jobQueue.Min()
		job, _ := v.(*jobItem) //ignore ok-assert
		timeout := job.actionTime.Sub(time.Now())
		timer.Reset(timeout)
		select {
		case <-timer.C:
			jl.count++

			job.action(true)

			if job.actionTimes == 0 || job.actionTimes > job.count {
				jl.jobQueue.Delete(job)
				job.actionTime = job.actionTime.Add(job.intervalTime)
				jl.jobQueue.Insert(job)
			} else {
				jl.removeJob(job)
			}
		case <-jl.pauseChan:
			goto Pause
		case <-jl.exitChan:
			goto Exit
		}
	}
Exit:
}

// UpdateJobTimeout update a timed task with time duration after now
//	@job:			job identifier
//	@actionTime:	new job schedule time,must be greater than 0
func (jl *Clock) UpdateJobTimeout(job Job, actionTime time.Duration) (updated bool) {
	if actionTime.Nanoseconds() <= 0 {
		return false
	}
	now := time.Now()

	jl.pause()
	defer jl.resume()

	item, ok := job.(*jobItem)
	if !ok {
		return false
	}
	// update jobitem in job queue
	jl.jobQueue.Delete(item)
	item.actionTime = now.Add(actionTime)
	jl.jobQueue.Insert(item)

	updated = true
	return
}

// AddJobWithInterval insert a timed task with time duration after now
// 	@actionTime:	duration after now
//	@jobFunc:		action function
//	return
// 	@jobScheduled:	A reference to a task that has been scheduled.
func (jl *Clock) AddJobWithInterval(actionInterval time.Duration, jobFunc func()) (jobScheduled Job, inserted bool) {
	if actionInterval.Nanoseconds() <= 0 {
		return nil, false
	}
	now := time.Now()

	jl.pause()

	newitem, inserted := jl.addJob(now, actionInterval, 1, jobFunc)

	jl.resume()

	jobScheduled = newitem
	return
}

// AddJobWithDeadtime insert a timed task with time point after now
//	@actionTime:	Execution start time. must after now,else return false
//	@jobFunc:		Execution function
//	return
// 	@jobScheduled:	A reference to a task that has been scheduled.
func (jl *Clock) AddJobWithDeadtime(actionTime time.Time, jobFunc func()) (jobScheduled Job, inserted bool) {
	actionInterval := actionTime.Sub(time.Now())
	if actionInterval.Nanoseconds() <= 0 {
		return nil, false
	}
	now := time.Now()

	jl.pause()

	newItem, inserted := jl.addJob(now, actionInterval, 1, jobFunc)

	jl.resume()

	jobScheduled = newItem
	return
}

// AddJobRepeat add a repeat task with interval duration
//	@actionTime:	The two time interval operation
//	@jobTimes:		The number of job execution
//	@jobFunc:		The function of job execution
//	return
// 	@jobScheduled:	A reference to a task that has been scheduled.
//Note：
// when jobTimes==0,the job will be executed without limitation。If you no longer use, be sure to call the DelJob method to release
func (jl *Clock) AddJobRepeat(actionInterval time.Duration, jobTimes uint64, jobFunc func()) (jobScheduled Job, inserted bool) {
	if actionInterval.Nanoseconds() <= 0 {
		return nil, false
	}
	now := time.Now()

	jl.pause()
	newItem, inserted := jl.addJob(now, actionInterval, jobTimes, jobFunc)

	jl.resume()

	jobScheduled = newItem
	return
}

func (jl *Clock) addJob(createTime time.Time, actionInterval time.Duration, actionTimes uint64, jobFunc func()) (job *jobItem, inserted bool) {
	inserted = true
	jl.seq++
	job = &jobItem{
		id:           jl.seq,
		actionTimes:  actionTimes,
		createTime:   createTime,
		actionTime:   createTime.Add(actionInterval),
		intervalTime: actionInterval,
		msgChan:      make(chan Job, 10),
		fn:           jobFunc,
	}
	jl.jobQueue.Insert(job)

	return

}

// DelJob Deletes the task that has been added to the task queue. If the key does not exist, return false.
func (jl *Clock) DelJob(job Job) (deleted bool) {
	if job == nil {
		deleted = false
		return
	}

	jl.pause()
	defer jl.resume()

	item, ok := job.(*jobItem)
	if !ok {
		return false
	}
	jl.removeJob(item)
	deleted = true

	return
}

// DelJobs remove jobs from clock schedule list
func (jl *Clock) DelJobs(jobIds []Job) {
	jl.pause()
	defer jl.resume()

	for _, job := range jobIds {
		item, ok := job.(*jobItem)
		if !ok {
			continue
		}
		jl.removeJob(item)
	}

	return
}
func (jl *Clock) removeJob(item *jobItem) {
	jl.jobQueue.Delete(item)
	close(item.msgChan)

	return
}

// Count 已经执行的任务数。对于重复任务，会计算多次
func (jl *Clock) Count() uint64 {
	return atomic.LoadUint64(&jl.count)
}

//重置Clock的内部状态
func (jl *Clock) Reset() *Clock {
	jl.exit()
	jl.count = 0

	jl.cleanJobs()
	jl.start()
	return jl
}

func (jl *Clock) cleanJobs() {
	item := jl.jobQueue.Min()
	for item != nil {
		job, ok := item.(*jobItem)
		if ok {
			jl.removeJob(job)
		}
		item = jl.jobQueue.Min()
	}
}

//WaitJobs get how much jobs waiting for call
func (jl *Clock) WaitJobs() uint {
	tmp := jl.jobQueue.Len() - 1
	if tmp > 0 {
		return tmp
	}
	return 0
}

//Stop stop clock , and cancel all waiting jobs
func (jl *Clock) Stop() {
	jl.exit()

	jl.cleanJobs()
}

//StopGracefull stop clock ,and do once every waiting job including Once\Reapeat
//Note:对于任务队列中，即使安排执行多次或者不限次数的，也仅仅执行一次。
func (jl *Clock) StopGracefull() {
	jl.exit()

	jl.immediate()
}
