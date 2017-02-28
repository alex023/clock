package clock

import (
	"fmt"
	"time"
	"log"
	"sync"
)

//ExampleClock_Repeat1 基于函数回调，一个对重复任务的使用演示。
func ExampleClock_Repeat1() {
	var (
		myClock   = NewClock()
		counter   = 0
		mut       sync.Mutex
		sigalChan = make(chan struct{}, 0)
	)
	fn := func() {
		fmt.Println("schedule repeat")
		mut.Lock()
		defer mut.Unlock()
		counter++
		if counter == 3 {
			sigalChan <- struct{}{}
		}

	}
	//创建一个重复执行的任务，间隔50毫秒
	event, inserted := myClock.AddJobRepeat(time.Duration(time.Millisecond*50), 0, fn)
	if !inserted {
		log.Println("新增事件失败")
	}

	//等待阻塞信号
	<-sigalChan
	myClock.DelJob(event.Id())

	//休眠1秒，判断任务是否真正注销
	time.Sleep(time.Second)
	//Output:
	//
	//schedule repeat
	//schedule repeat
	//schedule repeat
}

//ExampleClock_Repeat2 ，基于函数回调，演示添加有次数限制的重复任务
//  执行3次之后，撤销定时事件
func ExampleClock_Repeat2() {
	var (
		myClock = NewClock()
	)
	//创建一个重复执行的任务，定时1秒
	fn := func() {
		fmt.Println("schedule repeat")
	}
	_, inserted := myClock.AddJobRepeat(time.Duration(time.Millisecond*200), 3, fn)
	if !inserted {
		log.Println("新增事件失败")
	}

	time.Sleep(time.Second)
	//Output:
	//
	//schedule repeat
	//schedule repeat
	//schedule repeat
}

//ExampleClock_Once 基于函数回调，对一次性任务正常使用的演示。
func ExampleClock_Once() {
	var (
		jobClock = NewClock()
		jobFunc  = func() {
			fmt.Println("schedule once")
		}
	)
	//创建一个一次性任务，定时1毫秒
	jobClock.AddJobWithTimeout(time.Duration(100*time.Millisecond), jobFunc)

	//等待1秒，看看足够的时间条件下，事件是否会执行多次
	time.Sleep(1 * time.Second)

	//Output:
	//
	//schedule once
}

//ExampleClock_Once2 基于事件提醒，对一次性任务中途放弃的使用演示。
func ExampleClock_Once2() {
	var (
		myClock = NewClock()
		jobFunc = func() {
			fmt.Println("schedule once")
		}
		actionTime = time.Now().Add(time.Millisecond * 500)
	)
	//创建一次性任务，定时500ms
	job, _ := myClock.AddJobWithDeadtime(actionTime, jobFunc)

	//任务执行前，撤销任务
	time.Sleep(time.Millisecond * 300)
	myClock.DelJob(job.Id())

	//等待2秒，正常情况下，事件不会再执行
	time.Sleep(2 * time.Second)

	//Output:
	//
	//
}
