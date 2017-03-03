package clock

import (
	"fmt"
	"github.com/HuKeping/rbtree"
	"log"
	"testing"
	"time"
)

//因为采用的第三方实现的红黑树，为确保其功能与性能，进行补充测试。
// 重点关注struct中，多属性同时作为判等条件的情况下，能否正确管理。
var (
	size = 100
	//r    = rand.New(rand.NewSource(time.Now().Unix()))
)

func newOnceJob(delay time.Duration) jobItem {
	now := time.Now()
	job := jobItem{
		createTime:   now,
		intervalTime: delay,
		fn: func() {
			fmt.Println("临时任务事件")
		},
	}
	return job
}
func print(item rbtree.Item) bool {
	ite, ok := item.(*jobItem)
	if !ok {
		return false
	}
	log.Printf("%+v|%+v \n", ite.id, ite.actionTime.String())
	return true
}
func TestRbtree_Insert(t *testing.T) {

	items := make([]jobItem, size)
	//now:=time.Now()
	tree := rbtree.New()
	for i := 0; i < size; i++ {
		items[i] = newOnceJob(time.Duration(i * 1000))
		items[i].actionTime = items[i].createTime.Add(items[i].intervalTime)
		tree.Insert(items[i])

	}
	startnode := tree.Min()
	tree.Ascend(startnode, print)
}

func TestRbtree_Delete(t *testing.T) {
	var (
		tree         = rbtree.New()
		items        = make([]*jobItem, size)
		wantdelitems = make([]*jobItem, 0)

		mod = 5 + r.Intn(15) // 获取(5,20)之间的随机数
	)
	print := func(item rbtree.Item) bool {
		_, ok := item.(Job)
		if !ok {
			return false
		}
		//t.Logf("%+v|%+v \n", je.id, je.actionTime.String())
		return true
	}

	for i := 0; i < size; i++ {
		item := newOnceJob(time.Duration(i * 1000))
		item.actionTime = item.createTime.Add(item.intervalTime)
		items[i] = &item
		tree.Insert(&item)
		//t.Logf("itmes[%v]:%+v \n",i,item)

		if i%mod == 0 {
			wantdelitems = append(wantdelitems, &item)
		}
	}

	for i := 0; i < len(wantdelitems); i++ {
		tree.Delete(wantdelitems[i])

	}
	if int(tree.Len()) != len(items)-len(wantdelitems) {
		t.Errorf("数据删除失败！最终树中存在%v条记录，而期望存在%v条记录", tree.Len(), len(items)-len(wantdelitems))
		startnode := tree.Min()
		tree.Ascend(startnode, print)

	}
}
