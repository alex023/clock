# clock
[![License](https://img.shields.io/:license-apache-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Report Card](https://goreportcard.com/badge/github.com/alex023/clock)](https://goreportcard.com/report/github.com/alex023/clock)
[![GoDoc](https://godoc.org/github.com/alex023/clock?status.svg)](https://godoc.org/github.com/alex023/clock)
[![Build Status](https://travis-ci.org/alex023/clock?status.svg)](https://travis-ci.org/alex023/clock)
[![Coverage Status](https://coveralls.io/repos/github/alex023/clock/badge.svg?branch=dev)](https://coveralls.io/github/alex023/clock?branch=dev)

 定时任务消息通知队列，实现了单一timer对多个注册任务的触发调用，其特点在于：
	
    1、能够添加一次性、重复性任务，并能在其执行前频繁撤销或更改。
    2、支持同一时间点，多个任务提醒。
    3、适用于单机，中等密度，大跨度的单次、多次定时任务。
    4、支持10万次/秒的定时任务执行、提醒、撤销或添加操作，平均延迟10微秒内
    5、支持注册任务的函数调用，及事件通知。    
 
 基本处理逻辑：

	1、重复性任务，流程是：
 		a、注册重复任务
		b、时间抵达时，控制器调用注册函数，并发送通知
		c、如果次数达到限制，则撤销；否则，控制器更新该任务的下次执行时间点
		d、控制器等待下一个最近需要执行的任务
	2、一次性任务，可以是服务运行时，当前时间点之后的任意事件，流程是：
 		a、注册一次性任务
		b、时间抵达时，控制器调用注册函数，并发送通知
		c、控制器释放该任务
		d、控制器等待下一个最近需要执行的任务
 
 使用方式，参见示例代码。