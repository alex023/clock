package clock

import (
	"time"
	"math/rand"
	"sync"
	"testing"
	"math"
)

var (
	r = rand.New(rand.NewSource(time.Now().Unix()))
)
//counter 支持并发的计数器
type Counter struct {
	sync.Mutex
	counter int
}

func (counter *Counter) AddOne() {
	counter.Lock()
	counter.counter++
	counter.Unlock()
}
func (counter *Counter) Count() int {
	return counter.counter
}

func TestClock_Create(t *testing.T) {
	myClock := NewClock()
	if myClock.WaitJobs() != 0 || myClock.Counter() != 0 {
		t.Errorf("JobList init have error.len=%d,counter=%d", myClock.WaitJobs(), myClock.Counter())
		//joblist.Debug()
	}

}

func TestClock_AddOnceJob(t *testing.T) {
	var (
		randscope = 50 * 1000 * 1000 //随机范围
		interval  = time.Millisecond*100 + time.Duration(r.Intn(randscope))
		myClock   = NewClock()
		jobFunc   = func() {
			//fmt.Println("任务事件")
		}
	)

	//插入时间间隔≤0，应该不允许添加
	if _, inserted := myClock.AddJobWithTimeout(0, jobFunc); inserted {
		t.Error("任务添加失败，加入了间隔时间≤0的任务。")
	}

	if _, inserted := myClock.AddJobWithTimeout(interval, jobFunc); !inserted {
		t.Error("任务添加失败，未加入任务。")
	}

	time.Sleep(time.Second)

	if myClock.Counter() != 1 {
		t.Errorf("任务执行存在问题，应该执行%d次,实际执行%d次", 1, myClock.Counter())
	}
}

//TestClock_WaitJobs 测试当前待执行任务列表中的事件
func TestClock_WaitJobs(t *testing.T) {
	var (
		myClock   = NewClock()
		randscope = 50 * 1000 * 1000 //随机范围
		interval  = time.Millisecond*50 + time.Duration(r.Intn(randscope))
		jobFunc   = func() {
			//fmt.Println("任务事件")
		}
	)
	job, inserted := myClock.AddJobRepeat(interval, 0, jobFunc)
	if !inserted {
		t.Error("定时任务创建失败")
	}
	time.Sleep(time.Second)

	if myClock.WaitJobs() != 1 {
		t.Error("任务添加异常")
	}
	lists := myClock.waitJobs()
	if len(lists) != myClock.WaitJobs() {
		t.Error("数据列表操作获取的数据与Clock实际情况不一致！")
	}
	myClock.DelJob(job.Id())

}

//TestClock_AddRepeatJob 测试重复任务定时执行情况
func TestClock_AddRepeatJob(t *testing.T) {
	var (
		myClock   = NewClock()
		jobsNum   = uint64(1000)                                            //执行次数
		randscope = 50 * 1000                                               //随机范围
		interval  = time.Microsecond*100 + time.Duration(r.Intn(randscope)) //100-150µs时间间隔
		counter   = new(Counter)
	)
	f := func() {
		counter.AddOne()
	}
	job, inserted := myClock.AddJobRepeat(interval, jobsNum, f)
	if !inserted {
		t.Error("任务初始化失败，任务事件没有添加成功")
	}
	for range job.C(){

	}
	//重复任务的方法是协程调用，可能还没有执行，job.C就已经退出，需要阻塞观察
	time.Sleep(time.Second)
	if int(myClock.Counter()) != counter.Count() || counter.Count() != int(jobsNum) {
		t.Errorf("任务添加存在问题,应该%v次，实际执行%v\n", jobsNum, counter.Count())
	}

}

//TestClock_AddRepeatJob2 测试间隔时间不同的两个重复任务，是否会交错执行
func TestClock_AddRepeatJob2(t *testing.T) {
	var (
		myClock    = NewClock()
		interval1  = time.Millisecond * 20 //间隔20毫秒
		interval2  = time.Millisecond * 20 //间隔20毫秒
		singalChan = make(chan int, 10)
	)
	jobFunc := func(sigal int) {
		singalChan <- sigal

	}
	go func() {
		cacheSigal := 2
		for z := range singalChan {
			if z == cacheSigal {
				t.Error("两个任务没有间隔执行")
			} else {
				cacheSigal = z
			}
		}
	}()
	call1, inserted1 := myClock.AddJobRepeat(interval1, 0, func() { jobFunc(1) })
	time.Sleep(time.Millisecond * 10)
	call2, inserted2 := myClock.AddJobRepeat(interval2, 0, func() { jobFunc(2) })

	if !inserted1 || !inserted2 {
		t.Error("任务初始化失败，没有添加成功")
	}
	time.Sleep(time.Second)

	myClock.DelJob(call1.Id())
	myClock.DelJob(call2.Id())

}

//TestClock_AddMixJob 测试一次性任务+重复性任务的运行撤销情况
func TestClock_AddMixJob(t *testing.T) {
	var (
		myClock  = NewClock()
		counter1 int
		counter2 int
	)
	f1 := func() {
		counter1++
	}
	f2 := func() {
		counter2++
	}
	_, inserted1 := myClock.AddJobWithTimeout(time.Millisecond*500, f1)
	_, inserted2 := myClock.AddJobRepeat(time.Millisecond*300, 0, f2)

	if !inserted1 && !inserted2 {
		t.Fatal("任务添加失败！")
	}
	time.Sleep(time.Second * 2)
	if counter1 != 1 || counter2 < 5 {
		t.Errorf("执行次数异常！,一次性任务执行了:%v，重复性任务执行了%v\n", counter1, counter2)
	}
}

//TestClock_AddJobs 测试短时间，高频率的情况下，事件提醒功能能否实现。
func TestClock_AddJobs(t *testing.T) {
	var (
		jobsNum   = 200000                 //添加任务数量
		randscope = 1 * 1000 * 1000 * 1000 //随机范围1秒
		myClock   = NewClock()
		counter   = &Counter{}
		wg        sync.WaitGroup
	)
	f := func() {
		//do nothing
	}
	//创建jobsNum个任务，每个任务都会间隔[1,2)秒内执行一次
	for i := 0; i < jobsNum; i++ {
		job, inserted := myClock.AddJobWithTimeout(time.Second+time.Duration(r.Intn(randscope)), f)
		if !inserted {
			t.Error("任务添加存在问题")
			break
		}
		wg.Add(1)
		go func() {
			<-job.C()
			counter.AddOne() //收到消息就计数
			wg.Done()
		}()
	}
	wg.Wait()
	if jobsNum != int(myClock.Counter()) || jobsNum != counter.Count() {
		t.Errorf("应该执行%v次，实际执行%v次,外部信号接受到%v次。\n", jobsNum, myClock.Counter(), counter.Count())
	}
}

//TestClock_AddEvent200kJob 测试20万条任务下，其中任意一条数据从加入到执行的时间延迟，是否超过约定的最大值
// 目标：
// 	1.按照10k次/秒的任务频率，延时超过200µs的不得超过2%。
//	2.不得有任何一条事件提醒，延时超过100ms。
// Note:笔记本(尤其是windows操作系统）可能无法通过测试
func TestClock_AddEvent200kJob(t *testing.T) {
	var (
		jobsNum              = 200000 //添加任务数量
		beyondCounter  = &Counter{}
		wg            sync.WaitGroup
		interval      = time.Microsecond * 100
		delay1        = time.Microsecond * 200 //适度允许的小延时
		delay2        = time.Millisecond * 100 //不允许出现的最大延时
		myClock       = NewClock()
	)
	//初始化20万条任务。考虑到初始化耗时，延时1秒后启动
	//按照每秒10k次，间隔100µs一条。
	for i := 0; i < jobsNum; i++ {
		jobInterval := time.Second + time.Duration(i)*interval
		job, inserted := myClock.AddJobWithTimeout(jobInterval, nil)
		if !inserted {
			t.Error("任务添加存在问题")
			break
		}
		wg.Add(1)
		go func() {
			event := <-job.C()
			if event.delay().Nanoseconds() > delay1.Nanoseconds() {
				beyondCounter.AddOne()
			}
			if event.delay().Nanoseconds() > delay2.Nanoseconds() {
				t.Fatalf("事件提醒延时=%vµs,超过误差最大允许值%vµs:\njobItem%+v\n", event.delay().Nanoseconds()/1000, delay2.Nanoseconds()/1000, event)
			}
			wg.Done()
		}()
	}
	wg.Wait()
	if jobsNum != int(myClock.Counter()) {
		t.Errorf("应该执行%v次，实际执行%v次。所有值应该相等。\n", jobsNum, myClock.Counter())
	}
	if beyondCounter.Count()*100/jobsNum >= 2 {
		t.Errorf("超时任务数=%v,超过了总数%v的2%%。/n", beyondCounter.Count(), jobsNum)
	}
}

//TestClock_DelJob 检测待运行任务中，能否随机删除一条任务。
func TestClock_DelJob(t *testing.T) {
	//思路：
	//新增一定数量的任务，延时1秒开始执行
	//在一秒内，删除所有的任务。
	//如果执行次数=0，说明一秒内无法满足对应条数的增删
	var (
		jobsNum   = 20000
		randscope = 1 * 1000 * 1000 * 1000
		jobs      = make([]Job, jobsNum)
		delmod    = r.Intn(jobsNum)
		myClock   = NewClock()
	)
	for i := 0; i < jobsNum; i++ {
		delay := time.Second + time.Duration(r.Intn(randscope)) //增加一秒作为延迟，以避免删除的时候，已经存在任务被通知执行，导致后续判断失误
		job, _ := myClock.AddJobWithTimeout(delay, nil)
		jobs[i] = job
	}

	deleted := myClock.DelJob(jobs[delmod].Id())
	if !deleted || myClock.WaitJobs() != jobsNum-1 {
		t.Errorf("任务删除%v，删除后，应该只剩下%v条任务，实际还有%v条\n", deleted, myClock.Counter(), jobsNum-1)

	}
}

// TestJob_Delay 判断延时是否存在超过100ms
func TestJob_Delay(t *testing.T) {
	var (
		jobsNum  = 100000 //添加任务数量
		wg       sync.WaitGroup
		interval = time.Microsecond * 100
		myClock  = NewClock()
	)

	//初始化20万条任务。考虑到初始化耗时，延时1秒后启动
	//按照每秒10k次，间隔100µs一条。
	for i := 0; i < jobsNum; i++ {
		jobInterval := time.Second + time.Duration(i)*interval
		job, inserted := myClock.AddJobWithTimeout(jobInterval, nil)
		if !inserted {
			t.Error("任务添加存在问题")
			break
		}
		wg.Add(1)
		go func() {
			<-job.C()
			if job.delay().Seconds() > 0.01 {
				t.Error("存在延时超过100ms极限的情况：", job.delay().Seconds())
			}
			wg.Done()
		}()
	}
	wg.Wait()

}

//TestClock_DelJobs 本测试主要检测添加、删除任务的性能。保证每秒1万次新增+删除操作。
func TestClock_DelJobs(t *testing.T) {
	//思路：
	//新增一定数量的任务，延时1秒开始执行
	//在一秒内，删除所有的任务。
	//如果执行次数！=0，说明一秒内无法满足对应条数的增删
	var (
		myClock     = NewClock()
		jobsNum     = 20000
		randscope   = 1 * 1000 * 1000 * 1000
		jobs        = make([]Job, jobsNum)
		wantdeljobs = make([]uint64, jobsNum)
	)
	for i := 0; i < jobsNum; i++ {
		delay := time.Second + time.Duration(r.Intn(randscope)) //增加一秒作为延迟，以避免删除的时候，已经存在任务被通知执行，导致后续判断失误
		job, _ := myClock.AddJobWithTimeout(delay, nil)
		jobs[i] = job
		wantdeljobs[i] = job.Id()
	}

	myClock.DelJobs(wantdeljobs)

	if 0 != int(myClock.Counter()) || myClock.WaitJobs() != 0 || myClock.jobList.Len()-1 != 0 {
		t.Errorf("应该执行%v次，实际执行%v次,此时任务队列中残余记录,myClock.actionindex.len=%v,jobList.len=%v\n", jobsNum-len(wantdeljobs), myClock.Counter(), myClock.WaitJobs(), myClock.jobList.Len()-1)

	}
}

func BenchmarkClock_AddJob(b *testing.B) {
	myClock := NewClock()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		newjob, inserted := myClock.AddJobWithTimeout(time.Millisecond*5, nil)
		if !inserted {
			b.Error("can not insert jobItem")
			break
		}
		<-newjob.C()
	}
}

// 测试通道消息传送的时间消耗
func BenchmarkChan(b *testing.B) {
	tmpChan := make(chan time.Duration, 1)
	maxnum := int64(math.MaxInt64)
	for i := 0; i < b.N; i++ {
		dur := time.Duration(maxnum - time.Now().UnixNano())
		tmpChan <- dur
		<-tmpChan

	}
}
