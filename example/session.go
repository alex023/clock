package main

import (
	"sync"
	"errors"
	"time"
	"fmt"
	"github.com/alex023/clock"
)

type Session struct {
	sync.Mutex
	cache map[string]tokenjob
	clock *clock.Clock
}
type tokenjob struct {
	token string
	jobid uint64
}

func NewSession() *Session {
	return &Session{
		cache:make(map[string]tokenjob),
		clock: clock.NewClock(),
	}
}

// AddToken add token string which can release after seconds
// @interval：	TTL seconds
// return:
//	@added:	if add when inserted sucessfull;else updated release time
//	@error:	if interval==0
func (s *Session) AddToken(token string, interval uint64) (added bool, err error) {
	if interval == 0 {
		err = errors.New("interval cannot be zero!")
		return
	}
	s.Lock()
	defer s.Unlock()

	item, founded := s.cache[token]
	if founded {
		s.clock.UpdateJobTimeout(item.jobid, time.Duration(interval)*time.Second)
		added = false //update token
	} else {
		job, _ := s.clock.AddJobWithTimeout(time.Duration(interval)*time.Second, func() { s.removeToken(token) })
		item := tokenjob{
			token: token,
			jobid: job.Id(),
		}
		s.cache[token] = item
		added = true
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

func (s *Session) GetTokenNum()int {
	s.Lock()
	defer s.Unlock()

	return len(s.cache)
}
func (s *Session) removeToken(token string) {
	s.Lock()
	defer s.Unlock()
	fmt.Println("token:",token," is removed!@",time.Now().Format("15:04:05:00")) //just for watching
	delete(s.cache, token)
}

func main() {
	session := NewSession()
	fmt.Println("test add token,and ttl can action")
	session.AddToken("alex023", 3)
	for i:=0;i<3;i++{
		time.Sleep(time.Second*2)
		fmt.Printf("%v|session have %2d tokens,found token=alex023 %v \n",time.Now().Format("15:04:05"),session.GetTokenNum(),session.GetToken("alex023"))
	}
	fmt.Println()

	fmt.Println("test add token and update it")
	session.AddToken("alex023_2", 4)
	for i:=0;i<5;i++{
		time.Sleep(time.Second*1)
		if i==1{
			session.AddToken("alex023_2",5)
		}
		fmt.Printf("%v|session have %2d tokens,found token=alex023_2 %v \n",time.Now().Format("15:04:05"),session.GetTokenNum(),session.GetToken("alex023_2"))
	}
}
