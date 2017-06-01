# Clock
[![License](https://img.shields.io/:license-apache-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Report Card](https://goreportcard.com/badge/github.com/alex023/clock)](https://goreportcard.com/report/github.com/alex023/clock)
[![GoDoc](https://godoc.org/github.com/alex023/clock?status.svg)](https://godoc.org/github.com/alex023/clock)
[![Build Status](https://travis-ci.org/alex023/clock.svg?branch=dev)](https://travis-ci.org/alex023/clock?branch=dev)
[![Coverage Status](https://coveralls.io/repos/github/alex023/clock/badge.svg?branch=dev)](https://coveralls.io/github/alex023/clock?branch=dev)
 
# Brief
 Timing task manager based on red black tree in memory
 
# Feature
 - support task function call, and event notifications
 - support task that executes once or several times
 - support task cancel which added
 - 100k/s execution,  add  or remove task operation
     
 # Example
 ## add a task that executes once
 ```golang
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
 ```
 ## add repeat task that executes three times
 ```golang
 func ExampleClock_AddJobRepeat() {
 	var (
    		myClock = NewClock()
    	)
    	//define a repeat task 
    	fn := func() {
    		fmt.Println("schedule repeat")
    	}
    	//add in clock,execute three times,interval 200 millisecond
    	_, inserted := myClock.AddJobRepeat(time.Duration(time.Millisecond*200), 3, fn)
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
 ```
 ## more examples
 ### [event notify][1]
 ### [TTL Session][2] 
 [1]: https://github.com/alex023/clock/blob/master/clock_example_test.go#L11 
 [2]: https://github.com/alex023/clock/blob/master/example/session.go