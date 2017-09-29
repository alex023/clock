package main

import (
	"errors"
	"fmt"
	"github.com/alex023/clock"
	"sync"
	"time"
)

// Session is a large volume session manager that supports TTL
type Session struct {
	sync.Mutex
	cache map[string]tokenjob
	clock *clock.Clock
}
type tokenjob struct {
	token string
	job   clock.Job
}

// NewSession create a Session
func NewSession() *Session {
	return &Session{
		cache: make(map[string]tokenjob),
		clock: clock.NewClock(),
	}
}

// AddToken add token string which can release after seconds
// @interval：	Token of the survival time
// return:
//	@added:		return true if add as new,else return false
//	@error:		if interval==0
func (s *Session) AddToken(token string, timeout time.Duration) (added bool, err error) {
	if timeout == 0 {
		err = errors.New("timeout cannot be zero")
		return
	}
	s.Lock()
	defer s.Unlock()

	item, founded := s.cache[token]
	if founded {
		s.clock.UpdateJobTimeout(item.job, timeout)
		added = false //update token
		fmt.Printf("%v| [updated] token [%v] duration=[%2d] \n", time.Now().Format("15:04:05"), token, int(timeout.Seconds()))

	} else {
		job, _ := s.clock.AddJobWithInterval(timeout, func() { s.RemoveToken(token) })
		item := tokenjob{
			token: token,
			job:   job,
		}
		s.cache[token] = item
		added = true
		//output for example
		fmt.Printf("%v| [ added ] token [%v] duration=[%2d] \n", time.Now().Format("15:04:05"), token, int(timeout.Seconds()))
	}
	return
}

// GetToken determine whether token exists
func (s *Session) GetToken(token string) bool {
	s.Lock()
	defer s.Unlock()
	_, founded := s.cache[token]
	return founded
}

//GetTokenNum return numbers of tokens in Session
func (s *Session) GetTokenNum() int {
	s.Lock()
	defer s.Unlock()

	return len(s.cache)
}

//RemoveToken remove token by name from Session
func (s *Session) RemoveToken(token string) {
	s.Lock()
	defer s.Unlock()
	fmt.Printf("%v| [removed] token [%v] by TTLSession \n", time.Now().Format("15:04:05"), token)
	if tokenjob,founded:=s.cache[token];founded{
		delete(s.cache, token)
		tokenjob.job.Cancel()
	}
}

func main() {
	session := NewSession()

	token1 := "alex023_1"
	fmt.Println("add token and timeout")
	session.AddToken(token1, time.Second*3)
	for i := 0; i < 5; i++ {
		time.Sleep(time.Second * 1)
		fmt.Printf("%v|[watching] token [%v] founded=%v\n", time.Now().Format("15:04:05"), token1, session.GetToken(token1))
	}
	fmt.Println()

	token2 := "alex023_2"
	fmt.Println("add token and update it before timeout")
	session.AddToken(token2, time.Second*3)
	for i := 0; i < 10; i++ {
		time.Sleep(time.Second * 1)
		if i == 1 {
			session.AddToken(token2, time.Second*5)
		}
		fmt.Printf("%v|[watching] token [%v] founded=%v\n", time.Now().Format("15:04:05"), token2, session.GetToken(token2))
	}
}
