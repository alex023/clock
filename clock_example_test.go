package clock

import (
	"fmt"
	"log"
	"time"
)

//ExampleClock_AddJobRepeat, show how to add repeat job。
func ExampleClock_AddJobRepeat() {
	var (
		myClock = NewClock()
	)
	fn := func() {
		fmt.Println("schedule repeat")

	}
	//create a task that executes three times,interval 50 millisecond
	_, inserted := myClock.AddJobRepeat(time.Duration(time.Millisecond*50), 3, fn)
	if !inserted {
		log.Println("failure")
	}

	//wait a second,watching
	time.Sleep(time.Second)
	//Output:
	//
	//schedule repeat
	//schedule repeat
	//schedule repeat
}

//ExampleClock_AddJobForEvent，show receive signal when job is called by system
func ExampleClock_AddJobForEvent() {
	var (
		myClock = NewClock()
	)
	//define a repeat task
	fn := func() {
		//fmt.Println("schedule repeat")
	}
	//add in clock,execute three times,interval 200 millisecond
	job, inserted := myClock.AddJobRepeat(time.Duration(time.Millisecond*200), 3, fn)
	if !inserted {
		log.Println("failure")
	}

	go func() {
		for _ = range job.C() {
			fmt.Println("job done")
		}
	}()
	//wait a second,watching
	time.Sleep(time.Second)
	//Output:
	//
	//job done
	//job done
	//job done
}

//ExampleClock_AddJobWithInterval,show how to add a job just do once with interval
func ExampleClock_AddJobWithInterval() {
	var (
		jobClock = NewClock()
		jobFunc  = func() {
			fmt.Println("schedule once")
		}
	)
	//add a task that executes once,interval 100 millisecond
	jobClock.AddJobWithInterval(time.Duration(100*time.Millisecond), jobFunc)

	//wait a second,watching
	time.Sleep(1 * time.Second)

	//Output:
	//
	//schedule once
}

//ExampleClock_AddJobWithDeadtime,show how to add a job with a point time
func ExampleClock_AddJobWithDeadtime() {
	var (
		myClock = Default()
		jobFunc = func() {
			fmt.Println("schedule once")
		}
		actionTime = time.Now().Add(time.Millisecond * 500)
	)
	//创建一次性任务，定时500ms
	job, _ := myClock.AddJobWithDeadtime(actionTime, jobFunc)

	//任务执行前，撤销任务
	time.Sleep(time.Millisecond * 300)
	job.Cancel()

	//等待2秒，正常情况下，事件不会再执行
	time.Sleep(2 * time.Second)

	//Output:
	//
	//
}

//ExampleClock_AddJobWithDeadtime,show how to cancel a job which added in system
func ExampleClock_RmJob() {
	var (
		myClock = NewClock()
		count   int
		jobFunc = func() {
			count++
			fmt.Println("do ", count)
		}
	)
	//创建任务，间隔1秒，执行两次
	job, _ := myClock.AddJobRepeat(time.Second*1, 2, jobFunc)

	//任务执行前，撤销任务
	time.Sleep(time.Millisecond * 500)
	job.Cancel()

	//等待2秒，正常情况下，事件不会再执行
	time.Sleep(2 * time.Second)

	//再次添加一个任务，病观察
	myClock.AddJobRepeat(time.Second*1, 1, jobFunc)
	time.Sleep(time.Second * 2)
	//Output:
	//
	//do  1
}
